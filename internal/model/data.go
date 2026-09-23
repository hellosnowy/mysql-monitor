package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// TableData 单个数据表的数据行快照
type TableData struct {
	// TableName 数据表名称
	TableName string `json:"table_name"`
	// PrimaryKeyColumns 主键列名清单
	PrimaryKeyColumns []string `json:"primary_key_columns"`
	// ColumnNames 列名清单（保持查询返回顺序）
	ColumnNames []string `json:"column_names"`
	// Rows 所有行数据，每行是一个以列名为键、列值为值的映射
	Rows []map[string]interface{} `json:"rows"`
	// TotalRows 捕获到的数据总行数
	TotalRows int `json:"total_rows"`
}

// GenerateRowKey 根据主键列计算单行数据的唯一标识 Key；若无主键则计算全列哈希
func (t *TableData) GenerateRowKey(row map[string]interface{}) string {
	if len(t.PrimaryKeyColumns) > 0 {
		parts := make([]interface{}, 0, len(t.PrimaryKeyColumns))
		for _, pkCol := range t.PrimaryKeyColumns {
			parts = append(parts, row[pkCol])
		}
		// JSON 编码保留值类型和边界，避免复合主键拼接碰撞。
		encoded, err := json.Marshal(parts)
		if err == nil {
			return "pk:" + string(encoded)
		}
		return fmt.Sprintf("pk:%#v", parts)
	}

	// 无主键表：按列名排序后计算整行所有字段值的 SHA256 哈希作为唯一键
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]interface{}, 0, len(keys)*2)
	for _, k := range keys {
		parts = append(parts, k, row[k])
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		encoded = []byte(fmt.Sprintf("%#v", parts))
	}
	hash := sha256.Sum256(encoded)
	return "row:" + hex.EncodeToString(hash[:])
}

// DatabaseData 数据库层面的数据快照汇总
type DatabaseData struct {
	// Tables 被监控表的数据映射表（key: 表名）
	Tables map[string]*TableData `json:"tables"`
}
