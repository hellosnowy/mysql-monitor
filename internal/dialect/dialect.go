package dialect

import (
	"context"
	"database/sql"
	"fmt"

	"sql-monitor/internal/diff"
	"sql-monitor/internal/model"
	"sql-monitor/internal/sqlgen"
)

// TableFilter 表黑白名单过滤抽象
type TableFilter interface {
	ShouldInclude(tableName string) bool
}

// Capabilities 方言能力描述，供前端展示与后续功能裁剪
type Capabilities struct {
	SchemaNamespace  bool `json:"schema_namespace"`
	IdentityInsert   bool `json:"identity_insert"`
	TransactionalDDL bool `json:"transactional_ddl"`
	TableComments    bool `json:"table_comments"`
	ColumnComments   bool `json:"column_comments"`
	PartialIndexes   bool `json:"partial_indexes"`
}

// DriverInfo 内置驱动与方言信息
type DriverInfo struct {
	Vendor          model.Vendor `json:"vendor"`
	DisplayName     string       `json:"display_name"`
	DriverPackage   string       `json:"driver_package"`
	DriverVersion   string       `json:"driver_version"`
	BuiltIn         bool         `json:"built_in"`
	DefaultPort     int          `json:"default_port"`
	DefaultSchema   string       `json:"default_schema"`
	SupportsSchemas bool         `json:"supports_schemas"`
	Capabilities    Capabilities `json:"capabilities"`
}

// Plugin 数据库方言插件：连接、抓取快照、比对与 SQL 生成均由插件实现
type Plugin interface {
	Info() DriverInfo
	Normalize(cfg *model.ConnectionConfig)
	Validate(cfg *model.ConnectionConfig) error
	Open(ctx context.Context, cfg *model.ConnectionConfig) (*sql.DB, error)
	TestConnection(ctx context.Context, cfg *model.ConnectionConfig) (*model.ConnectionTestResult, error)
	ListSchemas(ctx context.Context, cfg *model.ConnectionConfig) ([]string, error)
	ExtractSchema(ctx context.Context, db *sql.DB, cfg *model.ConnectionConfig, f TableFilter) (*model.DatabaseSchema, error)
	ExtractData(ctx context.Context, db *sql.DB, cfg *model.ConnectionConfig, schema *model.DatabaseSchema) (*model.DatabaseData, error)
	Capture(ctx context.Context, cfg *model.ConnectionConfig, f TableFilter) (*model.DatabaseSchema, *model.DatabaseData, error)
	Emitter() sqlgen.Emitter
	Compare(from, to *model.Snapshot, dir model.DiffDirection) (*model.DiffResult, error)
}

// CaptureWith 通用的“打开连接 → 提取结构 → 提取数据 → 关闭”流程
func CaptureWith(p Plugin, ctx context.Context, cfg *model.ConnectionConfig, f TableFilter) (*model.DatabaseSchema, *model.DatabaseData, error) {
	p.Normalize(cfg)
	if err := p.Validate(cfg); err != nil {
		return nil, nil, fmt.Errorf("连接配置不合法: %w", err)
	}

	db, err := p.Open(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	schema, err := p.ExtractSchema(ctx, db, cfg, f)
	if err != nil {
		return nil, nil, fmt.Errorf("抓取表结构失败: %w", err)
	}
	data, err := p.ExtractData(ctx, db, cfg, schema)
	if err != nil {
		return nil, nil, fmt.Errorf("抓取表数据快照失败: %w", err)
	}
	return schema, data, nil
}

// CompareWith 通用的版本比对流程，方言只负责 SQL 发射
func CompareWith(p Plugin, from, to *model.Snapshot, dir model.DiffDirection) (*model.DiffResult, error) {
	if from == nil || to == nil {
		return nil, fmt.Errorf("起始版本和目标版本快照不能为空")
	}
	from.Normalize()
	to.Normalize()
	if from.Meta.Vendor != to.Meta.Vendor {
		return nil, fmt.Errorf("无法比对不同数据库类型的快照: %s 与 %s", from.Meta.Vendor.DisplayName(), to.Meta.Vendor.DisplayName())
	}
	return diff.GenerateDiffResult(from.Meta.ConnectionID, from.Meta.VersionID, to.Meta.VersionID, from, to, dir, p.Emitter()), nil
}
