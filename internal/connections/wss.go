package connections

import (
	"fmt"
	"ling_flow/internal/utilities"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type AWSWebSokcetGateway struct{}

type WebsokcetConnectionManager struct {
	connections map[string]*websocket.Conn
	mu          sync.Mutex
}

func NewWebsocketGatewayConnectionManager() *WebsokcetConnectionManager {
	return &WebsokcetConnectionManager{
		connections: make(map[string]*websocket.Conn),
	}
}

func (connectionManager *WebsokcetConnectionManager) AddConnection(id string, conn *websocket.Conn) {
	connectionManager.mu.Lock()
	defer connectionManager.mu.Unlock()

	connectionManager.connections[id] = conn

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

	for connectionIdentifier, websocketConnection := range connectionManager.connections {
		if sendError := websocketConnection.WriteMessage(websocket.TextMessage, messagePayload); sendError != nil {
			utilities.Error(
				"向连接 [%s] 发送消息失败: %v",
				connectionIdentifier,
				sendError,
			)
			websocketConnection.Close()

			delete(connectionManager.connections, connectionIdentifier)
		}
	}
}

func WebsocketHandler(upgrader *websocket.Upgrader) {
	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		connection, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			utilities.Error("无法升级 HTTP 请求为 WebSocket 连接: %v", err)
			return
		}

		_ = connection.RemoteAddr().String()
	})
}
