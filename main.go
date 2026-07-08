package main

import (
	"context"
	"ling_flow/internal/services"
	"ling_flow/internal/utilities"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
)

// WebSocketBufferConfiguration 定义 WebSocket 连接的读写缓冲区大小
type WebSocketBufferConfiguration struct {
	ReadBufferSizeBytes  int // 读缓冲区大小（字节）
	WriteBufferSizeBytes int // 写缓冲区大小（字节）
}

// ApplicationRuntimeConfiguration 定义应用程序运行时的全局配置
type ApplicationRuntimeConfiguration struct {
	RuntimeContext    context.Context              // 应用程序生命周期上下文
	RuntimeCancelFunc context.CancelFunc           // 用于取消上下文的函数
	WebSocketBuffers  WebSocketBufferConfiguration // WebSocket 缓冲区配置
}

func main() {
	runtimeConfiguration := ApplicationRuntimeConfiguration{
		WebSocketBuffers: WebSocketBufferConfiguration{
			ReadBufferSizeBytes:  1024,
			WriteBufferSizeBytes: 1024,
		},
	}

	_ = websocket.Upgrader{
		ReadBufferSize:  runtimeConfiguration.WebSocketBuffers.ReadBufferSizeBytes,
		WriteBufferSize: runtimeConfiguration.WebSocketBuffers.WriteBufferSizeBytes,
	}

	utilities.LogStart("Main", "Startup")

	Application()
}

func Application() {
	initContext := context.Background()
	if err := utilities.LoadConfig(initContext); err != nil {
		utilities.Error("配置加载失败，系统启动中断！原因: %v", err)
		utilities.Error("请检查配置文件路径和环境变量设置")
		os.Exit(1)
	}

	if utilities.IsLocalMode() {
		utilities.LogProgress("Main", "Startup", "运行模式=local，启动 cron+MCP stdio 服务器")

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		if err := services.Run(ctx); err != nil {
			utilities.Error("fatal: %v", err)
			os.Exit(1)
		}

		return
	}
}
