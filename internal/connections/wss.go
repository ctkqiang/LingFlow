package connections

import (
	"context"
	"fmt"
	"ling_flow/internal/events"
	"ling_flow/internal/utilities"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// MessageHandler defines the interface for processing incoming WebSocket messages.
// Implementations receive raw message bytes and return response bytes.
type MessageHandler interface {
	HandleIncomingMessage(ctx context.Context, rawPayload []byte) ([]byte, error)
}

const WebSocketChatEndpointPathPrefix = "/chat/"

type AWSWebSokcetGateway struct{}

type WebsokcetConnectionManager struct {
	connections map[string]*websocket.Conn
	eventStore  events.EventStore
	mu          sync.Mutex
}

// NewWebsocketGatewayConnectionManager 创建 WebSocket 连接管理器。
//
// 该管理器用于 EC2/本地 server 模式下维护活跃连接；
// Lambda + API Gateway 模式下连接生命周期由 API Gateway 管理。
func NewWebsocketGatewayConnectionManager() *WebsokcetConnectionManager {
	return &WebsokcetConnectionManager{
		connections: make(map[string]*websocket.Conn),
	}
}

// NewEventSourcedWebsocketGatewayConnectionManager 创建带事件存储的连接管理器。
func NewEventSourcedWebsocketGatewayConnectionManager(
	eventStore events.EventStore,
) *WebsokcetConnectionManager {
	return &WebsokcetConnectionManager{
		connections: make(map[string]*websocket.Conn),
		eventStore:  eventStore,
	}
}

// NewDefaultUpgrader 返回项目默认的 WebSocket 升级器配置。
//
// Origin 校验默认仅允许同源请求；需要跨域时可通过
// WSS_ALLOWED_ORIGINS 或 WSS_ALLOW_ALL_ORIGINS 显式开启。
func NewDefaultUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     isWebSocketOriginAllowed,
	}
}

// AddConnection 注册一个新的 WebSocket 连接。
func (connectionManager *WebsokcetConnectionManager) AddConnection(
	connectionIdentifier string,
	websocketConnection *websocket.Conn,
) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	connectionManager.connections[connectionIdentifier] = websocketConnection
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

	utilities.LogProgress(
		"AddConnection",
		"连接管理",
		fmt.Sprintf("连接已添加，当前连接总数: %d", len(connectionManager.connections)),
	)
}

// RemoveConnection 根据连接标识移除一个 WebSocket 连接
func (connectionManager *WebsokcetConnectionManager) RemoveConnection(connectionIdentifier string) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

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

// BroadcastMessage 向所有已注册的 WebSocket 连接广播消息
func (connectionManager *WebsokcetConnectionManager) BroadcastMessage(messagePayload []byte) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	recipientCount := len(connectionManager.connections)
	failedCount := 0
	for connectionIdentifier, websocketConnection := range connectionManager.connections {
		if sendError := websocketConnection.WriteMessage(websocket.TextMessage, messagePayload); sendError != nil {
			failedCount++
			utilities.Error(
				"向连接 [%s] 发送消息失败: %v",
				connectionIdentifier,
				sendError,
			)
			websocketConnection.Close()

			delete(connectionManager.connections, connectionIdentifier)
		}
	}

	connectionManager.recordEvent(
		context.Background(),
		"",
		events.EventTypeChatMessageBroadcasted,
		events.ChatBroadcastEventData{
			RecipientCount:   recipientCount,
			PayloadSizeBytes: len(messagePayload),
			FailedCount:      failedCount,
		},
		map[string]string{"component": "connections"},
	)
}

// WebsocketHandler 将默认 /chat/{uuid} 处理器注册到 http.DefaultServeMux。
func WebsocketHandler(upgrader *websocket.Upgrader) {
	websocketConnectionManager := NewWebsocketGatewayConnectionManager()
	RegisterWebSocketHandlers(http.DefaultServeMux, websocketConnectionManager, upgrader)
}

// RegisterWebSocketHandlers 注册 EC2/本地 server 模式下的 WebSocket endpoint。
//
// endpoint 格式：/chat/{uuid}
// messageHandler 为可选参数，传入时消息将通过 LLM pipeline 处理后再广播。
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
