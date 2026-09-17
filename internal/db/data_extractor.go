package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"mysql-monitor/internal/model"
	"mysql-monitor/pkg/sqlutil"
)

// DataExtractor MySQL 表数据快照提取器
type DataExtractor struct {
	db     *sql.DB
	dbName string
	config model.TableFilterConfig
}

// NewDataExtractor 创建数据快照提取器
func NewDataExtractor(db *sql.DB, dbName string, config model.TableFilterConfig) *DataExtractor {
	return &DataExtractor{
		db:     db,
		dbName: dbName,
		config: config,
	}
}

// ExtractDatabaseData 依据结构信息分批拉取被监控表的行数据快照
func (e *DataExtractor) ExtractDatabaseData(ctx context.Context, schema *model.DatabaseSchema) (*model.DatabaseData, error) {
	result := &model.DatabaseData{
		Tables: make(map[string]*model.TableData),
	}

	// 如果设置为 schema_only（仅监控表结构），则直接跳过数据抓取
	if e.config.MonitorMode == "schema_only" {
		return result, nil
	}

	for tableName, tableSchema := range schema.Tables {
		tableData, err := e.extractTableData(ctx, tableSchema)
		if err != nil {
			return nil, fmt.Errorf("提取数据表 %s 数据失败: %w", tableName, err)
		}
		result.Tables[tableName] = tableData
	}

	return result, nil
}

// extractTableData 提取单张表的所有行数据
func (e *DataExtractor) extractTableData(ctx context.Context, tableSchema *model.TableSchema) (*model.TableData, error) {
	tableName := tableSchema.Name
	var pkCols []string
	if tableSchema.PrimaryKey != nil {
		pkCols = tableSchema.PrimaryKey.Columns
	}

	var sb strings.Builder
	sb.WriteString("SELECT * FROM ")
	sb.WriteString(sqlutil.EscapeIdentifier(tableName))

	// 若存在主键，按主键升序排序，保证多次抓取数据行顺序的一致性
	if len(pkCols) > 0 {
		sb.WriteString(" ORDER BY ")
		var escapedPKs []string
		for _, pk := range pkCols {
			escapedPKs = append(escapedPKs, sqlutil.EscapeIdentifier(pk))
		}
		sb.WriteString(strings.Join(escapedPKs, ", "))
	}

	// 若配置了每表最大拉取行数
	if e.config.MaxRowsPerTable > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", e.config.MaxRowsPerTable))
	}

	rows, err := e.db.QueryContext(ctx, sb.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("读取列名失败: %w", err)
	}

	tableData := &model.TableData{
		TableName:         tableName,
		PrimaryKeyColumns: pkCols,
		ColumnNames:       colNames,
		Rows:              make([]map[string]interface{}, 0),
		TotalRows:         0,
	}

	colCount := len(colNames)
	for rows.Next() {
		// 动态准备接收扫描结果的切片
		scanArgs := make([]interface{}, colCount)
		rawValues := make([]interface{}, colCount)
		for i := range rawValues {
			scanArgs[i] = &rawValues[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("扫描行数据失败: %w", err)
		}

		rowMap := make(map[string]interface{}, colCount)
		for i, col := range colNames {
			val := rawValues[i]
			// MySQL 驱动常将 varchar/text 等以 []byte 返回，转换处理
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = val
			}
		}

		tableData.Rows = append(tableData.Rows, rowMap)
		tableData.TotalRows++
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tableData, nil
}
