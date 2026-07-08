package main

import (
	"context"
	"ling_flow/internal/services"
	"ling_flow/internal/utilities"
	"os"
	"os/signal"
	"syscall"
)

func main() {}

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
