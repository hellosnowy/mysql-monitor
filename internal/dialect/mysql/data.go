package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	scanutil "sql-monitor/internal/db"
	"sql-monitor/internal/model"
	"sql-monitor/pkg/sqlutil"
)

// ExtractData 依据结构信息拉取被监控表的行数据快照
func (p *Plugin) ExtractData(ctx context.Context, db *sql.DB, cfg *model.ConnectionConfig, schema *model.DatabaseSchema) (*model.DatabaseData, error) {
	result := &model.DatabaseData{Tables: make(map[string]*model.TableData)}

	// 仅监控表结构时跳过数据抓取
	if cfg.Filter.MonitorMode == "schema_only" {
		return result, nil
	}

	for tableName, tableSchema := range schema.Tables {
		tableData, err := p.extractTableData(ctx, db, cfg, tableSchema)
		if err != nil {
			return nil, fmt.Errorf("提取数据表 %s 数据失败: %w", tableName, err)
		}
		result.Tables[tableName] = tableData
	}
	return result, nil
}

func (p *Plugin) extractTableData(ctx context.Context, db *sql.DB, cfg *model.ConnectionConfig, tableSchema *model.TableSchema) (*model.TableData, error) {
	var pkCols []string
	if tableSchema.PrimaryKey != nil {
		pkCols = tableSchema.PrimaryKey.Columns
	}

	var sb strings.Builder
	sb.WriteString("SELECT * FROM ")
	sb.WriteString(sqlutil.EscapeIdentifier(tableSchema.Name))

	if len(pkCols) > 0 {
		sb.WriteString(" ORDER BY ")
		escapedPKs := make([]string, 0, len(pkCols))
		for _, pk := range pkCols {
			escapedPKs = append(escapedPKs, sqlutil.EscapeIdentifier(pk))
		}
		sb.WriteString(strings.Join(escapedPKs, ", "))
	}
	if cfg.Filter.MaxRowsPerTable > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", cfg.Filter.MaxRowsPerTable))
	}

	rows, err := db.QueryContext(ctx, sb.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, rowMaps, err := scanutil.ScanRows(rows)
	if err != nil {
		return nil, err
	}

	return &model.TableData{
		TableName:         tableSchema.Name,
		PrimaryKeyColumns: pkCols,
		ColumnNames:       colNames,
		Rows:              rowMaps,
		TotalRows:         len(rowMaps),
	}, nil
}
