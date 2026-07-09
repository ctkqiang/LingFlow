package connections

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"ling_flow/internal/events"
	"ling_flow/internal/models"
	"ling_flow/internal/utilities"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultHeartbeatInterval     = 30 * time.Second
	defaultHeartbeatTimeout      = 90 * time.Second
	defaultHeartbeatWriteTimeout = 10 * time.Second
	defaultMaxConnectionsPerIP   = 10
	defaultMaxFrameBytes         = 64 * 1024 // 64KB WebSocket frame cap
)

type MessageHandler interface {
	HandleIncomingMessage(ctx context.Context, rawPayload []byte) ([]byte, error)
}

type ChatStreamer interface {
	HandleUserChatWithStreaming(ctx context.Context, connectionID string, messagePayload []byte) bool
}

// ConnectionReadyNotifier 连接建立就绪时的回调接口，
// 用于向新连接发送初始数据（如技能列表、欢迎消息等）。
type ConnectionReadyNotifier interface {
	OnConnectionReady(ctx context.Context, connectionID string)
}

const WebSocketChatEndpointPathPrefix = "/chat/"

type AWSWebSokcetGateway struct{}

type connectionState struct {
	conn            *websocket.Conn
	lastActiveAt    time.Time
	heartbeatTicker *time.Ticker
	pingNonce       string
	pingSentAt      time.Time
	connectionMutex sync.Mutex
}

type WebsokcetConnectionManager struct {
	connections         map[string]*connectionState
	eventStore          events.EventStore
	managerMutex        sync.Mutex
	heartbeatInterval   time.Duration
	heartbeatTimeout    time.Duration
	writeTimeout        time.Duration
	maxConnectionsPerIP int
	ipConnectionCount   map[string]int
	ipConnectionMutex   sync.Mutex
	ctx                 context.Context
	cancel              context.CancelFunc
}

func NewWebsocketGatewayConnectionManager() *WebsokcetConnectionManager {
	return &WebsokcetConnectionManager{
		connections:         make(map[string]*connectionState),
		ipConnectionCount:   make(map[string]int),
		heartbeatInterval:   defaultHeartbeatInterval,
		heartbeatTimeout:    defaultHeartbeatTimeout,
		writeTimeout:        defaultHeartbeatWriteTimeout,
		maxConnectionsPerIP: defaultMaxConnectionsPerIP,
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

	maxPerIP := defaultMaxConnectionsPerIP
	if v := os.Getenv("WSS_MAX_CONNECTIONS_PER_IP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPerIP = n
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	manager := &WebsokcetConnectionManager{
		connections:         make(map[string]*connectionState),
		ipConnectionCount:   make(map[string]int),
		eventStore:          eventStore,
		heartbeatInterval:   interval,
		heartbeatTimeout:    timeout,
		writeTimeout:        writeTimeout,
		maxConnectionsPerIP: maxPerIP,
		ctx:                 ctx,
		cancel:              cancel,
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
) bool {
	remoteIP := remoteIPFromAddr(websocketConnection.RemoteAddr().String())
	if remoteIP != "" && connectionManager.maxConnectionsPerIP > 0 {
		connectionManager.ipConnectionMutex.Lock()
		if connectionManager.ipConnectionCount[remoteIP] >= connectionManager.maxConnectionsPerIP {
			connectionManager.ipConnectionMutex.Unlock()
			utilities.LogWarn(
				"Connections",
				"AddConnection",
				fmt.Sprintf("IP %s 达到最大连接数 %d，拒绝新连接", remoteIP, connectionManager.maxConnectionsPerIP),
				0,
			)
			return false
		}
		connectionManager.ipConnectionCount[remoteIP]++
		connectionManager.ipConnectionMutex.Unlock()
	}

	connectionManager.managerMutex.Lock()
	defer connectionManager.managerMutex.Unlock()

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
	return true
}

func (connectionManager *WebsokcetConnectionManager) RemoveConnection(connectionIdentifier string) {
	connectionManager.managerMutex.Lock()

	state, exists := connectionManager.connections[connectionIdentifier]
	if exists {
		remoteIP := remoteIPFromAddr(state.conn.RemoteAddr().String())
		if remoteIP != "" {
			connectionManager.ipConnectionMutex.Lock()
			connectionManager.ipConnectionCount[remoteIP]--
			if connectionManager.ipConnectionCount[remoteIP] <= 0 {
				delete(connectionManager.ipConnectionCount, remoteIP)
			}
			connectionManager.ipConnectionMutex.Unlock()
		}

		if state.heartbeatTicker != nil {
			state.heartbeatTicker.Stop()
		}

		delete(connectionManager.connections, connectionIdentifier)
	}
	connectionManager.managerMutex.Unlock()

	if !exists {
		return
	}

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
	connectionManager.managerMutex.Lock()
	defer connectionManager.managerMutex.Unlock()

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
	connectionManager.managerMutex.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	connectionManager.managerMutex.Unlock()

	if !exists {
		return fmt.Errorf("连接不存在: %s", connectionIdentifier)
	}

	return connectionManager.sendMessageToConnection(state.conn, messagePayload)
}

func (connectionManager *WebsokcetConnectionManager) SendMessage(
	connectionID string,
	payload []byte,
) error {
	return connectionManager.SendMessageToConnection(connectionID, payload)
}

func (connectionManager *WebsokcetConnectionManager) sendMessageToConnection(conn *websocket.Conn, payload []byte) error {
	conn.SetWriteDeadline(time.Now().Add(connectionManager.writeTimeout))
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func (connectionManager *WebsokcetConnectionManager) UpdateLastActive(connectionIdentifier string) {
	connectionManager.managerMutex.Lock()
	defer connectionManager.managerMutex.Unlock()

	if state, exists := connectionManager.connections[connectionIdentifier]; exists {
		state.connectionMutex.Lock()
		state.lastActiveAt = time.Now()
		state.connectionMutex.Unlock()
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
			Nonce:                nonce,
			Action:               "ping",
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
			Nonce:                nonce,
			LatencyMs:            latency,
			Action:               "pong",
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
			Nonce:                nonce,
			Action:               "pong",
		},
		map[string]string{"component": "connections"},
	)

	connectionManager.managerMutex.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	connectionManager.managerMutex.Unlock()

	if exists && state.pingNonce == nonce {
		latency := time.Since(state.pingSentAt).Milliseconds()
		utilities.LogProgress(
			"Heartbeat",
			"Ping/Pong",
			fmt.Sprintf("收到 pong [%s], nonce=%s, latency=%dms", connectionIdentifier, nonce, latency),
		)
		state.connectionMutex.Lock()
		state.pingNonce = ""
		state.connectionMutex.Unlock()
	}
}

func (connectionManager *WebsokcetConnectionManager) sendPing(connectionIdentifier string) {
	connectionManager.managerMutex.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	if !exists {
		connectionManager.managerMutex.Unlock()
		return
	}

	if state.pingNonce != "" {
		connectionManager.managerMutex.Unlock()
		return
	}

	nonceBytes := make([]byte, 16)
	if _, randErr := rand.Read(nonceBytes); randErr != nil {
		connectionManager.managerMutex.Unlock()
		utilities.Error("生成心跳随机数失败 [%s]: %v", connectionIdentifier, randErr)
		return
	}
	nonce := hex.EncodeToString(nonceBytes)
	state.pingNonce = nonce
	state.pingSentAt = time.Now()
	state.lastActiveAt = time.Now()
	websocketConnection := state.conn
	connectionManager.managerMutex.Unlock()

	pingMessage, err := buildHeartbeatPing(nonce)
	if err != nil {
		utilities.Error("构建 ping 消息失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	if err := connectionManager.sendMessageToConnection(websocketConnection, pingMessage); err != nil {
		utilities.Error("发送 ping 失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeHeartbeatPingSent,
		events.HeartbeatEventData{
			ConnectionIdentifier: connectionIdentifier,
			Nonce:                nonce,
			Action:               "ping",
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
			connectionManager.managerMutex.Lock()
			_, exists := connectionManager.connections[connectionIdentifier]
			connectionManager.managerMutex.Unlock()

			if !exists {
				return
			}

			connectionManager.sendPing(connectionIdentifier)

			select {
			case <-time.After(connectionManager.heartbeatTimeout):
				connectionManager.managerMutex.Lock()
				s, ok := connectionManager.connections[connectionIdentifier]
				if ok && s.pingNonce != "" {
					connectionManager.managerMutex.Unlock()
					connectionManager.handleHeartbeatTimeout(connectionIdentifier)
					return
				}
				connectionManager.managerMutex.Unlock()
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
	connectionManager.managerMutex.Lock()
	defer connectionManager.managerMutex.Unlock()

	now := time.Now()
	for connectionIdentifier, state := range connectionManager.connections {
		state.connectionMutex.Lock()
		lastActive := state.lastActiveAt
		state.connectionMutex.Unlock()

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
			Action:               "timeout",
		},
		map[string]string{"component": "connections"},
	)

	utilities.LogProgress(
		"Heartbeat",
		"Timeout",
		fmt.Sprintf("连接心跳超时 [%s]，将断开连接", connectionIdentifier),
	)

	connectionManager.managerMutex.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	if exists {
		state.conn.Close()
		delete(connectionManager.connections, connectionIdentifier)
	}
	connectionManager.managerMutex.Unlock()
}

func WebsocketHandler(upgrader *websocket.Upgrader) {
	websocketConnectionManager := NewWebsocketGatewayConnectionManager()
	RegisterWebSocketHandlers(http.DefaultServeMux, websocketConnectionManager, upgrader, nil, nil, nil)
}

func RegisterWebSocketHandlers(
	websocketServeMux *http.ServeMux,
	connectionManager *WebsokcetConnectionManager,
	upgrader *websocket.Upgrader,
	messageHandler MessageHandler,
	chatStreamer ChatStreamer,
	connectionNotifier ConnectionReadyNotifier,
) {
	if upgrader == nil {
		upgrader = NewDefaultUpgrader()
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

		authenticatedUserID, authOK := authenticateWebSocketUpgrade(httpRequest)
		if !authOK {
			utilities.LogWarn(
				"Connections",
				"WebsocketHandler",
				fmt.Sprintf("WebSocket 升级鉴权失败，拒绝连接 [%s]", connectionIdentifier),
				0,
				fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
			)
			http.Error(responseWriter, "unauthorized", http.StatusUnauthorized)
			return
		}

		websocketConnection, upgradeError := upgrader.Upgrade(responseWriter, httpRequest, nil)
		if upgradeError != nil {
			utilities.Error("无法升级 HTTP 请求为 WebSocket 连接: %v", upgradeError)
			return
		}

		// Enforce a hard cap on incoming frame size to prevent OOM via giant frames.
		websocketConnection.SetReadLimit(defaultMaxFrameBytes)

		accepted := connectionManager.AddConnection(connectionIdentifier, websocketConnection)
		if !accepted {
			_ = websocketConnection.Close()
			http.Error(responseWriter, "too many connections", http.StatusTooManyRequests)
			return
		}
		defer func() {
			connectionManager.RemoveConnection(connectionIdentifier)
			_ = websocketConnection.Close()
		}()

		utilities.LogProgress(
			"WebsocketHandler",
			"Connected",
			fmt.Sprintf("WebSocket 已连接 [%s] user=%s", connectionIdentifier, authenticatedUserID),
		)

		if connectionNotifier != nil {
			go connectionNotifier.OnConnectionReady(httpRequest.Context(), connectionIdentifier)
		}

		readDeadline := 2 * connectionManager.heartbeatTimeout
		for {
			_ = websocketConnection.SetReadDeadline(time.Now().Add(readDeadline))
			messageType, messagePayload, readError := websocketConnection.ReadMessage()
			if readError != nil {
				if isExpectedCloseError(readError) {
					utilities.LogProgress(
						"WebsocketHandler",
						"ReadMessage",
						fmt.Sprintf("连接正常关闭 [%s] user=%s", connectionIdentifier, authenticatedUserID),
					)
				} else {
					utilities.LogWarn(
						"WebsocketHandler",
						"ReadMessage",
						fmt.Sprintf("连接异常关闭 [%s]: %v", connectionIdentifier, readError),
						0,
					)
				}
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

			if chatStreamer != nil {
				handled := chatStreamer.HandleUserChatWithStreaming(
					httpRequest.Context(), connectionIdentifier, messagePayload,
				)
				if handled {
					continue
				}
			}

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
	if err := json.Unmarshal(payload, &msg); err != nil {
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
	if err := json.Unmarshal(payload, &msg); err != nil {
		utilities.Error("解析心跳消息失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	var heartbeatData models.HeartbeatChatData
	if err := json.Unmarshal(msg.Data, &heartbeatData); err != nil {
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

	dataBytes, err := json.Marshal(heartbeatData)
	if err != nil {
		return nil, err
	}

	msg := models.WSMessage{
		Type:      models.HeartbeatChat,
		Data:      dataBytes,
		Timestamp: time.Now(),
	}

	return json.Marshal(msg)
}

func buildHeartbeatPong(nonce string, pingSentAt time.Time) ([]byte, error) {
	latency := time.Since(pingSentAt).Milliseconds()
	heartbeatData := models.HeartbeatChatData{
		Action:    "pong",
		Nonce:     nonce,
		Timestamp: time.Now(),
		Latency:   latency,
	}

	dataBytes, err := json.Marshal(heartbeatData)
	if err != nil {
		return nil, err
	}

	msg := models.WSMessage{
		Type:      models.HeartbeatChat,
		Data:      dataBytes,
		Timestamp: time.Now(),
	}

	return json.Marshal(msg)
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
		utilities.LogWarn(
			"Connections",
			"isWebSocketOriginAllowed",
			"WSS_ALLOW_ALL_ORIGINS=true 已启用，接受所有 Origin（仅限本地调试）",
			0,
		)
		return true
	}

	if !utilities.IsProductionMode() {
		requestOrigin := strings.TrimSpace(httpRequest.Header.Get("Origin"))
		utilities.LogProgress(
			"Connections",
			"isWebSocketOriginAllowed",
			fmt.Sprintf("开发模式，允许 Origin: %s", requestOrigin),
		)
		return true
	}

	requestOrigin := strings.TrimSpace(httpRequest.Header.Get("Origin"))
	if requestOrigin == "" {
		return false
	}

	allowedOriginsList := os.Getenv("WSS_ALLOWED_ORIGINS")
	if allowedOriginsList == "" {
		return false
	}

	for _, allowedOrigin := range strings.Split(allowedOriginsList, ",") {
		if strings.EqualFold(strings.TrimSpace(allowedOrigin), requestOrigin) {
			return true
		}
	}
	return false
}

func isLocalhostOrigin(origin string) bool {
	localhostVariants := []string{
		"http://localhost",
		"http://127.0.0.1",
		"https://localhost",
		"https://127.0.0.1",
	}
	for _, variant := range localhostVariants {
		if strings.HasPrefix(origin, variant) {
			return true
		}
	}
	return false
}

func remoteIPFromAddr(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// isExpectedCloseError returns true when the WebSocket closed for a benign
// reason (client disconnect, normal close handshake, idle timeout).
func isExpectedCloseError(err error) bool {
	if err == nil {
		return true
	}
	if websocket.IsCloseError(
		err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
	) {
		return true
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer")
}

// authenticateWebSocketUpgrade 验证 WebSocket 升级前的认证。
//
// 判断逻辑：
//   - 非生产模式（MODE!=production）：验证 token 是否为 debug-token-* 格式，提取用户 ID
//   - 生产模式（MODE=production）：调用 authenticateWebSocketProduction() 执行真实认证
//
// 开发模式流程：
//   1. 客户端先调用 POST /api/auth/token 获取 debug-token-{user_id}
//   2. 连接 WebSocket 时携带 ?token=debug-token-{user_id}
//   3. 服务端验证 token 格式，提取 user_id
func authenticateWebSocketUpgrade(httpRequest *http.Request) (string, bool) {
	token := strings.TrimSpace(httpRequest.URL.Query().Get("token"))
	if token == "" {
		authHeader := strings.TrimSpace(httpRequest.Header.Get("Authorization"))
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}

	if !utilities.IsProductionMode() {
		if token == "" {
			utilities.LogWarn(
				"Connections",
				"authenticateWebSocketUpgrade",
				"开发模式下需要提供 token，请先调用 POST /api/auth/token",
				0,
				fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
			)
			return "", false
		}

		if strings.HasPrefix(token, "debug-token-") {
			userID := strings.TrimPrefix(token, "debug-token-")
			utilities.LogProgress(
				"Connections",
				"authenticateWebSocketUpgrade",
				fmt.Sprintf("开发模式认证通过，用户=%s", userID),
				fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
			)
			return userID, true
		}

		utilities.LogWarn(
			"Connections",
			"authenticateWebSocketUpgrade",
			"无效的 debug token 格式，请使用 POST /api/auth/token 获取",
			0,
			fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
		)
		return "", false
	}

	return authenticateWebSocketProduction(httpRequest)
}

// authenticateWebSocketProduction 在生产模式下执行真实的 WebSocket 认证逻辑。
// 用户需要根据自己的业务需求实现以下逻辑：
//
// 1. 从请求中提取认证凭证（token、API Key 等）
// 2. 验证凭证的有效性（检查签名、过期时间等）
// 3. 提取并返回用户 ID
//
// 当前为占位实现，请根据实际需求修改：
//   - 修改凭证提取方式（当前从 ?token= 或 Authorization: Bearer 提取）
//   - 添加 Token 验证逻辑（JWT 解析、HMAC 验证等）
//   - 添加速率限制和防暴力破解措施
func authenticateWebSocketProduction(httpRequest *http.Request) (string, bool) {
	token := strings.TrimSpace(httpRequest.URL.Query().Get("token"))
	if token == "" {
		authHeader := strings.TrimSpace(httpRequest.Header.Get("Authorization"))
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}

	if token == "" {
		utilities.LogWarn(
			"Connections",
			"authenticateWebSocketProduction",
			"生产模式下缺少认证 token",
			0,
			fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
		)
		return "", false
	}

	// TODO: 用户需要实现的认证逻辑
	// ============================================
	// 1. 解析并验证 Token（JWT、HMAC 等）
	//    claims, err := parseAndVerifyToken(token)
	//    if err != nil {
	//        return "", false
	//    }
	//
	// 2. 提取用户 ID
	//    userID := claims.UserID
	// ============================================

	// 以下为示例代码，生产环境必须替换为真实实现
	_ = token // 移除警告：生产环境需要使用此变量

	utilities.LogProgress(
		"Connections",
		"authenticateWebSocketProduction",
		fmt.Sprintf("生产模式认证通过，用户=%s", "user-from-token"),
	)
	return "user-from-token", true
}

func computeTokenMAC(payload string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
