package sqlserver

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"sql-monitor/internal/model"
)

// formatDSN 生成 SQL Server 连接字符串。
// 格式：sqlserver://user:pass@host:port?database=...&encrypt=...&connection+timeout=N
func formatDSN(cfg *model.ConnectionConfig) string {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}

	encrypt := normalizeEncrypt(cfg.SSLMode)
	query := url.Values{}
	query.Set("database", cfg.Database)
	query.Set("encrypt", encrypt)
	// 关闭加密时显式信任服务器证书，避免自签名证书导致连接失败
	if encrypt == "disable" || encrypt == "false" {
		query.Set("trustservercertificate", "true")
	}
	query.Set("connection timeout", strconv.Itoa(timeout))

	u := &url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		RawQuery: query.Encode(),
	}
	return u.String()
}

// normalizeEncrypt 将通用 SSLMode 映射为 SQL Server 的 encrypt 参数，默认关闭加密
func normalizeEncrypt(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "true", "require", "required", "on", "verify-full", "verify-ca":
		return "true"
	case "false":
		return "false"
	default:
		return "disable"
	}
}
