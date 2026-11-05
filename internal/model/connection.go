package model

import (
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

// ConnectionConfig 数据库连接配置信息（多方言通用）
type ConnectionConfig struct {
	// ID 连接唯一标识 ID
	ID string `json:"id"`
	// Name 连接名称或别名
	Name string `json:"name"`
	// Vendor 数据库厂商: mysql / postgres / sqlserver，老数据缺省为 mysql
	Vendor Vendor `json:"vendor,omitempty"`
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
	// Schema 数据库模式: PostgreSQL 默认 public，SQL Server 默认 dbo，MySQL 使用数据库名
	Schema string `json:"schema,omitempty"`
	// Charset 字符集，仅 MySQL 使用，默认 utf8mb4
	Charset string `json:"charset,omitempty"`
	// SSLMode PostgreSQL sslmode 或 SQL Server encrypt 参数
	SSLMode string `json:"ssl_mode,omitempty"`
	// Options 其他厂商特定连接参数
	Options map[string]string `json:"options,omitempty"`
	// TimeoutSeconds 连接超时时间（秒），默认 10
	TimeoutSeconds int `json:"timeout_seconds"`
	// Filter 针对该连接的数据表黑白名单与监控模式
	Filter TableFilterConfig `json:"filter"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// Normalize 补齐连接配置的默认值，并归一化老版本缺失的厂商字段
func (c *ConnectionConfig) Normalize() {
	c.Vendor = NormalizeVendor(c.Vendor)
	switch c.Vendor {
	case VendorPostgres:
		if c.Port == 0 {
			c.Port = 5432
		}
		if strings.TrimSpace(c.Schema) == "" {
			c.Schema = "public"
		}
		if strings.TrimSpace(c.SSLMode) == "" {
			c.SSLMode = "prefer"
		}
	case VendorSQLServer:
		if c.Port == 0 {
			c.Port = 1433
		}
		if strings.TrimSpace(c.Schema) == "" {
			c.Schema = "dbo"
		}
		if strings.TrimSpace(c.SSLMode) == "" {
			c.SSLMode = "disable"
		}
	default:
		if c.Port == 0 {
			c.Port = 3306
		}
		if strings.TrimSpace(c.Charset) == "" {
			c.Charset = "utf8mb4"
		}
		// MySQL 使用数据库名作为 schema，单独的 schema 字段无意义
		c.Schema = ""
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 10
	}
	if c.Filter.MonitorMode == "" {
		c.Filter.MonitorMode = "schema_and_data"
	}
}

// EffectiveSchema 返回实际用于抓取的模式名
func (c *ConnectionConfig) EffectiveSchema() string {
	switch NormalizeVendor(c.Vendor) {
	case VendorPostgres:
		if c.Schema == "" {
			return "public"
		}
		return c.Schema
	case VendorSQLServer:
		if c.Schema == "" {
			return "dbo"
		}
		return c.Schema
	default:
		return c.Database
	}
}

// ConnectionTestResult 数据库连通性测试结果
type ConnectionTestResult struct {
	// Success 是否连接成功
	Success bool `json:"success"`
	// Message 结果描述或错误信息
	Message string `json:"message"`
	// ServerVersion 服务端版本信息
	ServerVersion string `json:"server_version"`
	// LatencyMs 响应延迟毫秒数
	LatencyMs int64 `json:"latency_ms"`
}
