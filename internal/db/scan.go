// Package db 提供跨方言的 database/sql 行扫描辅助。
package db

import (
	"database/sql"
	"fmt"
)

// ScanRows 将查询结果扫描为列名与行映射，[]byte 统一转换为 string。
// 返回的 map 以列名为键，保持数据库返回的顺序由调用方另行记录。
func ScanRows(rows *sql.Rows) ([]string, []map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("读取列名失败: %w", err)
	}

	colCount := len(columns)
	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		rawValues := make([]interface{}, colCount)
		scanArgs := make([]interface{}, colCount)
		for i := range rawValues {
			scanArgs[i] = &rawValues[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, nil, fmt.Errorf("扫描行数据失败: %w", err)
		}

		row := make(map[string]interface{}, colCount)
		for i, col := range columns {
			if b, ok := rawValues[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = rawValues[i]
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return columns, result, nil
}
