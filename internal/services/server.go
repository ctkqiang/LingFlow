package services

import (
	"context"
	"crypto/tls"
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
//   - server 模式监听地址，例如 ":4030"
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
	runStart := time.Now()
	traceID := utilities.NewTraceID()
	utilities.LogStart("Services", "Run")

	// 读取并记录所有配置环境变量
	resolvedRuntime := ResolveWebSocketRuntime()
	resolvedAddr := utilities.GetEnv("WSS_ADDR", ":4030")
	resolvedCert := os.Getenv("WSS_CERT_FILE")
	resolvedKey := os.Getenv("WSS_KEY_FILE")

	hasTLS := resolvedCert != "" && resolvedKey != ""
	utilities.LogVerbose("Services", "Run", "已解析全部运行时配置",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("runtime=%s", resolvedRuntime),
		fmt.Sprintf("address=%s", resolvedAddr),
		fmt.Sprintf("tls_enabled=%v", hasTLS),
		fmt.Sprintf("cert_file=%s", utilities.Mask(resolvedCert)),
		fmt.Sprintf("key_file=%s", utilities.Mask(resolvedKey)),
		fmt.Sprintf("env.WSS_ADDR=%s", resolvedAddr),
		fmt.Sprintf("env.WEBSOCKET_RUNTIME=%s", os.Getenv("WEBSOCKET_RUNTIME")),
		fmt.Sprintf("env.WSS_RUNTIME=%s", os.Getenv("WSS_RUNTIME")),
		fmt.Sprintf("env.RUNTIME_MODE=%s", os.Getenv("RUNTIME_MODE")),
		fmt.Sprintf("env.AWS_LAMBDA_RUNTIME_API=%s", utilities.Mask(os.Getenv("AWS_LAMBDA_RUNTIME_API"))),
	)

	err := ServeWebSocket(ctx, WebSocketServerConfig{
		Runtime:  resolvedRuntime,
		Address:  resolvedAddr,
		CertFile: resolvedCert,
		KeyFile:  resolvedKey,
	})

	if err != nil {
		utilities.LogError("Services", "Run", err, time.Since(runStart),
			fmt.Sprintf("trace_id=%s", traceID))
	} else {
		utilities.LogSuccess("Services", "Run", time.Since(runStart),
			fmt.Sprintf("trace_id=%s", traceID))
	}

	return err
}

// ServeWebSocket 根据配置切换 WebSocket 服务运行方式。
//
// Lambda 模式：注册 API Gateway WebSocket Lambda handler。
// Server 模式：启动 EC2/本地可用的 WebSocket HTTP(S) 服务。
func ServeWebSocket(runtimeContext context.Context, websocketServerConfiguration WebSocketServerConfig) error {
	serveStart := time.Now()
	utilities.LogStart("Services", "ServeWebSocket")

	websocketRuntime := websocketServerConfiguration.Runtime
	originalRuntime := websocketRuntime

	if websocketRuntime == "" || websocketRuntime == WebSocketRuntimeAuto {
		websocketRuntime = ResolveWebSocketRuntime()
		utilities.LogVerbose("Services", "ServeWebSocket", "运行模式自动解析完成",
			fmt.Sprintf("原始值=%s", originalRuntime),
			fmt.Sprintf("解析结果=%s", websocketRuntime),
			fmt.Sprintf("配置地址=%s", websocketServerConfiguration.Address),
			fmt.Sprintf("tls_cert=%v", websocketServerConfiguration.CertFile != ""),
			fmt.Sprintf("tls_key=%v", websocketServerConfiguration.KeyFile != ""),
		)
	} else {
		utilities.LogVerbose("Services", "ServeWebSocket", "使用显式指定的运行模式",
			fmt.Sprintf("runtime=%s", websocketRuntime),
		)
	}

	switch websocketRuntime {
	case WebSocketRuntimeLambda:
		utilities.LogProgress("Services", "ServeWebSocket", "运行模式=lambda，启动 API Gateway WebSocket Lambda handler")
		utilities.LogNano("Services", "ServeWebSocket", utilities.INFO, "LAMBDA_DISPATCH",
			time.Since(serveStart))
		lambdaadapter.HandleLambdaRequest()
		utilities.LogSuccess("Services", "ServeWebSocket", time.Since(serveStart), "mode=lambda")
		return nil
	case WebSocketRuntimeServer:
		utilities.LogProgress("Services", "ServeWebSocket", "运行模式=server，启动 EC2/local WebSocket server")
		utilities.LogNano("Services", "ServeWebSocket", utilities.INFO, "SERVER_DISPATCH",
			time.Since(serveStart))
		return serveWebSocketHTTPServer(runtimeContext, websocketServerConfiguration)
	default:
		err := fmt.Errorf("不支持的 WebSocket 运行模式: %s", websocketRuntime)
		utilities.LogError("Services", "ServeWebSocket", err, time.Since(serveStart),
			fmt.Sprintf("runtime=%s", websocketRuntime))
		return err
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
	utilities.LogStart("Services", "ResolveWebSocketRuntime")
	resolveStart := time.Now()

	envWebsocketRuntime := os.Getenv("WEBSOCKET_RUNTIME")
	envWssRuntime := os.Getenv("WSS_RUNTIME")
	envRuntimeMode := os.Getenv("RUNTIME_MODE")

	utilities.LogVerbose("Services", "ResolveWebSocketRuntime", "读取运行模式环境变量",
		fmt.Sprintf("WEBSOCKET_RUNTIME=%q", envWebsocketRuntime),
		fmt.Sprintf("WSS_RUNTIME=%q", envWssRuntime),
		fmt.Sprintf("RUNTIME_MODE=%q", envRuntimeMode),
	)

	rawRuntimeMode := firstNonEmpty(
		envWebsocketRuntime,
		envWssRuntime,
		envRuntimeMode,
		string(WebSocketRuntimeAuto),
	)

	websocketRuntime := WebSocketRuntime(strings.ToLower(strings.TrimSpace(rawRuntimeMode)))
	if websocketRuntime != WebSocketRuntimeAuto {
		utilities.LogNano("Services", "ResolveWebSocketRuntime", utilities.INFO, "RESOLVED_EXPLICIT",
			time.Since(resolveStart), fmt.Sprintf("result=%s", websocketRuntime))
		return websocketRuntime
	}

	lambdaAPI := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if lambdaAPI != "" {
		utilities.LogNano("Services", "ResolveWebSocketRuntime", utilities.INFO, "RESOLVED_LAMBDA",
			time.Since(resolveStart),
			fmt.Sprintf("AWS_LAMBDA_RUNTIME_API=%s", utilities.Mask(lambdaAPI)))
		return WebSocketRuntimeLambda
	}

	utilities.LogNano("Services", "ResolveWebSocketRuntime", utilities.INFO, "RESOLVED_SERVER",
		time.Since(resolveStart), "未检测到 Lambda 环境，使用 server 模式")
	return WebSocketRuntimeServer
}

func serveWebSocketHTTPServer(
	runtimeContext context.Context,
	websocketServerConfiguration WebSocketServerConfig,
) error {
	httpServerStart := time.Now()
	traceID := utilities.NewTraceID()
	utilities.LogStart("Services", "serveWebSocketHTTPServer")
	utilities.LogVerbose("Services", "serveWebSocketHTTPServer", "开始初始化 HTTP WebSocket 服务器",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("address=%s", websocketServerConfiguration.Address),
	)

	// ── 步骤1: TLS 检查 ──
	stepStart := time.Now()
	tlsRequired := !utilities.IsLocalMode()
	hasTLSConfig := websocketServerConfiguration.CertFile != "" && websocketServerConfiguration.KeyFile != ""
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "TLS_CHECK",
		time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("tls_required=%v", tlsRequired),
		fmt.Sprintf("has_tls_config=%v", hasTLSConfig),
		fmt.Sprintf("is_local_mode=%v", !tlsRequired),
	)

	if tlsRequired && !hasTLSConfig {
		err := fmt.Errorf(
			"WebSocket TLS 证书缺失：在非 local 模式下必须配置 WSS_CERT_FILE 和 WSS_KEY_FILE 启用 wss://",
		)
		utilities.LogError("Services", "serveWebSocketHTTPServer", err, time.Since(httpServerStart),
			fmt.Sprintf("trace_id=%s", traceID))
		return err
	}
	if !hasTLSConfig {
		utilities.LogWarn(
			"Services",
			"serveWebSocketHTTPServer",
			"未配置 TLS 证书，WebSocket 以明文 ws:// 启动（仅限 local 模式）",
			time.Since(stepStart),
			fmt.Sprintf("trace_id=%s", traceID),
		)
	}

	// ── 步骤2: 创建 HTTP 多路复用器 ──
	stepStart = time.Now()
	websocketServeMux := http.NewServeMux()
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "MUX_CREATED",
		time.Since(stepStart), fmt.Sprintf("trace_id=%s", traceID))

	// ── 步骤3: 初始化事件存储 ──
	stepStart = time.Now()
	eventStore := events.NewInMemoryEventStore()
	websocketConnectionManager := connections.NewEventSourcedWebsocketGatewayConnectionManager(eventStore)
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "EVENT_STORE_INIT",
		time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", traceID),
		"type=InMemoryEventStore",
	)

	// ── 步骤4: 注册认证处理器 ──
	stepStart = time.Now()
	authHandler := NewAuthHandler()
	RegisterAuthHandlers(websocketServeMux, authHandler)
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "AUTH_HANDLER_REGISTERED",
		time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", traceID),
		"endpoint=/api/auth/token",
	)

	// ── 步骤5: 创建技能注册中心 ──
	stepStart = time.Now()
	skillRegistry := NewSkillRegistry()
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "SKILL_REGISTRY_CREATED",
		time.Since(stepStart), fmt.Sprintf("trace_id=%s", traceID))

	// ── 步骤6: 初始化 LLM 服务 ──
	stepStart = time.Now()
	modelID := utilities.GetEnv("AWS_BEDROCK_MODEL_ID", "anthropic.claude-3-5-sonnet-20241022-v2:0")
	region := utilities.GetEnv("AWS_BEDROCK_REGION", "ap-east-1")
	timeout := utilities.GetEnv("AWS_BEDROCK_TIMEOUT", "60s")
	utilities.LogVerbose("Services", "serveWebSocketHTTPServer", "正在初始化 LLM 服务",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("model_id=%s", modelID),
		fmt.Sprintf("region=%s", region),
		fmt.Sprintf("timeout=%s", timeout),
	)

	llmService, err := NewLLMService(runtimeContext)
	if err != nil {
		utilities.LogError("Services", "serveWebSocketHTTPServer", err, time.Since(stepStart),
			fmt.Sprintf("trace_id=%s", traceID), "phase=llm_init")
		return fmt.Errorf("初始化 LLM 服务失败: %w", err)
	}
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "LLM_SERVICE_INIT",
		time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("model_id=%s", modelID),
		fmt.Sprintf("region=%s", region),
	)

	// ── 步骤7: 初始化 S3 技能加载器并加载技能 ──
	stepStart = time.Now()
	s3Loader := NewS3SkillLoader()
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "S3_LOADER_INIT",
		time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("s3_loader_available=%v", s3Loader != nil),
	)

	if s3Loader != nil {
		skillLoadStart := time.Now()
		loadedSkills, loadErr := s3Loader.LoadAllSkills(runtimeContext)
		if loadErr != nil {
			utilities.LogError("Services", "serveWebSocketHTTPServer", loadErr, time.Since(skillLoadStart),
				fmt.Sprintf("trace_id=%s", traceID), "phase=s3_skill_load")
		} else {
			skillNames := make([]string, 0, len(loadedSkills))
			for _, skill := range loadedSkills {
				_ = skillRegistry.RegisterSkill(skill)
				skillNames = append(skillNames, skill.SkillIdentifier)
			}
			utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "SKILLS_LOADED",
				time.Since(skillLoadStart),
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("count=%d", len(loadedSkills)),
				fmt.Sprintf("skill_ids=%v", skillNames),
				fmt.Sprintf("load_time=%s", time.Since(skillLoadStart)),
			)
		}
	}

	// ── 步骤8: 创建 ChatHandler ──
	stepStart = time.Now()
	chatHandler := NewChatHandlerWithS3Loader(skillRegistry, llmService, s3Loader, websocketConnectionManager)
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "CHAT_HANDLER_CREATED",
		time.Since(stepStart), fmt.Sprintf("trace_id=%s", traceID))

	// ── 步骤9: 注册 WebSocket 处理器 ──
	stepStart = time.Now()
	connections.RegisterWebSocketHandlers(
		websocketServeMux,
		websocketConnectionManager,
		connections.NewDefaultUpgrader(),
		chatHandler,
		chatHandler,
		chatHandler,
	)
	utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "WS_HANDLERS_REGISTERED",
		time.Since(stepStart),
		fmt.Sprintf("trace_id=%s", traceID),
		"path=/chat/{uuid}",
	)

	// ── 步骤10: 配置 HTTP 服务器 ──
	websocketServerAddress := websocketServerConfiguration.Address
	if websocketServerAddress == "" {
		websocketServerAddress = ":4030"
	}

	websocketHTTPServer := &http.Server{
		Addr:              websocketServerAddress,
		Handler:           websocketServeMux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	utilities.LogVerbose("Services", "serveWebSocketHTTPServer", "HTTP 服务器配置完成",
		fmt.Sprintf("trace_id=%s", traceID),
		fmt.Sprintf("addr=%s", websocketServerAddress),
		fmt.Sprintf("read_header_timeout=30s"),
		fmt.Sprintf("tls=%v", hasTLSConfig),
		fmt.Sprintf("初始化总耗时=%s", time.Since(httpServerStart)),
	)

	serverErrorChannel := make(chan error, 1)

	go func() {
		protocol := "ws"
		if hasTLSConfig {
			protocol = "wss"
		}
		utilities.LogProgress(
			"Services",
			"WebSocketServer",
			"监听 WebSocket endpoint",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("addr=%s", websocketServerAddress),
			fmt.Sprintf("protocol=%s", protocol),
			"path=/chat/{uuid}",
		)

		// ── 步骤11: 启动服务器 ──
		if hasTLSConfig {
			websocketHTTPServer.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
			utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "TLS_SERVER_START",
				time.Since(httpServerStart),
				fmt.Sprintf("trace_id=%s", traceID),
				fmt.Sprintf("cert=%s", utilities.Mask(websocketServerConfiguration.CertFile)),
				fmt.Sprintf("min_tls=1.2"),
			)
			serverErrorChannel <- websocketHTTPServer.ListenAndServeTLS(
				websocketServerConfiguration.CertFile,
				websocketServerConfiguration.KeyFile,
			)
			return
		}

		utilities.LogNano("Services", "serveWebSocketHTTPServer", utilities.INFO, "PLAINTEXT_SERVER_START",
			time.Since(httpServerStart), fmt.Sprintf("trace_id=%s", traceID))
		serverErrorChannel <- websocketHTTPServer.ListenAndServe()
	}()

	select {
	case <-runtimeContext.Done():
		// ── 步骤12: 优雅关闭 ──
		shutdownStart := time.Now()
		utilities.LogProgress("Services", "serveWebSocketHTTPServer", "收到关闭信号，开始优雅关闭",
			fmt.Sprintf("trace_id=%s", traceID),
			fmt.Sprintf("context_err=%v", runtimeContext.Err()),
		)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if shutdownError := websocketHTTPServer.Shutdown(shutdownCtx); shutdownError != nil {
			utilities.LogError("Services", "serveWebSocketHTTPServer", shutdownError, time.Since(shutdownStart),
				fmt.Sprintf("trace_id=%s", traceID), "phase=graceful_shutdown")
			return fmt.Errorf("WebSocket 服务器关闭失败: %w", shutdownError)
		}

		utilities.LogSuccess("Services", "serveWebSocketHTTPServer", time.Since(shutdownStart),
			fmt.Sprintf("trace_id=%s", traceID),
			"phase=graceful_shutdown",
			fmt.Sprintf("服务器总运行时间=%s", time.Since(httpServerStart)),
		)
		return runtimeContext.Err()
	case serverError := <-serverErrorChannel:
		if errors.Is(serverError, http.ErrServerClosed) {
			utilities.LogProgress("Services", "serveWebSocketHTTPServer", "服务器已正常关闭",
				fmt.Sprintf("trace_id=%s", traceID))
			return nil
		}
		if serverError != nil {
			utilities.LogError("Services", "serveWebSocketHTTPServer", serverError, time.Since(httpServerStart),
				fmt.Sprintf("trace_id=%s", traceID), "phase=server_error")
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
