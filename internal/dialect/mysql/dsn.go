package mysql

import (
	"fmt"

	"sql-monitor/internal/model"
)

// formatDSN 生成 MySQL 连接字符串
func formatDSN(cfg *model.ConnectionConfig) string {
	charset := cfg.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local&timeout=%ds",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, charset, timeout)
}
