package connections

import (
	"context"
	"fmt"
	"ling_flow/internal/events"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultHeartbeatInterval     = 30 * time.Second
	defaultHeartbeatTimeout      = 90 * time.Second
	defaultHeartbeatWriteTimeout = 10 * time.Second
)

type MessageHandler interface {
	HandleIncomingMessage(ctx context.Context, rawPayload []byte) ([]byte, error)
}

const WebSocketChatEndpointPathPrefix = "/chat/"

type AWSWebSokcetGateway struct{}

type connectionState struct {
	conn           *websocket.Conn
	lastActiveAt   time.Time
	heartbeatTicker *time.Ticker
	pingNonce      string
	pingSentAt     time.Time
	mu             sync.Mutex
}

type WebsokcetConnectionManager struct {
	connections      map[string]*connectionState
	eventStore       events.EventStore
	mu               sync.Mutex
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	writeTimeout      time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
}

func NewWebsocketGatewayConnectionManager() *WebsokcetConnectionManager {
	return &WebsokcetConnectionManager{
		connections:      make(map[string]*connectionState),
		heartbeatInterval: defaultHeartbeatInterval,
		heartbeatTimeout:  defaultHeartbeatTimeout,
		writeTimeout:      defaultHeartbeatWriteTimeout,
	}
}

func NewEventSourcedWebsocketGatewayConnectionManager(
	eventStore events.EventStore,
) *WebsokcetConnectionManager {
	interval := defaultHeartbeatInterval
	if v := os.Getenv("WSS_HEARTBEAT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	timeout := defaultHeartbeatTimeout
	if v := os.Getenv("WSS_HEARTBEAT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		}
	}

	writeTimeout := defaultHeartbeatWriteTimeout
	if v := os.Getenv("WSS_HEARTBEAT_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			writeTimeout = d
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	manager := &WebsokcetConnectionManager{
		connections:      make(map[string]*connectionState),
		eventStore:       eventStore,
		heartbeatInterval: interval,
		heartbeatTimeout:  timeout,
		writeTimeout:      writeTimeout,
		ctx:              ctx,
		cancel:           cancel,
	}

	go manager.startHeartbeatMonitor()

	return manager
}

func NewDefaultUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     isWebSocketOriginAllowed,
	}
}

func (connectionManager *WebsokcetConnectionManager) AddConnection(
	connectionIdentifier string,
	websocketConnection *websocket.Conn,
) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	state := &connectionState{
		conn:         websocketConnection,
		lastActiveAt: time.Now(),
	}

	connectionManager.connections[connectionIdentifier] = state
	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeChatSessionConnected,
		events.ChatSessionEventData{
			ConnectionIdentifier: connectionIdentifier,
			RemoteAddress:        websocketConnection.RemoteAddr().String(),
		},
		map[string]string{"component": "connections"},
	)

	go connectionManager.startConnectionHeartbeat(connectionIdentifier, state)

	utilities.LogProgress(
		"AddConnection",
		"连接管理",
		fmt.Sprintf("连接已添加，当前连接总数: %d", len(connectionManager.connections)),
	)
}

func (connectionManager *WebsokcetConnectionManager) RemoveConnection(connectionIdentifier string) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	state, exists := connectionManager.connections[connectionIdentifier]
	if !exists {
		return
	}

	if state.heartbeatTicker != nil {
		state.heartbeatTicker.Stop()
	}

	delete(connectionManager.connections, connectionIdentifier)
	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeChatSessionDisconnected,
		events.ChatSessionEventData{
			ConnectionIdentifier: connectionIdentifier,
		},
		map[string]string{"component": "connections"},
	)

	utilities.LogProgress(
		"RemoveConnection",
		"连接管理",
		fmt.Sprintf("连接已移除 [%s]，当前连接总数: %d", connectionIdentifier, len(connectionManager.connections)),
	)
}

func (connectionManager *WebsokcetConnectionManager) BroadcastMessage(messagePayload []byte) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	recipientCount := len(connectionManager.connections)
	for connectionIdentifier, state := range connectionManager.connections {
		failedCount := 0
		if sendError := connectionManager.sendMessageToConnection(state.conn, messagePayload); sendError != nil {
			failedCount++
			utilities.Error(
				"向连接 [%s] 发送消息失败: %v",
				connectionIdentifier,
				sendError,
			)
			state.conn.Close()
			delete(connectionManager.connections, connectionIdentifier)
		}

		connectionManager.recordEvent(
			context.Background(),
			connectionIdentifier,
			events.EventTypeChatMessageBroadcasted,
			events.ChatBroadcastEventData{
				ConnectionIdentifier: connectionIdentifier,
				RecipientCount:       recipientCount,
				PayloadSizeBytes:     len(messagePayload),
				FailedCount:          failedCount,
			},
			map[string]string{"component": "connections"},
		)
	}
}

func (connectionManager *WebsokcetConnectionManager) SendMessageToConnection(
	connectionIdentifier string,
	messagePayload []byte,
) error {
	connectionManager.mu.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	connectionManager.mu.Unlock()

	if !exists {
		return fmt.Errorf("连接不存在: %s", connectionIdentifier)
	}

	return connectionManager.sendMessageToConnection(state.conn, messagePayload)
}

func (connectionManager *WebsokcetConnectionManager) sendMessageToConnection(conn *websocket.Conn, payload []byte) error {
	conn.SetWriteDeadline(time.Now().Add(connectionManager.writeTimeout))
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func (connectionManager *WebsokcetConnectionManager) UpdateLastActive(connectionIdentifier string) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	if state, exists := connectionManager.connections[connectionIdentifier]; exists {
		state.mu.Lock()
		state.lastActiveAt = time.Now()
		state.mu.Unlock()
	}
}

func (connectionManager *WebsokcetConnectionManager) handlePing(connectionIdentifier string, nonce string, sentAt time.Time) {
	connectionManager.UpdateLastActive(connectionIdentifier)

	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeHeartbeatPingReceived,
		events.HeartbeatEventData{
			ConnectionIdentifier: connectionIdentifier,
			Nonce:               nonce,
			Action:              "ping",
		},
		map[string]string{"component": "connections"},
	)

	pongMessage, err := buildHeartbeatPong(nonce, sentAt)
	if err != nil {
		utilities.Error("构建 pong 消息失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	if err := connectionManager.SendMessageToConnection(connectionIdentifier, pongMessage); err != nil {
		utilities.Error("发送 pong 失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	latency := time.Since(sentAt).Milliseconds()
	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeHeartbeatPongSent,
		events.HeartbeatEventData{
			ConnectionIdentifier: connectionIdentifier,
			Nonce:               nonce,
			LatencyMs:           latency,
			Action:              "pong",
		},
		map[string]string{"component": "connections"},
	)

	utilities.LogProgress(
		"Heartbeat",
		"Ping/Pong",
		fmt.Sprintf("收到 ping 并回复 pong [%s], nonce=%s, latency=%dms", connectionIdentifier, nonce, latency),
	)
}

func (connectionManager *WebsokcetConnectionManager) handlePong(connectionIdentifier string, nonce string) {
	connectionManager.UpdateLastActive(connectionIdentifier)

	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeHeartbeatPongReceived,
		events.HeartbeatEventData{
			ConnectionIdentifier: connectionIdentifier,
			Nonce:               nonce,
			Action:              "pong",
		},
		map[string]string{"component": "connections"},
	)

	connectionManager.mu.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	connectionManager.mu.Unlock()

	if exists && state.pingNonce == nonce {
		latency := time.Since(state.pingSentAt).Milliseconds()
		utilities.LogProgress(
			"Heartbeat",
			"Ping/Pong",
			fmt.Sprintf("收到 pong [%s], nonce=%s, latency=%dms", connectionIdentifier, nonce, latency),
		)
		state.mu.Lock()
		state.pingNonce = ""
		state.mu.Unlock()
	}
}

func (connectionManager *WebsokcetConnectionManager) sendPing(connectionIdentifier string) {
	connectionManager.mu.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	if !exists {
		connectionManager.mu.Unlock()
		return
	}

	if state.pingNonce != "" {
		connectionManager.mu.Unlock()
		return
	}

	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	state.pingNonce = nonce
	state.pingSentAt = time.Now()
	state.lastActiveAt = time.Now()
	conn := state.conn
	connectionManager.mu.Unlock()

	pingMessage, err := buildHeartbeatPing(nonce)
	if err != nil {
		utilities.Error("构建 ping 消息失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	if err := connectionManager.sendMessageToConnection(conn, pingMessage); err != nil {
		utilities.Error("发送 ping 失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeHeartbeatPingSent,
		events.HeartbeatEventData{
			ConnectionIdentifier: connectionIdentifier,
			Nonce:               nonce,
			Action:              "ping",
		},
		map[string]string{"component": "connections"},
	)

	utilities.LogProgress(
		"Heartbeat",
		"Ping/Pong",
		fmt.Sprintf("发送 ping [%s], nonce=%s", connectionIdentifier, nonce),
	)
}

func (connectionManager *WebsokcetConnectionManager) startConnectionHeartbeat(connectionIdentifier string, state *connectionState) {
	state.heartbeatTicker = time.NewTicker(connectionManager.heartbeatInterval)
	defer state.heartbeatTicker.Stop()

	for {
		select {
		case <-state.heartbeatTicker.C:
			connectionManager.mu.Lock()
			_, exists := connectionManager.connections[connectionIdentifier]
			connectionManager.mu.Unlock()

			if !exists {
				return
			}

			connectionManager.sendPing(connectionIdentifier)

			select {
			case <-time.After(connectionManager.heartbeatTimeout):
				connectionManager.mu.Lock()
				s, ok := connectionManager.connections[connectionIdentifier]
				if ok && s.pingNonce != "" {
					connectionManager.mu.Unlock()
					connectionManager.handleHeartbeatTimeout(connectionIdentifier)
					return
				}
				connectionManager.mu.Unlock()
			case <-connectionManager.ctx.Done():
				return
			}
		case <-connectionManager.ctx.Done():
			return
		}
	}
}

func (connectionManager *WebsokcetConnectionManager) startHeartbeatMonitor() {
	ticker := time.NewTicker(connectionManager.heartbeatInterval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			connectionManager.checkHeartbeatTimeouts()
		case <-connectionManager.ctx.Done():
			return
		}
	}
}

func (connectionManager *WebsokcetConnectionManager) checkHeartbeatTimeouts() {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	now := time.Now()
	for connectionIdentifier, state := range connectionManager.connections {
		state.mu.Lock()
		lastActive := state.lastActiveAt
		state.mu.Unlock()

		if now.Sub(lastActive) > connectionManager.heartbeatTimeout {
			go connectionManager.handleHeartbeatTimeout(connectionIdentifier)
		}
	}
}

func (connectionManager *WebsokcetConnectionManager) handleHeartbeatTimeout(connectionIdentifier string) {
	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeHeartbeatTimeout,
		events.HeartbeatEventData{
			ConnectionIdentifier: connectionIdentifier,
			Action:              "timeout",
		},
		map[string]string{"component": "connections"},
	)

	utilities.LogProgress(
		"Heartbeat",
		"Timeout",
		fmt.Sprintf("连接心跳超时 [%s]，将断开连接", connectionIdentifier),
	)

	connectionManager.mu.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	if exists {
		state.conn.Close()
		delete(connectionManager.connections, connectionIdentifier)
	}
	connectionManager.mu.Unlock()
}

func WebsocketHandler(upgrader *websocket.Upgrader) {
	websocketConnectionManager := NewWebsocketGatewayConnectionManager()
	RegisterWebSocketHandlers(http.DefaultServeMux, websocketConnectionManager, upgrader)
}

func RegisterWebSocketHandlers(
	websocketServeMux *http.ServeMux,
	connectionManager *WebsokcetConnectionManager,
	upgrader *websocket.Upgrader,
	messageHandlers ...MessageHandler,
) {
	if upgrader == nil {
		upgrader = NewDefaultUpgrader()
	}

	var messageHandler MessageHandler
	if len(messageHandlers) > 0 && messageHandlers[0] != nil {
		messageHandler = messageHandlers[0]
	}

	websocketServeMux.HandleFunc(WebSocketChatEndpointPathPrefix, func(
		responseWriter http.ResponseWriter,
		httpRequest *http.Request,
	) {
		connectionIdentifier, connectionIdentifierError := chatConnectionIdentifierFromRequest(httpRequest)
		if connectionIdentifierError != nil {
			http.Error(responseWriter, connectionIdentifierError.Error(), http.StatusBadRequest)
			return
		}

		websocketConnection, upgradeError := upgrader.Upgrade(responseWriter, httpRequest, nil)
		if upgradeError != nil {
			utilities.Error("无法升级 HTTP 请求为 WebSocket 连接: %v", upgradeError)
			return
		}

		connectionManager.AddConnection(connectionIdentifier, websocketConnection)
		defer func() {
			connectionManager.RemoveConnection(connectionIdentifier)
			_ = websocketConnection.Close()
		}()

		for {
			messageType, messagePayload, readError := websocketConnection.ReadMessage()
			if readError != nil {
				utilities.LogProgress(
					"WebsocketHandler",
					"ReadMessage",
					fmt.Sprintf("连接关闭 [%s]: %v", connectionIdentifier, readError),
				)
				return
			}

			if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
				continue
			}

			if isHeartbeatMessage(messagePayload) {
				handleHeartbeatMessage(connectionManager, connectionIdentifier, messagePayload)
				continue
			}

			connectionManager.UpdateLastActive(connectionIdentifier)
			connectionManager.recordEvent(
				httpRequest.Context(),
				connectionIdentifier,
				events.EventTypeChatMessageReceived,
				events.ChatMessageEventData{
					ConnectionIdentifier: connectionIdentifier,
					MessageType:          messageType,
					PayloadSizeBytes:     len(messagePayload),
					Payload:              string(messagePayload),
				},
				map[string]string{"component": "connections"},
			)

			if messageHandler != nil {
				responsePayload, handleError := messageHandler.HandleIncomingMessage(
					httpRequest.Context(), messagePayload,
				)
				if handleError != nil {
					connectionManager.recordEvent(
						httpRequest.Context(),
						connectionIdentifier,
						events.EventTypeChatMessageProcessingFailed,
						events.ChatMessageEventData{
							ConnectionIdentifier: connectionIdentifier,
							MessageType:          messageType,
							PayloadSizeBytes:     len(messagePayload),
							ErrorMessage:         handleError.Error(),
						},
						map[string]string{"component": "connections"},
					)
					utilities.Error("消息处理失败 [%s]: %v", connectionIdentifier, handleError)
					connectionManager.BroadcastMessage(messagePayload)
					continue
				}
				connectionManager.recordEvent(
					httpRequest.Context(),
					connectionIdentifier,
					events.EventTypeChatMessageProcessed,
					events.ChatMessageEventData{
						ConnectionIdentifier: connectionIdentifier,
						MessageType:          websocket.TextMessage,
						PayloadSizeBytes:     len(responsePayload),
						Payload:              string(responsePayload),
					},
					map[string]string{"component": "connections"},
				)
				connectionManager.BroadcastMessage(responsePayload)
			} else {
				connectionManager.BroadcastMessage(messagePayload)
			}
		}
	})
}

func isHeartbeatMessage(payload []byte) bool {
	var msg models.WSMessage
	if err := msg.UnmarshalJSON(payload); err != nil {
		return false
	}
	return msg.Type == models.HeartbeatChat
}

func handleHeartbeatMessage(
	connectionManager *WebsokcetConnectionManager,
	connectionIdentifier string,
	payload []byte,
) {
	var msg models.WSMessage
	if err := msg.UnmarshalJSON(payload); err != nil {
		utilities.Error("解析心跳消息失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	var heartbeatData models.HeartbeatChatData
	if err := msg.Data.Unmarshal(&heartbeatData); err != nil {
		utilities.Error("解析心跳数据失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	switch heartbeatData.Action {
	case "ping":
		connectionManager.handlePing(connectionIdentifier, heartbeatData.Nonce, heartbeatData.Timestamp)
	case "pong":
		connectionManager.handlePong(connectionIdentifier, heartbeatData.Nonce)
	default:
		utilities.Error("未知心跳动作 [%s]: %s", connectionIdentifier, heartbeatData.Action)
	}
}

func buildHeartbeatPing(nonce string) ([]byte, error) {
	heartbeatData := models.HeartbeatChatData{
		Action:    "ping",
		Nonce:     nonce,
		Timestamp: time.Now(),
	}

	dataBytes, err := heartbeatData.MarshalJSON()
	if err != nil {
		return nil, err
	}

	msg := models.WSMessage{
		Type:      models.HeartbeatChat,
		Data:      dataBytes,
		Timestamp: time.Now(),
	}

	return msg.MarshalJSON()
}

func buildHeartbeatPong(nonce string, pingSentAt time.Time) ([]byte, error) {
	latency := time.Since(pingSentAt).Milliseconds()
	heartbeatData := models.HeartbeatChatData{
		Action:    "pong",
		Nonce:     nonce,
		Timestamp: time.Now(),
		Latency:   latency,
	}

	dataBytes, err := heartbeatData.MarshalJSON()
	if err != nil {
		return nil, err
	}

	msg := models.WSMessage{
		Type:      models.HeartbeatChat,
		Data:      dataBytes,
		Timestamp: time.Now(),
	}

	return msg.MarshalJSON()
}

func chatConnectionIdentifierFromRequest(httpRequest *http.Request) (string, error) {
	connectionIdentifier := strings.TrimPrefix(httpRequest.URL.Path, WebSocketChatEndpointPathPrefix)
	switch {
	case connectionIdentifier == "":
		return "", fmt.Errorf("缺少 chat uuid，endpoint 格式为 /chat/{uuid}")
	case strings.Contains(connectionIdentifier, "/"):
		return "", fmt.Errorf("chat uuid 不能包含路径分隔符: %s", connectionIdentifier)
	default:
		return connectionIdentifier, nil
	}
}

func (connectionManager *WebsokcetConnectionManager) recordEvent(
	runtimeContext context.Context,
	connectionIdentifier string,
	eventType events.EventType,
	eventData interface{},
	metadata map[string]string,
) {
	if connectionManager.eventStore == nil {
		return
	}

	streamIdentifier := "websocket"
	if connectionIdentifier != "" {
		streamIdentifier = events.ChatStreamID(connectionIdentifier)
	}

	domainEvent, eventBuildError := events.NewDomainEvent(
		streamIdentifier,
		connectionIdentifier,
		eventType,
		eventData,
		metadata,
	)
	if eventBuildError != nil {
		utilities.Error("构建事件失败 [%s]: %v", eventType, eventBuildError)
		return
	}

	if _, appendError := connectionManager.eventStore.Append(runtimeContext, domainEvent); appendError != nil {
		utilities.Error("追加事件失败 [%s]: %v", eventType, appendError)
	}
}

func isWebSocketOriginAllowed(httpRequest *http.Request) bool {
	if strings.EqualFold(os.Getenv("WSS_ALLOW_ALL_ORIGINS"), "true") {
		return true
	}

	requestOrigin := httpRequest.Header.Get("Origin")
	if requestOrigin == "" {
		return true
	}

	for _, allowedOrigin := range strings.Split(os.Getenv("WSS_ALLOWED_ORIGINS"), ",") {
		if strings.EqualFold(strings.TrimSpace(allowedOrigin), requestOrigin) {
			return true
		}
	}

	return strings.Contains(requestOrigin, "://"+httpRequest.Host)
}