package postgres

import (
	"net"
	"net/url"
	"strconv"

	"sql-monitor/internal/model"
)

// formatDSN 使用 net/url 生成 PostgreSQL 连接字符串，确保账号、密码与库名正确转义
func formatDSN(cfg *model.ConnectionConfig) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "prefer"
	}
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:   "/" + cfg.Database,
	}
	query := u.Query()
	query.Set("sslmode", sslMode)
	query.Set("connect_timeout", strconv.Itoa(timeout))
	u.RawQuery = query.Encode()
	return u.String()
}
