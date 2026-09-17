package model

import (
	"fmt"
	"strings"
	"time"
)

// TableFilterConfig 数据表黑白名单配置
type TableFilterConfig struct {
	// IncludeTables 表白名单（支持精确表名、通配符如 t_*、正则表达式）
	IncludeTables []string `json:"include_tables"`
	// ExcludeTables 表黑名单（优先级高于白名单）
	ExcludeTables []string `json:"exclude_tables"`
	// MonitorMode 监控模式: "schema_and_data"(结构与数据), "schema_only"(仅结构)
	MonitorMode string `json:"monitor_mode"`
	// MaxRowsPerTable 每张表抓取数据的最大行数限制，0表示不限制
	MaxRowsPerTable int `json:"max_rows_per_table"`
}

// ConnectionConfig MySQL 数据库连接配置信息
type ConnectionConfig struct {
	// ID 连接唯一标识 ID
	ID string `json:"id"`
	// Name 连接名称或别名
	Name string `json:"name"`
	// Host 数据库主机地址
	Host string `json:"host"`
	// Port 数据库端口
	Port int `json:"port"`
	// User 登录用户名
	User string `json:"user"`
	// Password 登录密码
	Password string `json:"password"`
	// Database 目标数据库名称
	Database string `json:"database"`
	// Charset 字符集，默认 utf8mb4
	Charset string `json:"charset"`
	// TimeoutSeconds 连接超时时间（秒），默认 10
	TimeoutSeconds int `json:"timeout_seconds"`
	// Filter 针对该连接的数据表黑白名单与监控模式
	Filter TableFilterConfig `json:"filter"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// FormatDSN 生成 MySQL 连接字符串 (DSN)
func (c *ConnectionConfig) FormatDSN() string {
	charset := c.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	timeout := c.TimeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	// 用户名:密码@tcp(主机:端口)/数据库名?charset=utf8mb4&parseTime=True&loc=Local&timeout=10s
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local&timeout=%ds",
		c.User, c.Password, c.Host, c.Port, c.Database, charset, timeout)
}

// Validate 校验数据库连接配置的合法性
func (c *ConnectionConfig) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("连接 ID 不能为空")
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("数据库主机不能为空")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("数据库端口无效: %d", c.Port)
	}
	if strings.TrimSpace(c.User) == "" {
		return fmt.Errorf("数据库用户名不能为空")
	}
	if strings.TrimSpace(c.Database) == "" {
		return fmt.Errorf("数据库名称不能为空")
	}
	if c.Filter.MonitorMode == "" {
		c.Filter.MonitorMode = "schema_and_data"
	}
	return nil
}

// ConnectionTestResult 数据库连通性测试结果
type ConnectionTestResult struct {
	// Success 是否连接成功
	Success bool `json:"success"`
	// Message 结果描述或错误信息
	Message string `json:"message"`
	// ServerVersion MySQL 服务端版本信息
	ServerVersion string `json:"server_version"`
	// LatencyMs 响应延迟毫秒数
	LatencyMs int64 `json:"latency_ms"`
}
