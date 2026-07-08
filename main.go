package main

import (
	"context"
	"ling_flow/internal/services"
	"ling_flow/internal/utilities"
	"os"
	"os/signal"
	"syscall"
)

func main() {
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if utilities.IsLocalMode() {
		utilities.LogProgress("Main", "Startup", "运行模式=local/server，启动 WebSocket 服务")
	} else {
		utilities.LogProgress("Main", "Startup", "运行模式=cloud，按配置启动 WebSocket 服务")
	}

	if err := services.Run(ctx); err != nil && err != context.Canceled {
		utilities.Error("fatal: %v", err)
		os.Exit(1)
	}
}
