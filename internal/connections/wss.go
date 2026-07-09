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
	"runtime"
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

// memSnapshot 获取当前内存统计快照，用于详细日志输出。
func memSnapshot() (heapMB, sysMB float64, goroutines int) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / 1024 / 1024, float64(m.Sys) / 1024 / 1024, runtime.NumGoroutine()
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
	opStart := time.Now()
	utilities.LogStart("WebSocket连接管理", "AddConnection")

	remoteAddr := websocketConnection.RemoteAddr().String()
	remoteIP := remoteIPFromAddr(remoteAddr)
	heapMB, sysMB, goroutines := memSnapshot()

	// 详细记录新连接请求的完整上下文
	utilities.LogVerbose(
		"WebSocket连接管理", "AddConnection",
		fmt.Sprintf("收到新连接请求: 连接ID=%s, 远程地址=%s, 远程IP=%s", connectionIdentifier, remoteAddr, remoteIP),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("remote_addr=%s", remoteAddr),
		fmt.Sprintf("remote_ip=%s", remoteIP),
		fmt.Sprintf("heap_mb=%.2f", heapMB),
		fmt.Sprintf("sys_mb=%.2f", sysMB),
		fmt.Sprintf("goroutines=%d", goroutines),
		fmt.Sprintf("max_per_ip=%d", connectionManager.maxConnectionsPerIP),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

	if remoteIP != "" && connectionManager.maxConnectionsPerIP > 0 {
		connectionManager.ipConnectionMutex.Lock()
		currentIPCount := connectionManager.ipConnectionCount[remoteIP]

		utilities.LogVerbose(
			"WebSocket连接管理", "AddConnection",
			fmt.Sprintf("IP限流检查: IP=%s, 当前连接数=%d, 最大允许=%d", remoteIP, currentIPCount, connectionManager.maxConnectionsPerIP),
			fmt.Sprintf("ip=%s", remoteIP),
			fmt.Sprintf("current_ip_count=%d", currentIPCount),
			fmt.Sprintf("max_per_ip=%d", connectionManager.maxConnectionsPerIP),
		)

		if currentIPCount >= connectionManager.maxConnectionsPerIP {
			connectionManager.ipConnectionMutex.Unlock()
			utilities.LogWarn(
				"WebSocket连接管理",
				"AddConnection",
				fmt.Sprintf("IP %s 达到最大连接数 %d，拒绝新连接 [%s]", remoteIP, connectionManager.maxConnectionsPerIP, connectionIdentifier),
				time.Since(opStart),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("remote_ip=%s", remoteIP),
				fmt.Sprintf("current_ip_count=%d", currentIPCount),
				fmt.Sprintf("rejected=true"),
			)
			utilities.LogNano("WebSocket连接管理", "AddConnection", utilities.WARN, "REJECTED",
				time.Since(opStart),
				fmt.Sprintf("reason=ip_limit_exceeded"),
				fmt.Sprintf("ip=%s", remoteIP),
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
	totalConnections := len(connectionManager.connections)

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

	// 获取该IP的最新连接数
	connectionManager.ipConnectionMutex.Lock()
	ipCount := connectionManager.ipConnectionCount[remoteIP]
	connectionManager.ipConnectionMutex.Unlock()

	elapsed := time.Since(opStart)
	heapMB2, sysMB2, goroutines2 := memSnapshot()

	utilities.LogVerbose(
		"WebSocket连接管理", "AddConnection",
		fmt.Sprintf("连接已成功添加: 连接ID=%s, 总连接数=%d, IP连接数=%d, 耗时=%s",
			connectionIdentifier, totalConnections, ipCount, elapsed),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("remote_addr=%s", remoteAddr),
		fmt.Sprintf("remote_ip=%s", remoteIP),
		fmt.Sprintf("total_connections=%d", totalConnections),
		fmt.Sprintf("ip_connection_count=%d", ipCount),
		fmt.Sprintf("heap_mb_before=%.2f", heapMB),
		fmt.Sprintf("heap_mb_after=%.2f", heapMB2),
		fmt.Sprintf("sys_mb=%.2f", sysMB2),
		fmt.Sprintf("goroutines_before=%d", goroutines),
		fmt.Sprintf("goroutines_after=%d", goroutines2),
		fmt.Sprintf("accepted=true"),
	)

	utilities.LogNano("WebSocket连接管理", "AddConnection", utilities.INFO, "OK",
		elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("total=%d", totalConnections),
	)

	utilities.LogSuccess("WebSocket连接管理", "AddConnection", elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("total_connections=%d", totalConnections),
	)
	return true
}

func (connectionManager *WebsokcetConnectionManager) RemoveConnection(connectionIdentifier string) {
	opStart := time.Now()
	utilities.LogStart("WebSocket连接管理", "RemoveConnection")

	connectionManager.managerMutex.Lock()

	state, exists := connectionManager.connections[connectionIdentifier]
	if exists {
		remoteAddr := state.conn.RemoteAddr().String()
		remoteIP := remoteIPFromAddr(remoteAddr)
		lastActive := state.lastActiveAt
		idleDuration := time.Since(lastActive)

		utilities.LogVerbose(
			"WebSocket连接管理", "RemoveConnection",
			fmt.Sprintf("开始移除连接: 连接ID=%s, 远程IP=%s, 空闲时长=%s", connectionIdentifier, remoteIP, idleDuration),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("remote_addr=%s", remoteAddr),
			fmt.Sprintf("remote_ip=%s", remoteIP),
			fmt.Sprintf("last_active=%s", lastActive.Format(time.RFC3339Nano)),
			fmt.Sprintf("idle_duration_ns=%d", idleDuration.Nanoseconds()),
			fmt.Sprintf("idle_duration=%s", idleDuration),
		)

		if remoteIP != "" {
			connectionManager.ipConnectionMutex.Lock()
			connectionManager.ipConnectionCount[remoteIP]--
			ipCountAfter := connectionManager.ipConnectionCount[remoteIP]
			if ipCountAfter <= 0 {
				delete(connectionManager.ipConnectionCount, remoteIP)
				ipCountAfter = 0
			}
			connectionManager.ipConnectionMutex.Unlock()

			utilities.LogVerbose(
				"WebSocket连接管理", "RemoveConnection",
				fmt.Sprintf("IP连接计数已更新: IP=%s, 剩余连接数=%d", remoteIP, ipCountAfter),
				fmt.Sprintf("remote_ip=%s", remoteIP),
				fmt.Sprintf("ip_count_after=%d", ipCountAfter),
			)
		}

		if state.heartbeatTicker != nil {
			state.heartbeatTicker.Stop()
			utilities.LogVerbose(
				"WebSocket连接管理", "RemoveConnection",
				fmt.Sprintf("心跳定时器已停止: 连接ID=%s", connectionIdentifier),
			)
		}

		delete(connectionManager.connections, connectionIdentifier)
	}

	remainingConnections := len(connectionManager.connections)
	connectionManager.managerMutex.Unlock()

	if !exists {
		utilities.LogVerbose(
			"WebSocket连接管理", "RemoveConnection",
			fmt.Sprintf("连接不存在，跳过移除: 连接ID=%s", connectionIdentifier),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("exists=false"),
		)
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

	elapsed := time.Since(opStart)
	heapMB, sysMB, goroutines := memSnapshot()

	utilities.LogVerbose(
		"WebSocket连接管理", "RemoveConnection",
		fmt.Sprintf("连接已成功移除: 连接ID=%s, 剩余连接数=%d, 耗时=%s",
			connectionIdentifier, remainingConnections, elapsed),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("remaining_connections=%d", remainingConnections),
		fmt.Sprintf("heap_mb=%.2f", heapMB),
		fmt.Sprintf("sys_mb=%.2f", sysMB),
		fmt.Sprintf("goroutines=%d", goroutines),
	)

	utilities.LogNano("WebSocket连接管理", "RemoveConnection", utilities.INFO, "OK",
		elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("remaining=%d", remainingConnections),
	)

	utilities.LogSuccess("WebSocket连接管理", "RemoveConnection", elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("remaining_connections=%d", remainingConnections),
	)
}

func (connectionManager *WebsokcetConnectionManager) BroadcastMessage(messagePayload []byte) {
	opStart := time.Now()
	utilities.LogStart("WebSocket广播", "BroadcastMessage")

	connectionManager.managerMutex.Lock()
	defer connectionManager.managerMutex.Unlock()

	recipientCount := len(connectionManager.connections)
	payloadSize := len(messagePayload)

	utilities.LogVerbose(
		"WebSocket广播", "BroadcastMessage",
		fmt.Sprintf("开始广播消息: 载荷大小=%d字节, 接收者数量=%d", payloadSize, recipientCount),
		fmt.Sprintf("payload_size_bytes=%d", payloadSize),
		fmt.Sprintf("recipient_count=%d", recipientCount),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

	successCount := 0
	failedCount := 0

	for connectionIdentifier, state := range connectionManager.connections {
		sendStart := time.Now()
		perFailed := 0

		if sendError := connectionManager.sendMessageToConnection(state.conn, messagePayload); sendError != nil {
			perFailed++
			failedCount++
			sendElapsed := time.Since(sendStart)

			utilities.LogNano("WebSocket广播", "BroadcastMessage.Send", utilities.ERROR, "FAIL",
				sendElapsed,
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("payload_size=%d", payloadSize),
				fmt.Sprintf("error=%v", sendError),
			)

			utilities.Error(
				"向连接 [%s] 广播消息失败: 载荷=%d字节, 耗时=%s, 错误=%v",
				connectionIdentifier, payloadSize, sendElapsed, sendError,
			)
			state.conn.Close()
			delete(connectionManager.connections, connectionIdentifier)
		} else {
			successCount++
			sendElapsed := time.Since(sendStart)

			utilities.LogNano("WebSocket广播", "BroadcastMessage.Send", utilities.INFO, "OK",
				sendElapsed,
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("payload_size=%d", payloadSize),
			)

			utilities.LogVerbose(
				"WebSocket广播", "BroadcastMessage.Send",
				fmt.Sprintf("单连接发送成功: 连接ID=%s, 耗时=%s", connectionIdentifier, sendElapsed),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("send_elapsed_ns=%d", sendElapsed.Nanoseconds()),
			)
		}

		connectionManager.recordEvent(
			context.Background(),
			connectionIdentifier,
			events.EventTypeChatMessageBroadcasted,
			events.ChatBroadcastEventData{
				ConnectionIdentifier: connectionIdentifier,
				RecipientCount:       recipientCount,
				PayloadSizeBytes:     len(messagePayload),
				FailedCount:          perFailed,
			},
			map[string]string{"component": "connections"},
		)
	}

	totalElapsed := time.Since(opStart)

	utilities.LogVerbose(
		"WebSocket广播", "BroadcastMessage",
		fmt.Sprintf("广播完成: 成功=%d, 失败=%d, 总接收者=%d, 载荷=%d字节, 总耗时=%s",
			successCount, failedCount, recipientCount, payloadSize, totalElapsed),
		fmt.Sprintf("success_count=%d", successCount),
		fmt.Sprintf("failed_count=%d", failedCount),
		fmt.Sprintf("recipient_count=%d", recipientCount),
		fmt.Sprintf("payload_size_bytes=%d", payloadSize),
		fmt.Sprintf("total_elapsed_ns=%d", totalElapsed.Nanoseconds()),
	)

	utilities.LogNano("WebSocket广播", "BroadcastMessage", utilities.INFO, "OK",
		totalElapsed,
		fmt.Sprintf("success=%d", successCount),
		fmt.Sprintf("failed=%d", failedCount),
		fmt.Sprintf("total=%d", recipientCount),
	)

	utilities.LogSuccess("WebSocket广播", "BroadcastMessage", totalElapsed,
		fmt.Sprintf("payload_size=%d", payloadSize),
		fmt.Sprintf("recipients=%d", recipientCount),
	)
}

func (connectionManager *WebsokcetConnectionManager) SendMessageToConnection(
	connectionIdentifier string,
	messagePayload []byte,
) error {
	opStart := time.Now()
	payloadSize := len(messagePayload)

	utilities.LogVerbose(
		"WebSocket发送", "SendMessageToConnection",
		fmt.Sprintf("准备向指定连接发送消息: 连接ID=%s, 载荷大小=%d字节", connectionIdentifier, payloadSize),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("payload_size_bytes=%d", payloadSize),
		fmt.Sprintf("write_deadline=%s", connectionManager.writeTimeout),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

	connectionManager.managerMutex.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	connectionManager.managerMutex.Unlock()

	if !exists {
		elapsed := time.Since(opStart)
		utilities.LogNano("WebSocket发送", "SendMessageToConnection", utilities.ERROR, "FAIL",
			elapsed,
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("reason=connection_not_found"),
		)
		return fmt.Errorf("连接不存在: %s", connectionIdentifier)
	}

	err := connectionManager.sendMessageToConnection(state.conn, messagePayload)
	elapsed := time.Since(opStart)

	if err != nil {
		utilities.LogNano("WebSocket发送", "SendMessageToConnection", utilities.ERROR, "FAIL",
			elapsed,
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("payload_size=%d", payloadSize),
			fmt.Sprintf("error=%v", err),
		)
	} else {
		utilities.LogNano("WebSocket发送", "SendMessageToConnection", utilities.INFO, "OK",
			elapsed,
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("payload_size=%d", payloadSize),
		)
	}

	return err
}

func (connectionManager *WebsokcetConnectionManager) SendMessage(
	connectionID string,
	payload []byte,
) error {
	utilities.LogVerbose(
		"WebSocket发送", "SendMessage",
		fmt.Sprintf("SendMessage 代理调用: 连接ID=%s, 载荷大小=%d字节", connectionID, len(payload)),
		fmt.Sprintf("connection_id=%s", connectionID),
		fmt.Sprintf("payload_size_bytes=%d", len(payload)),
	)
	return connectionManager.SendMessageToConnection(connectionID, payload)
}

func (connectionManager *WebsokcetConnectionManager) sendMessageToConnection(conn *websocket.Conn, payload []byte) error {
	deadline := time.Now().Add(connectionManager.writeTimeout)
	conn.SetWriteDeadline(deadline)

	utilities.LogVerbose(
		"WebSocket发送", "sendMessageToConnection",
		fmt.Sprintf("底层写入: 载荷=%d字节, 写入截止时间=%s, 超时=%s",
			len(payload), deadline.Format(time.RFC3339Nano), connectionManager.writeTimeout),
		fmt.Sprintf("payload_size_bytes=%d", len(payload)),
		fmt.Sprintf("write_deadline=%s", deadline.Format(time.RFC3339Nano)),
		fmt.Sprintf("write_timeout=%s", connectionManager.writeTimeout),
		fmt.Sprintf("remote_addr=%s", conn.RemoteAddr().String()),
	)

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
	opStart := time.Now()
	latencyFromSender := time.Since(sentAt)

	utilities.LogVerbose(
		"WebSocket心跳", "handlePing",
		fmt.Sprintf("收到 Ping: 连接ID=%s, Nonce=%s, 发送时间=%s, 传输延迟=%s (%dns)",
			connectionIdentifier, nonce, sentAt.Format(time.RFC3339Nano), latencyFromSender, latencyFromSender.Nanoseconds()),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("nonce=%s", nonce),
		fmt.Sprintf("sent_at=%s", sentAt.Format(time.RFC3339Nano)),
		fmt.Sprintf("sender_latency_ns=%d", latencyFromSender.Nanoseconds()),
		fmt.Sprintf("sender_latency=%s", latencyFromSender),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

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
		utilities.LogNano("WebSocket心跳", "handlePing", utilities.ERROR, "FAIL",
			time.Since(opStart),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("nonce=%s", nonce),
			fmt.Sprintf("error=构建pong消息失败: %v", err),
		)
		utilities.Error("构建 pong 消息失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	if err := connectionManager.SendMessageToConnection(connectionIdentifier, pongMessage); err != nil {
		utilities.LogNano("WebSocket心跳", "handlePing", utilities.ERROR, "FAIL",
			time.Since(opStart),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("nonce=%s", nonce),
			fmt.Sprintf("error=发送pong失败: %v", err),
		)
		utilities.Error("发送 pong 失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	latency := time.Since(sentAt)
	elapsed := time.Since(opStart)

	connectionManager.recordEvent(
		context.Background(),
		connectionIdentifier,
		events.EventTypeHeartbeatPongSent,
		events.HeartbeatEventData{
			ConnectionIdentifier: connectionIdentifier,
			Nonce:                nonce,
			LatencyMs:            latency.Milliseconds(),
			Action:               "pong",
		},
		map[string]string{"component": "connections"},
	)

	utilities.LogNano("WebSocket心跳", "handlePing", utilities.INFO, "OK",
		elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("nonce=%s", nonce),
		fmt.Sprintf("round_trip_latency_ns=%d", latency.Nanoseconds()),
		fmt.Sprintf("round_trip_latency_ms=%d", latency.Milliseconds()),
		fmt.Sprintf("processing_time_ns=%d", elapsed.Nanoseconds()),
	)

	utilities.LogVerbose(
		"WebSocket心跳", "handlePing",
		fmt.Sprintf("Ping处理完成并已回复Pong: 连接ID=%s, Nonce=%s, 往返延迟=%dns, 处理耗时=%dns",
			connectionIdentifier, nonce, latency.Nanoseconds(), elapsed.Nanoseconds()),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("nonce=%s", nonce),
		fmt.Sprintf("latency_ns=%d", latency.Nanoseconds()),
	)
}

func (connectionManager *WebsokcetConnectionManager) handlePong(connectionIdentifier string, nonce string) {
	opStart := time.Now()

	utilities.LogVerbose(
		"WebSocket心跳", "handlePong",
		fmt.Sprintf("收到 Pong: 连接ID=%s, Nonce=%s", connectionIdentifier, nonce),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("nonce=%s", nonce),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

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
		roundTripLatency := time.Since(state.pingSentAt)

		utilities.LogNano("WebSocket心跳", "handlePong", utilities.INFO, "NONCE_MATCH",
			roundTripLatency,
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("nonce=%s", nonce),
			fmt.Sprintf("nonce_match=true"),
			fmt.Sprintf("round_trip_ns=%d", roundTripLatency.Nanoseconds()),
			fmt.Sprintf("round_trip_ms=%d", roundTripLatency.Milliseconds()),
			fmt.Sprintf("ping_sent_at=%s", state.pingSentAt.Format(time.RFC3339Nano)),
		)

		utilities.LogVerbose(
			"WebSocket心跳", "handlePong",
			fmt.Sprintf("Pong Nonce匹配成功: 连接ID=%s, Nonce=%s, 往返延迟=%dns (%dms)",
				connectionIdentifier, nonce, roundTripLatency.Nanoseconds(), roundTripLatency.Milliseconds()),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("nonce=%s", nonce),
			fmt.Sprintf("round_trip_latency_ns=%d", roundTripLatency.Nanoseconds()),
		)

		state.connectionMutex.Lock()
		state.pingNonce = ""
		state.connectionMutex.Unlock()
	} else if exists {
		utilities.LogWarn(
			"WebSocket心跳", "handlePong",
			fmt.Sprintf("Pong Nonce不匹配: 连接ID=%s, 期望=%s, 收到=%s", connectionIdentifier, state.pingNonce, nonce),
			time.Since(opStart),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("expected_nonce=%s", state.pingNonce),
			fmt.Sprintf("received_nonce=%s", nonce),
			fmt.Sprintf("nonce_match=false"),
		)
	} else {
		utilities.LogVerbose(
			"WebSocket心跳", "handlePong",
			fmt.Sprintf("Pong对应连接不存在: 连接ID=%s, Nonce=%s", connectionIdentifier, nonce),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("exists=false"),
		)
	}

	utilities.LogNano("WebSocket心跳", "handlePong", utilities.INFO, "OK",
		time.Since(opStart),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
	)
}

func (connectionManager *WebsokcetConnectionManager) sendPing(connectionIdentifier string) {
	opStart := time.Now()

	utilities.LogVerbose(
		"WebSocket心跳", "sendPing",
		fmt.Sprintf("准备发送 Ping: 连接ID=%s", connectionIdentifier),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

	connectionManager.managerMutex.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	if !exists {
		connectionManager.managerMutex.Unlock()
		utilities.LogVerbose(
			"WebSocket心跳", "sendPing",
			fmt.Sprintf("连接不存在，跳过Ping: 连接ID=%s", connectionIdentifier),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("exists=false"),
		)
		return
	}

	if state.pingNonce != "" {
		connectionManager.managerMutex.Unlock()
		utilities.LogVerbose(
			"WebSocket心跳", "sendPing",
			fmt.Sprintf("上一个Ping尚未收到Pong，跳过: 连接ID=%s, 待回复Nonce=%s", connectionIdentifier, state.pingNonce),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("pending_nonce=%s", state.pingNonce),
			fmt.Sprintf("skipped=true"),
		)
		return
	}

	nonceBytes := make([]byte, 16)
	if _, randErr := rand.Read(nonceBytes); randErr != nil {
		connectionManager.managerMutex.Unlock()
		utilities.LogNano("WebSocket心跳", "sendPing", utilities.ERROR, "FAIL",
			time.Since(opStart),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("error=随机数生成失败: %v", randErr),
		)
		utilities.Error("生成心跳随机数失败 [%s]: %v", connectionIdentifier, randErr)
		return
	}
	nonce := hex.EncodeToString(nonceBytes)
	state.pingNonce = nonce
	state.pingSentAt = time.Now()
	state.lastActiveAt = time.Now()
	websocketConnection := state.conn
	connectionManager.managerMutex.Unlock()

	utilities.LogVerbose(
		"WebSocket心跳", "sendPing",
		fmt.Sprintf("Nonce已生成: 连接ID=%s, Nonce=%s, 目标远程地址=%s",
			connectionIdentifier, nonce, websocketConnection.RemoteAddr().String()),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("nonce=%s", nonce),
		fmt.Sprintf("target_remote_addr=%s", websocketConnection.RemoteAddr().String()),
		fmt.Sprintf("nonce_generated_at_nano=%d", time.Now().UnixNano()),
	)

	pingMessage, err := buildHeartbeatPing(nonce)
	if err != nil {
		utilities.LogNano("WebSocket心跳", "sendPing", utilities.ERROR, "FAIL",
			time.Since(opStart),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("nonce=%s", nonce),
			fmt.Sprintf("error=构建ping消息失败: %v", err),
		)
		utilities.Error("构建 ping 消息失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	if err := connectionManager.sendMessageToConnection(websocketConnection, pingMessage); err != nil {
		utilities.LogNano("WebSocket心跳", "sendPing", utilities.ERROR, "FAIL",
			time.Since(opStart),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("nonce=%s", nonce),
			fmt.Sprintf("error=发送ping失败: %v", err),
		)
		utilities.Error("发送 ping 失败 [%s]: %v", connectionIdentifier, err)
		return
	}

	elapsed := time.Since(opStart)

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

	utilities.LogNano("WebSocket心跳", "sendPing", utilities.INFO, "OK",
		elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("nonce=%s", nonce),
		fmt.Sprintf("ping_size_bytes=%d", len(pingMessage)),
	)

	utilities.LogVerbose(
		"WebSocket心跳", "sendPing",
		fmt.Sprintf("Ping已发送: 连接ID=%s, Nonce=%s, 消息大小=%d字节, 耗时=%s",
			connectionIdentifier, nonce, len(pingMessage), elapsed),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("nonce=%s", nonce),
		fmt.Sprintf("ping_size_bytes=%d", len(pingMessage)),
	)
}

func (connectionManager *WebsokcetConnectionManager) startConnectionHeartbeat(connectionIdentifier string, state *connectionState) {
	utilities.LogVerbose(
		"WebSocket心跳", "startConnectionHeartbeat",
		fmt.Sprintf("心跳循环启动: 连接ID=%s, 间隔=%s, 超时=%s",
			connectionIdentifier, connectionManager.heartbeatInterval, connectionManager.heartbeatTimeout),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("heartbeat_interval=%s", connectionManager.heartbeatInterval),
		fmt.Sprintf("heartbeat_timeout=%s", connectionManager.heartbeatTimeout),
		fmt.Sprintf("started_at_nano=%d", time.Now().UnixNano()),
	)

	state.heartbeatTicker = time.NewTicker(connectionManager.heartbeatInterval)
	defer func() {
		state.heartbeatTicker.Stop()
		utilities.LogVerbose(
			"WebSocket心跳", "startConnectionHeartbeat",
			fmt.Sprintf("心跳循环已停止: 连接ID=%s", connectionIdentifier),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("stopped_at_nano=%d", time.Now().UnixNano()),
		)
	}()

	cycleCount := 0
	for {
		select {
		case <-state.heartbeatTicker.C:
			cycleCount++
			cycleStart := time.Now()

			connectionManager.managerMutex.Lock()
			_, exists := connectionManager.connections[connectionIdentifier]
			connectionManager.managerMutex.Unlock()

			if !exists {
				utilities.LogVerbose(
					"WebSocket心跳", "startConnectionHeartbeat",
					fmt.Sprintf("连接已不存在，退出心跳循环: 连接ID=%s, 已完成周期数=%d", connectionIdentifier, cycleCount),
					fmt.Sprintf("connection_id=%s", connectionIdentifier),
					fmt.Sprintf("cycle_count=%d", cycleCount),
				)
				return
			}

			utilities.LogVerbose(
				"WebSocket心跳", "startConnectionHeartbeat",
				fmt.Sprintf("心跳周期 #%d 开始: 连接ID=%s", cycleCount, connectionIdentifier),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("cycle=%d", cycleCount),
				fmt.Sprintf("cycle_start_nano=%d", cycleStart.UnixNano()),
			)

			connectionManager.sendPing(connectionIdentifier)

			select {
			case <-time.After(connectionManager.heartbeatTimeout):
				connectionManager.managerMutex.Lock()
				s, ok := connectionManager.connections[connectionIdentifier]
				if ok && s.pingNonce != "" {
					connectionManager.managerMutex.Unlock()
					utilities.LogVerbose(
						"WebSocket心跳", "startConnectionHeartbeat",
						fmt.Sprintf("心跳超时，Pong未收到: 连接ID=%s, 周期=%d, 待回复Nonce=%s",
							connectionIdentifier, cycleCount, s.pingNonce),
						fmt.Sprintf("connection_id=%s", connectionIdentifier),
						fmt.Sprintf("cycle=%d", cycleCount),
						fmt.Sprintf("pending_nonce=%s", s.pingNonce),
					)
					connectionManager.handleHeartbeatTimeout(connectionIdentifier)
					return
				}
				connectionManager.managerMutex.Unlock()
			case <-connectionManager.ctx.Done():
				utilities.LogVerbose(
					"WebSocket心跳", "startConnectionHeartbeat",
					fmt.Sprintf("上下文取消，退出心跳循环: 连接ID=%s", connectionIdentifier),
					fmt.Sprintf("connection_id=%s", connectionIdentifier),
				)
				return
			}
		case <-connectionManager.ctx.Done():
			utilities.LogVerbose(
				"WebSocket心跳", "startConnectionHeartbeat",
				fmt.Sprintf("上下文取消（外层），退出心跳循环: 连接ID=%s, 已完成周期数=%d", connectionIdentifier, cycleCount),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("cycle_count=%d", cycleCount),
			)
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
	opStart := time.Now()
	connectionManager.managerMutex.Lock()
	defer connectionManager.managerMutex.Unlock()

	now := time.Now()
	totalConnections := len(connectionManager.connections)

	utilities.LogVerbose(
		"WebSocket心跳监控", "checkHeartbeatTimeouts",
		fmt.Sprintf("开始检查心跳超时: 连接总数=%d, 超时阈值=%s", totalConnections, connectionManager.heartbeatTimeout),
		fmt.Sprintf("total_connections=%d", totalConnections),
		fmt.Sprintf("timeout_threshold=%s", connectionManager.heartbeatTimeout),
		fmt.Sprintf("check_time_nano=%d", now.UnixNano()),
	)

	timedOutCount := 0
	for connectionIdentifier, state := range connectionManager.connections {
		state.connectionMutex.Lock()
		lastActive := state.lastActiveAt
		state.connectionMutex.Unlock()

		idleDuration := now.Sub(lastActive)
		isTimedOut := idleDuration > connectionManager.heartbeatTimeout

		utilities.LogVerbose(
			"WebSocket心跳监控", "checkHeartbeatTimeouts",
			fmt.Sprintf("连接心跳检查: 连接ID=%s, 空闲时长=%s (%dns), 阈值=%s, 超时=%v",
				connectionIdentifier, idleDuration, idleDuration.Nanoseconds(), connectionManager.heartbeatTimeout, isTimedOut),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("last_active=%s", lastActive.Format(time.RFC3339Nano)),
			fmt.Sprintf("idle_duration_ns=%d", idleDuration.Nanoseconds()),
			fmt.Sprintf("idle_duration=%s", idleDuration),
			fmt.Sprintf("threshold=%s", connectionManager.heartbeatTimeout),
			fmt.Sprintf("timed_out=%v", isTimedOut),
		)

		if isTimedOut {
			timedOutCount++
			go connectionManager.handleHeartbeatTimeout(connectionIdentifier)
		}
	}

	elapsed := time.Since(opStart)
	utilities.LogNano("WebSocket心跳监控", "checkHeartbeatTimeouts", utilities.INFO, "OK",
		elapsed,
		fmt.Sprintf("total_checked=%d", totalConnections),
		fmt.Sprintf("timed_out=%d", timedOutCount),
	)
}

func (connectionManager *WebsokcetConnectionManager) handleHeartbeatTimeout(connectionIdentifier string) {
	opStart := time.Now()

	utilities.LogVerbose(
		"WebSocket心跳", "handleHeartbeatTimeout",
		fmt.Sprintf("处理心跳超时: 连接ID=%s, 超时阈值=%s", connectionIdentifier, connectionManager.heartbeatTimeout),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("timeout_threshold=%s", connectionManager.heartbeatTimeout),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

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

	connectionManager.managerMutex.Lock()
	state, exists := connectionManager.connections[connectionIdentifier]
	if exists {
		remoteAddr := state.conn.RemoteAddr().String()
		lastActive := state.lastActiveAt
		idleDuration := time.Since(lastActive)

		utilities.LogVerbose(
			"WebSocket心跳", "handleHeartbeatTimeout",
			fmt.Sprintf("正在清理超时连接: 连接ID=%s, 远程地址=%s, 最后活跃=%s, 空闲=%s",
				connectionIdentifier, remoteAddr, lastActive.Format(time.RFC3339Nano), idleDuration),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("remote_addr=%s", remoteAddr),
			fmt.Sprintf("last_active=%s", lastActive.Format(time.RFC3339Nano)),
			fmt.Sprintf("idle_duration_ns=%d", idleDuration.Nanoseconds()),
			fmt.Sprintf("idle_duration=%s", idleDuration),
		)

		state.conn.Close()
		delete(connectionManager.connections, connectionIdentifier)
	}
	remainingConnections := len(connectionManager.connections)
	connectionManager.managerMutex.Unlock()

	elapsed := time.Since(opStart)

	utilities.LogNano("WebSocket心跳", "handleHeartbeatTimeout", utilities.WARN, "TIMEOUT",
		elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("remaining_connections=%d", remainingConnections),
		fmt.Sprintf("cleaned_up=%v", exists),
	)

	utilities.LogWarn(
		"WebSocket心跳", "handleHeartbeatTimeout",
		fmt.Sprintf("连接心跳超时已处理: 连接ID=%s, 已清理=%v, 剩余连接=%d, 耗时=%s",
			connectionIdentifier, exists, remainingConnections, elapsed),
		elapsed,
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("remaining_connections=%d", remainingConnections),
	)
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

	utilities.LogVerbose(
		"WebSocket路由", "RegisterWebSocketHandlers",
		fmt.Sprintf("注册 WebSocket 处理器: 路径前缀=%s, 心跳间隔=%s, 心跳超时=%s, 写入超时=%s, 最大帧=%d字节",
			WebSocketChatEndpointPathPrefix, connectionManager.heartbeatInterval,
			connectionManager.heartbeatTimeout, connectionManager.writeTimeout, defaultMaxFrameBytes),
		fmt.Sprintf("path_prefix=%s", WebSocketChatEndpointPathPrefix),
		fmt.Sprintf("heartbeat_interval=%s", connectionManager.heartbeatInterval),
		fmt.Sprintf("heartbeat_timeout=%s", connectionManager.heartbeatTimeout),
		fmt.Sprintf("write_timeout=%s", connectionManager.writeTimeout),
		fmt.Sprintf("max_frame_bytes=%d", defaultMaxFrameBytes),
		fmt.Sprintf("has_message_handler=%v", messageHandler != nil),
		fmt.Sprintf("has_chat_streamer=%v", chatStreamer != nil),
		fmt.Sprintf("has_connection_notifier=%v", connectionNotifier != nil),
	)

	websocketServeMux.HandleFunc(WebSocketChatEndpointPathPrefix, func(
		responseWriter http.ResponseWriter,
		httpRequest *http.Request,
	) {
		upgradeStart := time.Now()

		// 记录每个 WebSocket 升级请求的完整详情
		utilities.LogVerbose(
			"WebSocket路由", "RegisterWebSocketHandlers.HandleFunc",
			fmt.Sprintf("收到 WebSocket 升级请求: 方法=%s, 路径=%s, 远程地址=%s, 请求头数量=%d, Origin=%s",
				httpRequest.Method, httpRequest.URL.Path, httpRequest.RemoteAddr,
				len(httpRequest.Header), httpRequest.Header.Get("Origin")),
			fmt.Sprintf("method=%s", httpRequest.Method),
			fmt.Sprintf("path=%s", httpRequest.URL.Path),
			fmt.Sprintf("remote_addr=%s", httpRequest.RemoteAddr),
			fmt.Sprintf("headers_count=%d", len(httpRequest.Header)),
			fmt.Sprintf("origin=%s", httpRequest.Header.Get("Origin")),
			fmt.Sprintf("user_agent=%s", httpRequest.Header.Get("User-Agent")),
			fmt.Sprintf("host=%s", httpRequest.Host),
			fmt.Sprintf("proto=%s", httpRequest.Proto),
			fmt.Sprintf("content_length=%d", httpRequest.ContentLength),
			fmt.Sprintf("timestamp_nano=%d", upgradeStart.UnixNano()),
		)

		connectionIdentifier, connectionIdentifierError := chatConnectionIdentifierFromRequest(httpRequest)
		if connectionIdentifierError != nil {
			utilities.LogNano("WebSocket路由", "RegisterWebSocketHandlers.HandleFunc", utilities.ERROR, "BAD_REQUEST",
				time.Since(upgradeStart),
				fmt.Sprintf("error=%v", connectionIdentifierError),
				fmt.Sprintf("path=%s", httpRequest.URL.Path),
				fmt.Sprintf("remote_addr=%s", httpRequest.RemoteAddr),
			)
			http.Error(responseWriter, connectionIdentifierError.Error(), http.StatusBadRequest)
			return
		}

		authenticatedUserID, authOK := authenticateWebSocketUpgrade(httpRequest)
		if !authOK {
			utilities.LogWarn(
				"WebSocket路由",
				"RegisterWebSocketHandlers.HandleFunc",
				fmt.Sprintf("WebSocket 升级鉴权失败，拒绝连接 [%s]", connectionIdentifier),
				time.Since(upgradeStart),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
				fmt.Sprintf("origin=%s", httpRequest.Header.Get("Origin")),
			)
			http.Error(responseWriter, "unauthorized", http.StatusUnauthorized)
			return
		}

		websocketConnection, upgradeError := upgrader.Upgrade(responseWriter, httpRequest, nil)
		if upgradeError != nil {
			utilities.LogNano("WebSocket路由", "RegisterWebSocketHandlers.HandleFunc", utilities.ERROR, "UPGRADE_FAIL",
				time.Since(upgradeStart),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("error=%v", upgradeError),
			)
			utilities.Error("无法升级 HTTP 请求为 WebSocket 连接: %v", upgradeError)
			return
		}

		// Enforce a hard cap on incoming frame size to prevent OOM via giant frames.
		websocketConnection.SetReadLimit(defaultMaxFrameBytes)

		utilities.LogNano("WebSocket路由", "RegisterWebSocketHandlers.HandleFunc", utilities.INFO, "UPGRADED",
			time.Since(upgradeStart),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("user_id=%s", authenticatedUserID),
			fmt.Sprintf("remote_addr=%s", httpRequest.RemoteAddr),
		)

		accepted := connectionManager.AddConnection(connectionIdentifier, websocketConnection)
		if !accepted {
			utilities.LogWarn(
				"WebSocket路由", "RegisterWebSocketHandlers.HandleFunc",
				fmt.Sprintf("连接被拒绝（超出限制）: 连接ID=%s", connectionIdentifier),
				time.Since(upgradeStart),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
			)
			_ = websocketConnection.Close()
			http.Error(responseWriter, "too many connections", http.StatusTooManyRequests)
			return
		}
		defer func() {
			connectionManager.RemoveConnection(connectionIdentifier)
			_ = websocketConnection.Close()
		}()

		utilities.LogVerbose(
			"WebSocket路由", "RegisterWebSocketHandlers.HandleFunc",
			fmt.Sprintf("WebSocket 连接已建立: 连接ID=%s, 用户=%s, 远程地址=%s, 升级耗时=%s",
				connectionIdentifier, authenticatedUserID, httpRequest.RemoteAddr, time.Since(upgradeStart)),
			fmt.Sprintf("connection_id=%s", connectionIdentifier),
			fmt.Sprintf("user_id=%s", authenticatedUserID),
			fmt.Sprintf("remote_addr=%s", httpRequest.RemoteAddr),
			fmt.Sprintf("upgrade_elapsed_ns=%d", time.Since(upgradeStart).Nanoseconds()),
		)

		if connectionNotifier != nil {
			go connectionNotifier.OnConnectionReady(httpRequest.Context(), connectionIdentifier)
		}

		readDeadline := 2 * connectionManager.heartbeatTimeout
		messageCount := 0
		for {
			_ = websocketConnection.SetReadDeadline(time.Now().Add(readDeadline))
			readStart := time.Now()
			messageType, messagePayload, readError := websocketConnection.ReadMessage()
			readElapsed := time.Since(readStart)

			if readError != nil {
				if isExpectedCloseError(readError) {
					utilities.LogVerbose(
						"WebSocket读取", "ReadMessage",
						fmt.Sprintf("连接正常关闭: 连接ID=%s, 用户=%s, 已处理消息数=%d",
							connectionIdentifier, authenticatedUserID, messageCount),
						fmt.Sprintf("connection_id=%s", connectionIdentifier),
						fmt.Sprintf("user_id=%s", authenticatedUserID),
						fmt.Sprintf("messages_processed=%d", messageCount),
						fmt.Sprintf("close_type=normal"),
					)
				} else {
					utilities.LogWarn(
						"WebSocket读取",
						"ReadMessage",
						fmt.Sprintf("连接异常关闭: 连接ID=%s, 错误=%v, 已处理消息数=%d",
							connectionIdentifier, readError, messageCount),
						readElapsed,
						fmt.Sprintf("connection_id=%s", connectionIdentifier),
						fmt.Sprintf("messages_processed=%d", messageCount),
						fmt.Sprintf("close_type=abnormal"),
					)
				}
				return
			}

			messageCount++
			payloadSize := len(messagePayload)

			if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
				utilities.LogVerbose(
					"WebSocket读取", "ReadMessage",
					fmt.Sprintf("忽略非文本/二进制消息: 连接ID=%s, 消息类型=%d", connectionIdentifier, messageType),
					fmt.Sprintf("connection_id=%s", connectionIdentifier),
					fmt.Sprintf("message_type=%d", messageType),
					fmt.Sprintf("skipped=true"),
				)
				continue
			}

			// 判断消息处理路径
			isHeartbeat := isHeartbeatMessage(messagePayload)
			processingPath := "broadcast"
			if isHeartbeat {
				processingPath = "heartbeat"
			} else if chatStreamer != nil {
				processingPath = "streaming"
			} else if messageHandler != nil {
				processingPath = "handler"
			}

			utilities.LogVerbose(
				"WebSocket读取", "ReadMessage",
				fmt.Sprintf("收到消息 #%d: 连接ID=%s, 类型=%d, 大小=%d字节, 读取耗时=%s, 处理路径=%s",
					messageCount, connectionIdentifier, messageType, payloadSize, readElapsed, processingPath),
				fmt.Sprintf("connection_id=%s", connectionIdentifier),
				fmt.Sprintf("message_number=%d", messageCount),
				fmt.Sprintf("message_type=%d", messageType),
				fmt.Sprintf("payload_size_bytes=%d", payloadSize),
				fmt.Sprintf("read_elapsed_ns=%d", readElapsed.Nanoseconds()),
				fmt.Sprintf("processing_path=%s", processingPath),
				fmt.Sprintf("is_heartbeat=%v", isHeartbeat),
			)

			if isHeartbeat {
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
				streamStart := time.Now()
				handled := chatStreamer.HandleUserChatWithStreaming(
					httpRequest.Context(), connectionIdentifier, messagePayload,
				)
				streamElapsed := time.Since(streamStart)

				utilities.LogVerbose(
					"WebSocket读取", "ReadMessage.Streaming",
					fmt.Sprintf("流式处理结果: 连接ID=%s, 已处理=%v, 耗时=%s",
						connectionIdentifier, handled, streamElapsed),
					fmt.Sprintf("connection_id=%s", connectionIdentifier),
					fmt.Sprintf("handled=%v", handled),
					fmt.Sprintf("stream_elapsed_ns=%d", streamElapsed.Nanoseconds()),
				)

				if handled {
					continue
				}
			}

			if messageHandler != nil {
				handlerStart := time.Now()
				responsePayload, handleError := messageHandler.HandleIncomingMessage(
					httpRequest.Context(), messagePayload,
				)
				handlerElapsed := time.Since(handlerStart)

				if handleError != nil {
					utilities.LogNano("WebSocket读取", "ReadMessage.Handler", utilities.ERROR, "FAIL",
						handlerElapsed,
						fmt.Sprintf("connection_id=%s", connectionIdentifier),
						fmt.Sprintf("payload_size=%d", payloadSize),
						fmt.Sprintf("error=%v", handleError),
					)

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

				utilities.LogNano("WebSocket读取", "ReadMessage.Handler", utilities.INFO, "OK",
					handlerElapsed,
					fmt.Sprintf("connection_id=%s", connectionIdentifier),
					fmt.Sprintf("request_size=%d", payloadSize),
					fmt.Sprintf("response_size=%d", len(responsePayload)),
				)

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
				utilities.LogVerbose(
					"WebSocket读取", "ReadMessage",
					fmt.Sprintf("无处理器，直接广播原始消息: 连接ID=%s, 大小=%d字节", connectionIdentifier, payloadSize),
					fmt.Sprintf("connection_id=%s", connectionIdentifier),
					fmt.Sprintf("payload_size=%d", payloadSize),
					fmt.Sprintf("path=direct_broadcast"),
				)
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

	persistedEvent, appendError := connectionManager.eventStore.Append(runtimeContext, domainEvent)
	if appendError != nil {
		utilities.Error("追加事件失败 [%s]: %v", eventType, appendError)
		return
	}

	// 记录事件追加详情
	utilities.LogVerbose(
		"WebSocket事件", "recordEvent",
		fmt.Sprintf("事件已记录: 类型=%s, 流ID=%s, 事件ID=%s, 连接ID=%s",
			eventType, streamIdentifier, persistedEvent.EventID, connectionIdentifier),
		fmt.Sprintf("event_type=%s", eventType),
		fmt.Sprintf("stream_id=%s", streamIdentifier),
		fmt.Sprintf("event_id=%s", persistedEvent.EventID),
		fmt.Sprintf("connection_id=%s", connectionIdentifier),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)
}

func isWebSocketOriginAllowed(httpRequest *http.Request) bool {
	opStart := time.Now()

	if strings.EqualFold(os.Getenv("WSS_ALLOW_ALL_ORIGINS"), "true") {
		utilities.LogWarn(
			"WebSocket安全", "isWebSocketOriginAllowed",
			"WSS_ALLOW_ALL_ORIGINS=true 已启用，接受所有 Origin（仅限本地调试）",
			0,
		)
		utilities.LogVerbose(
			"WebSocket安全", "isWebSocketOriginAllowed",
			"全部 Origin 放行模式已启用",
			fmt.Sprintf("allow_all=true"),
			fmt.Sprintf("result=allowed"),
		)
		return true
	}

	if !utilities.IsProductionMode() {
		requestOrigin := strings.TrimSpace(httpRequest.Header.Get("Origin"))
		utilities.LogVerbose(
			"WebSocket安全", "isWebSocketOriginAllowed",
			fmt.Sprintf("开发模式 Origin 检查: Origin=%s, 结果=允许", requestOrigin),
			fmt.Sprintf("origin=%s", requestOrigin),
			fmt.Sprintf("mode=development"),
			fmt.Sprintf("result=allowed"),
			fmt.Sprintf("elapsed_ns=%d", time.Since(opStart).Nanoseconds()),
		)
		return true
	}

	requestOrigin := strings.TrimSpace(httpRequest.Header.Get("Origin"))
	if requestOrigin == "" {
		utilities.LogVerbose(
			"WebSocket安全", "isWebSocketOriginAllowed",
			"Origin 为空，拒绝连接",
			fmt.Sprintf("origin=<empty>"),
			fmt.Sprintf("result=denied"),
		)
		return false
	}

	allowedOriginsList := os.Getenv("WSS_ALLOWED_ORIGINS")
	if allowedOriginsList == "" {
		utilities.LogVerbose(
			"WebSocket安全", "isWebSocketOriginAllowed",
			fmt.Sprintf("未配置允许的 Origin 列表，拒绝: Origin=%s", requestOrigin),
			fmt.Sprintf("origin=%s", requestOrigin),
			fmt.Sprintf("allowed_origins=<not_set>"),
			fmt.Sprintf("result=denied"),
		)
		return false
	}

	allowedList := strings.Split(allowedOriginsList, ",")
	for _, allowedOrigin := range allowedList {
		if strings.EqualFold(strings.TrimSpace(allowedOrigin), requestOrigin) {
			utilities.LogVerbose(
				"WebSocket安全", "isWebSocketOriginAllowed",
				fmt.Sprintf("Origin 匹配成功: Origin=%s, 匹配项=%s", requestOrigin, strings.TrimSpace(allowedOrigin)),
				fmt.Sprintf("origin=%s", requestOrigin),
				fmt.Sprintf("matched=%s", strings.TrimSpace(allowedOrigin)),
				fmt.Sprintf("allowed_count=%d", len(allowedList)),
				fmt.Sprintf("result=allowed"),
				fmt.Sprintf("elapsed_ns=%d", time.Since(opStart).Nanoseconds()),
			)
			return true
		}
	}

	utilities.LogVerbose(
		"WebSocket安全", "isWebSocketOriginAllowed",
		fmt.Sprintf("Origin 不在允许列表中: Origin=%s, 允许列表=%s", requestOrigin, allowedOriginsList),
		fmt.Sprintf("origin=%s", requestOrigin),
		fmt.Sprintf("allowed_origins=%s", allowedOriginsList),
		fmt.Sprintf("allowed_count=%d", len(allowedList)),
		fmt.Sprintf("result=denied"),
		fmt.Sprintf("elapsed_ns=%d", time.Since(opStart).Nanoseconds()),
	)
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

// isExpectedCloseError 判断 WebSocket 关闭是否为正常原因
// （客户端断开、正常关闭握手、空闲超时）。
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
func authenticateWebSocketUpgrade(httpRequest *http.Request) (string, bool) {
	opStart := time.Now()

	token := strings.TrimSpace(httpRequest.URL.Query().Get("token"))
	if token == "" {
		authHeader := strings.TrimSpace(httpRequest.Header.Get("Authorization"))
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}

	tokenPresent := token != ""
	mode := "production"
	if !utilities.IsProductionMode() {
		mode = "development"
	}

	utilities.LogVerbose(
		"WebSocket认证", "authenticateWebSocketUpgrade",
		fmt.Sprintf("认证尝试: 远程地址=%s, Token存在=%v, 模式=%s, Token=%s",
			httpRequest.RemoteAddr, tokenPresent, mode, utilities.Mask(token)),
		fmt.Sprintf("remote_addr=%s", httpRequest.RemoteAddr),
		fmt.Sprintf("token_present=%v", tokenPresent),
		fmt.Sprintf("token_masked=%s", utilities.Mask(token)),
		fmt.Sprintf("mode=%s", mode),
		fmt.Sprintf("timestamp_nano=%d", time.Now().UnixNano()),
	)

	if !utilities.IsProductionMode() {
		if token == "" {
			utilities.LogWarn(
				"WebSocket认证", "authenticateWebSocketUpgrade",
				"开发模式下需要提供 token，请先调用 POST /api/auth/token",
				time.Since(opStart),
				fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
				fmt.Sprintf("result=denied"),
			)
			return "", false
		}

		if strings.HasPrefix(token, "debug-token-") {
			userID := strings.TrimPrefix(token, "debug-token-")
			elapsed := time.Since(opStart)
			utilities.LogVerbose(
				"WebSocket认证", "authenticateWebSocketUpgrade",
				fmt.Sprintf("开发模式认证通过: 用户=%s, Token=%s, 耗时=%s",
					userID, utilities.Mask(token), elapsed),
				fmt.Sprintf("user_id=%s", userID),
				fmt.Sprintf("token_masked=%s", utilities.Mask(token)),
				fmt.Sprintf("mode=development"),
				fmt.Sprintf("result=allowed"),
				fmt.Sprintf("elapsed_ns=%d", elapsed.Nanoseconds()),
				fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
			)
			return userID, true
		}

		utilities.LogWarn(
			"WebSocket认证", "authenticateWebSocketUpgrade",
			"无效的 debug token 格式，请使用 POST /api/auth/token 获取",
			time.Since(opStart),
			fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
			fmt.Sprintf("token_masked=%s", utilities.Mask(token)),
			fmt.Sprintf("result=denied"),
		)
		return "", false
	}

	return authenticateWebSocketProduction(httpRequest)
}

// authenticateWebSocketProduction 在生产模式下执行真实的 WebSocket 认证逻辑。
func authenticateWebSocketProduction(httpRequest *http.Request) (string, bool) {
	opStart := time.Now()

	token := strings.TrimSpace(httpRequest.URL.Query().Get("token"))
	if token == "" {
		authHeader := strings.TrimSpace(httpRequest.Header.Get("Authorization"))
		token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}

	if token == "" {
		utilities.LogWarn(
			"WebSocket认证", "authenticateWebSocketProduction",
			"生产模式下缺少认证 token",
			time.Since(opStart),
			fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
			fmt.Sprintf("result=denied"),
		)
		return "", false
	}

	utilities.LogVerbose(
		"WebSocket认证", "authenticateWebSocketProduction",
		fmt.Sprintf("生产模式认证: Token=%s, 远程地址=%s", utilities.Mask(token), httpRequest.RemoteAddr),
		fmt.Sprintf("token_masked=%s", utilities.Mask(token)),
		fmt.Sprintf("remote=%s", httpRequest.RemoteAddr),
		fmt.Sprintf("mode=production"),
	)

	// TODO: 用户需要实现的认证逻辑
	_ = token

	elapsed := time.Since(opStart)
	utilities.LogVerbose(
		"WebSocket认证", "authenticateWebSocketProduction",
		fmt.Sprintf("生产模式认证通过: 用户=%s, 耗时=%s", "user-from-token", elapsed),
		fmt.Sprintf("user_id=%s", "user-from-token"),
		fmt.Sprintf("result=allowed"),
		fmt.Sprintf("elapsed_ns=%d", elapsed.Nanoseconds()),
	)
	return "user-from-token", true
}

func computeTokenMAC(payload string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}