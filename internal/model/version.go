package model

import "time"

// VersionMeta 历史版本快照元数据信息
type VersionMeta struct {
	// VersionID 版本唯一 ID（格式：vYYYYMMDD_HHMMSS 或自定义标识）
	VersionID string `json:"version_id"`
	// ConnectionID 所属数据库连接 ID
	ConnectionID string `json:"connection_id"`
	// DatabaseName 数据库名称
	DatabaseName string `json:"database_name"`
	// IsBaseline 是否为基准版本（首次拉取或用户手动设定的基线）
	IsBaseline bool `json:"is_baseline"`
	// Description 快照描述或变更备注
	Description string `json:"description"`
	// TableCount 本次快照包含的数据表总数
	TableCount int `json:"table_count"`
	// RowCount 本次快照抓取的数据记录总行数
	RowCount int `json:"row_count"`
	// CreatedAt 快照创建时间
	CreatedAt time.Time `json:"created_at"`
}

// Snapshot 包含元数据、结构定义与数据内容的完整快照实体
type Snapshot struct {
	// Meta 快照元数据
	Meta VersionMeta `json:"meta"`
	// Schema 数据库表结构快照
	Schema *DatabaseSchema `json:"schema"`
	// Data 数据库表数据快照（若为 schema_only 则可能仅包含结构）
	Data *DatabaseData `json:"data"`
}
