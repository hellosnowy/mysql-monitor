package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"mysql-monitor/internal/model"
)

// Client MySQL 数据库客户端包装
type Client struct {
	db     *sql.DB
	config *model.ConnectionConfig
}

// NewClient 根据配置创建 MySQL 客户端实例并初始化连接池
func NewClient(cfg *model.ConnectionConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("连接配置不合法: %w", err)
	}

	dsn := cfg.FormatDSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 连接失败: %w", err)
	}

	// 配置合理的连接池参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &Client{
		db:     db,
		config: cfg,
	}, nil
}

// DB 获取原生 *sql.DB 实例
func (c *Client) DB() *sql.DB {
	return c.db
}

// Close 关闭数据库连接池
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Ping 测试数据库连接连通性
func (c *Client) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// TestConnection 静态测试指定的数据库连接连通性并获取版本与响应延迟
func TestConnection(cfg *model.ConnectionConfig) (*model.ConnectionTestResult, error) {
	start := time.Now()
	client, err := NewClient(cfg)
	if err != nil {
		return &model.ConnectionTestResult{
			Success: false,
			Message: fmt.Sprintf("创建连接失败: %v", err),
		}, nil
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		latency := time.Since(start).Milliseconds()
		return &model.ConnectionTestResult{
			Success:   false,
			Message:   fmt.Sprintf("连接探测超时或失败: %v", err),
			LatencyMs: latency,
		}, nil
	}

	var version string
	err = client.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &model.ConnectionTestResult{
			Success:   false,
			Message:   fmt.Sprintf("查询数据库版本失败: %v", err),
			LatencyMs: latency,
		}, nil
	}

	return &model.ConnectionTestResult{
		Success:       true,
		Message:       "连接成功",
		ServerVersion: version,
		LatencyMs:     latency,
	}, nil
}
