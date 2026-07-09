package services

import (
	"context"
	"errors"
	"fmt"
	"ling_flow/internal/connections"
	"ling_flow/internal/events"
	lambdaadapter "ling_flow/internal/services/aws"
	"ling_flow/internal/utilities"
	"net/http"
	"os"
	"strings"
	"time"
)

type WebSocketRuntime string

const (
	WebSocketRuntimeAuto   WebSocketRuntime = "auto"
	WebSocketRuntimeLambda WebSocketRuntime = "lambda"
	WebSocketRuntimeServer WebSocketRuntime = "server"
)

// WebSocketServerConfig 描述 WebSocket 服务启动所需的运行时配置。
//
// Runtime:
//   - auto   : 自动判断 Lambda 或 EC2/本地服务器
//   - lambda : AWS Lambda + API Gateway WebSocket
//   - server : EC2/本地 HTTP(S) WebSocket 服务
//
// Address:
//   - server 模式监听地址，例如 ":9000"
//
// CertFile / KeyFile:
//   - 两者同时配置时启用 ListenAndServeTLS，直接提供 wss://
//   - 未配置时提供 ws://，通常由 ALB / Nginx / API Gateway 负责 TLS 终止
type WebSocketServerConfig struct {
	Runtime  WebSocketRuntime
	Address  string
	CertFile string
	KeyFile  string
}

func Run(ctx context.Context) error {
	utilities.LogProgress("Services", "Run", "启动服务器")

	return ServeWebSocket(ctx, WebSocketServerConfig{
		Runtime:  ResolveWebSocketRuntime(),
		Address:  utilities.GetEnv("WSS_ADDR", ":9000"),
		CertFile: os.Getenv("WSS_CERT_FILE"),
		KeyFile:  os.Getenv("WSS_KEY_FILE"),
	})
}

// ServeWebSocket 根据配置切换 WebSocket 服务运行方式。
//
// Lambda 模式：注册 API Gateway WebSocket Lambda handler。
// Server 模式：启动 EC2/本地可用的 WebSocket HTTP(S) 服务。
func ServeWebSocket(runtimeContext context.Context, websocketServerConfiguration WebSocketServerConfig) error {
	websocketRuntime := websocketServerConfiguration.Runtime
	if websocketRuntime == "" || websocketRuntime == WebSocketRuntimeAuto {
		websocketRuntime = ResolveWebSocketRuntime()
	}

	switch websocketRuntime {
	case WebSocketRuntimeLambda:
		utilities.LogProgress("Services", "ServeWebSocket", "运行模式=lambda，启动 API Gateway WebSocket Lambda handler")
		lambdaadapter.HandleLambdaRequest()
		return nil
	case WebSocketRuntimeServer:
		utilities.LogProgress("Services", "ServeWebSocket", "运行模式=server，启动 EC2/local WebSocket server")
		return serveWebSocketHTTPServer(runtimeContext, websocketServerConfiguration)
	default:
		return fmt.Errorf("不支持的 WebSocket 运行模式: %s", websocketRuntime)
	}
}

// ResolveWebSocketRuntime 解析 WebSocket 运行模式。
//
// 优先级：
//  1. WEBSOCKET_RUNTIME
//  2. WSS_RUNTIME
//  3. RUNTIME_MODE
//  4. auto 自动判断
//
// auto 模式下，检测到 AWS_LAMBDA_RUNTIME_API 时使用 Lambda，
// 否则使用 server 模式，适用于 EC2 或本地开发。
func ResolveWebSocketRuntime() WebSocketRuntime {
	rawRuntimeMode := firstNonEmpty(
		os.Getenv("WEBSOCKET_RUNTIME"),
		os.Getenv("WSS_RUNTIME"),
		os.Getenv("RUNTIME_MODE"),
		string(WebSocketRuntimeAuto),
	)

	websocketRuntime := WebSocketRuntime(strings.ToLower(strings.TrimSpace(rawRuntimeMode)))
	if websocketRuntime != WebSocketRuntimeAuto {
		return websocketRuntime
	}

	if os.Getenv("AWS_LAMBDA_RUNTIME_API") != "" {
		return WebSocketRuntimeLambda
	}

	return WebSocketRuntimeServer
}

func serveWebSocketHTTPServer(
	runtimeContext context.Context,
	websocketServerConfiguration WebSocketServerConfig,
) error {
	websocketServeMux := http.NewServeMux()
	eventStore := events.NewInMemoryEventStore()
	websocketConnectionManager := connections.NewEventSourcedWebsocketGatewayConnectionManager(eventStore)
	connections.RegisterWebSocketHandlers(
		websocketServeMux,
		websocketConnectionManager,
		connections.NewDefaultUpgrader(),
	)

	websocketServerAddress := websocketServerConfiguration.Address
	if websocketServerAddress == "" {
		websocketServerAddress = ":9000"
	}

	websocketHTTPServer := &http.Server{
		Addr:              websocketServerAddress,
		Handler:           websocketServeMux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	serverErrorChannel := make(chan error, 1)

	go func() {
		utilities.LogProgress(
			"Services",
			"WebSocketServer",
			"监听 WebSocket endpoint",
			fmt.Sprintf("addr=%s", websocketServerAddress),
			"path=/chat/{uuid}",
		)

		if websocketServerConfiguration.CertFile != "" && websocketServerConfiguration.KeyFile != "" {
			serverErrorChannel <- websocketHTTPServer.ListenAndServeTLS(
				websocketServerConfiguration.CertFile,
				websocketServerConfiguration.KeyFile,
			)
			return
		}

		serverErrorChannel <- websocketHTTPServer.ListenAndServe()
	}()

	select {
	case <-runtimeContext.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if shutdownError := websocketHTTPServer.Shutdown(shutdownCtx); shutdownError != nil {
			return fmt.Errorf("WebSocket 服务器关闭失败: %w", shutdownError)
		}
		return runtimeContext.Err()
	case serverError := <-serverErrorChannel:
		if errors.Is(serverError, http.ErrServerClosed) {
			return nil
		}
		return serverError
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
