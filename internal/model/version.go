package model

import (
	"strings"
	"time"
)

// SnapshotFormatVersion 当前快照格式版本
const SnapshotFormatVersion = 2

// VersionMeta 历史版本快照元数据信息
type VersionMeta struct {
	// VersionID 版本唯一 ID（格式：vYYYYMMDD_HHMMSS_mmm 或老版本 vYYYYMMDD_HHMMSS）
	VersionID string `json:"version_id"`
	// ConnectionID 所属数据库连接 ID
	ConnectionID string `json:"connection_id"`
	// Vendor 数据库厂商，老数据缺省为 mysql
	Vendor Vendor `json:"vendor,omitempty"`
	// DatabaseName 数据库名称
	DatabaseName string `json:"database_name"`
	// Schema 模式名（PostgreSQL/SQL Server）
	Schema string `json:"schema,omitempty"`
	// FormatVersion 快照格式版本，老数据缺省为 1
	FormatVersion int `json:"format_version,omitempty"`
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

// Normalize 归一化老版本快照中缺失的厂商与格式字段
func (m *VersionMeta) Normalize() {
	m.Vendor = NormalizeVendor(m.Vendor)
	if m.FormatVersion <= 0 {
		m.FormatVersion = 1
	}
}

// Normalize 归一化快照元数据与结构数据
func (s *Snapshot) Normalize() {
	if s == nil {
		return
	}
	s.Meta.Normalize()
	if s.Schema != nil {
		s.Schema.Normalize(s.Meta.Vendor)
		if s.Meta.Schema == "" {
			s.Meta.Schema = s.Schema.Schema
		}
	}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// Normalize 归一化老版本结构快照
func (d *DatabaseSchema) Normalize(fallback Vendor) {
	if d == nil {
		return
	}
	if d.Vendor == "" {
		d.Vendor = NormalizeVendor(fallback)
	}
	if d.Tables == nil {
		d.Tables = make(map[string]*TableSchema)
	}
	for _, t := range d.Tables {
		if t == nil {
			continue
		}
		if t.Schema == "" {
			t.Schema = d.Schema
		}
		if t.PrimaryKey != nil && t.PrimaryKey.Name == "" {
			switch d.Vendor {
			case VendorPostgres:
				t.PrimaryKey.Name = t.Name + "_pkey"
			case VendorSQLServer:
				t.PrimaryKey.Name = "PK_" + t.Name
			default:
				t.PrimaryKey.Name = "PRIMARY"
			}
		}
		for _, col := range t.Columns {
			if col == nil {
				continue
			}
			if !col.IsIdentity && containsFold(col.Extra, "auto_increment") {
				col.IsIdentity = true
			}
			if col.DataType == "" {
				col.DataType = col.ColumnType
			}
		}
		for _, idx := range t.Indexes {
			if idx == nil {
				continue
			}
			if idx.Method == "" {
				idx.Method = idx.IndexType
			}
		}
	}
}
