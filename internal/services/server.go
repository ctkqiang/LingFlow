package services

import (
	"context"
	"ling_flow/internal/utilities"
)

func Run(ctx context.Context) error {
	utilities.LogProgress("Services", "Run", "启动服务器")

	_ = utilities.GetEnv("DB_TIMEZONE", "Asia/Shanghai")

	return nil
}
