package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"sql-monitor/internal/dialect"
	"sql-monitor/internal/model"
	"sql-monitor/internal/sqlgen"
)

// Plugin PostgreSQL 方言插件
type Plugin struct{}

// New 创建 PostgreSQL 方言插件
func New() *Plugin { return &Plugin{} }

// Info 返回 PostgreSQL 驱动与方言信息
func (p *Plugin) Info() dialect.DriverInfo {
	return dialect.DriverInfo{
		Vendor:          model.VendorPostgres,
		DisplayName:     "PostgreSQL",
		DriverPackage:   "github.com/jackc/pgx/v5",
		DriverVersion:   "v5.12.0",
		BuiltIn:         true,
		DefaultPort:     5432,
		DefaultSchema:   "public",
		SupportsSchemas: true,
		Capabilities: dialect.Capabilities{
			SchemaNamespace:  true,
			IdentityInsert:   true,
			TransactionalDDL: true,
			TableComments:    true,
			ColumnComments:   true,
			PartialIndexes:   true,
		},
	}
}

// Normalize 补齐 PostgreSQL 连接默认值
func (p *Plugin) Normalize(cfg *model.ConnectionConfig) { cfg.Normalize() }

// Validate 校验 PostgreSQL 连接配置
func (p *Plugin) Validate(cfg *model.ConnectionConfig) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("数据库主机不能为空")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("数据库端口无效: %d", cfg.Port)
	}
	if strings.TrimSpace(cfg.User) == "" {
		return fmt.Errorf("数据库用户名不能为空")
	}
	if strings.TrimSpace(cfg.Database) == "" {
		return fmt.Errorf("数据库名称不能为空")
	}
	if strings.TrimSpace(cfg.Schema) == "" {
		cfg.Schema = "public"
	}
	return nil
}

// Open 建立 PostgreSQL 连接池
func (p *Plugin) Open(ctx context.Context, cfg *model.ConnectionConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", formatDSN(cfg))
	if err != nil {
		return nil, fmt.Errorf("打开 PostgreSQL 连接失败: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// TestConnection 测试连通性并返回服务端版本与延迟
func (p *Plugin) TestConnection(ctx context.Context, cfg *model.ConnectionConfig) (*model.ConnectionTestResult, error) {
	p.Normalize(cfg)
	start := time.Now()

	db, err := p.Open(ctx, cfg)
	if err != nil {
		return &model.ConnectionTestResult{Success: false, Message: fmt.Sprintf("创建连接失败: %v", err)}, nil
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		return &model.ConnectionTestResult{
			Success:   false,
			Message:   fmt.Sprintf("连接探测超时或失败: %v", err),
			LatencyMs: time.Since(start).Milliseconds(),
		}, nil
	}

	var version string
	if err := db.QueryRowContext(pingCtx, "SELECT version()").Scan(&version); err != nil {
		return &model.ConnectionTestResult{
			Success:   false,
			Message:   fmt.Sprintf("查询数据库版本失败: %v", err),
			LatencyMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return &model.ConnectionTestResult{
		Success:       true,
		Message:       "连接成功",
		ServerVersion: version,
		LatencyMs:     time.Since(start).Milliseconds(),
	}, nil
}

// ListSchemas 列出用户模式，排除 pg_ 系统模式与 information_schema
func (p *Plugin) ListSchemas(ctx context.Context, cfg *model.ConnectionConfig) ([]string, error) {
	p.Normalize(cfg)
	db, err := p.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT nspname
		FROM pg_catalog.pg_namespace
		WHERE nspname !~ '^pg_'
		  AND nspname <> 'information_schema'
		ORDER BY nspname`)
	if err != nil {
		return nil, fmt.Errorf("查询模式列表失败: %w", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		schemas = append(schemas, name)
	}
	return schemas, rows.Err()
}

// Capture 抓取 PostgreSQL 结构与数据快照
func (p *Plugin) Capture(ctx context.Context, cfg *model.ConnectionConfig, f dialect.TableFilter) (*model.DatabaseSchema, *model.DatabaseData, error) {
	return dialect.CaptureWith(p, ctx, cfg, f)
}

// Emitter 返回 PostgreSQL SQL 发射器
func (p *Plugin) Emitter() sqlgen.Emitter { return &Emitter{} }

// Compare 生成 PostgreSQL 方言的升级/回滚脚本
func (p *Plugin) Compare(from, to *model.Snapshot, dir model.DiffDirection) (*model.DiffResult, error) {
	return dialect.CompareWith(p, from, to, dir)
}
