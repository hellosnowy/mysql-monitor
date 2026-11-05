package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"

	"sql-monitor/internal/dialect"
	"sql-monitor/internal/model"
	"sql-monitor/internal/sqlgen"
)

// Plugin Microsoft SQL Server 方言插件
type Plugin struct{}

// New 创建 SQL Server 方言插件
func New() *Plugin { return &Plugin{} }

// Info 返回 SQL Server 驱动与方言信息
func (p *Plugin) Info() dialect.DriverInfo {
	return dialect.DriverInfo{
		Vendor:          model.VendorSQLServer,
		DisplayName:     "SQL Server",
		DriverPackage:   "github.com/microsoft/go-mssqldb",
		DriverVersion:   "v1.11.2",
		BuiltIn:         true,
		DefaultPort:     1433,
		DefaultSchema:   "dbo",
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

// Normalize 补齐 SQL Server 连接默认值
func (p *Plugin) Normalize(cfg *model.ConnectionConfig) { cfg.Normalize() }

// Validate 校验 SQL Server 连接配置
func (p *Plugin) Validate(cfg *model.ConnectionConfig) error {
	if cfg.Host == "" {
		return fmt.Errorf("数据库主机不能为空")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("数据库端口无效: %d", cfg.Port)
	}
	if cfg.User == "" {
		return fmt.Errorf("数据库用户名不能为空")
	}
	if cfg.Database == "" {
		return fmt.Errorf("数据库名称不能为空")
	}
	return nil
}

// Open 建立 SQL Server 连接池
func (p *Plugin) Open(ctx context.Context, cfg *model.ConnectionConfig) (*sql.DB, error) {
	db, err := sql.Open("sqlserver", formatDSN(cfg))
	if err != nil {
		return nil, fmt.Errorf("打开 SQL Server 连接失败: %w", err)
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
	if err := db.QueryRowContext(pingCtx, "SELECT @@VERSION").Scan(&version); err != nil {
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

// ListSchemas 列出当前数据库中的用户模式
func (p *Plugin) ListSchemas(ctx context.Context, cfg *model.ConnectionConfig) ([]string, error) {
	p.Normalize(cfg)
	db, err := p.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT name
		FROM sys.schemas
		WHERE schema_id < 16384
		  AND name NOT IN (N'sys', N'guest', N'INFORMATION_SCHEMA')
		ORDER BY name`)
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

// Capture 抓取 SQL Server 结构与数据快照
func (p *Plugin) Capture(ctx context.Context, cfg *model.ConnectionConfig, f dialect.TableFilter) (*model.DatabaseSchema, *model.DatabaseData, error) {
	return dialect.CaptureWith(p, ctx, cfg, f)
}

// Emitter 返回 SQL Server SQL 发射器
func (p *Plugin) Emitter() sqlgen.Emitter { return &Emitter{} }

// Compare 生成 SQL Server 方言的升级/回滚脚本
func (p *Plugin) Compare(from, to *model.Snapshot, dir model.DiffDirection) (*model.DiffResult, error) {
	return dialect.CompareWith(p, from, to, dir)
}
