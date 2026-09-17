package diff

import (
	"fmt"
	"sort"
	"strings"

	"mysql-monitor/internal/model"
	"mysql-monitor/pkg/sqlutil"
)

// DiffData 对比两个版本的数据快照，生成 INSERT, UPDATE, DELETE 等 DML 语句与汇总统计
func DiffData(fromData, toData *model.DatabaseData) ([]*model.TableDMLDiff, model.DMLDiffSummary) {
	var diffs []*model.TableDMLDiff
	var summary model.DMLDiffSummary

	if fromData == nil && toData == nil {
		return diffs, summary
	}
	if fromData == nil {
		fromData = &model.DatabaseData{Tables: make(map[string]*model.TableData)}
	}
	if toData == nil {
		toData = &model.DatabaseData{Tables: make(map[string]*model.TableData)}
	}

	// 收集所有数据表名并排序
	tableNamesMap := make(map[string]struct{})
	for name := range fromData.Tables {
		tableNamesMap[name] = struct{}{}
	}
	for name := range toData.Tables {
		tableNamesMap[name] = struct{}{}
	}

	var tableNames []string
	for name := range tableNamesMap {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)

	for _, tableName := range tableNames {
		fromTable := fromData.Tables[tableName]
		toTable := toData.Tables[tableName]

		// 双方均无数据
		if fromTable == nil && toTable == nil {
			continue
		}

		tableDML := diffSingleTableData(tableName, fromTable, toTable)
		if tableDML.InsertCount > 0 || tableDML.UpdateCount > 0 || tableDML.DeleteCount > 0 {
			summary.InsertCount += tableDML.InsertCount
			summary.UpdateCount += tableDML.UpdateCount
			summary.DeleteCount += tableDML.DeleteCount
			diffs = append(diffs, tableDML)
		}
	}

	return diffs, summary
}

// diffSingleTableData 对比单张表在两个版本间的数据行差异
func diffSingleTableData(tableName string, fromTable, toTable *model.TableData) *model.TableDMLDiff {
	result := &model.TableDMLDiff{
		TableName: tableName,
	}

	// 确定主键列
	var pkCols []string
	if toTable != nil && len(toTable.PrimaryKeyColumns) > 0 {
		pkCols = toTable.PrimaryKeyColumns
	} else if fromTable != nil && len(fromTable.PrimaryKeyColumns) > 0 {
		pkCols = fromTable.PrimaryKeyColumns
	}

	// 构造行索引映射 (rowKey -> rowMap)
	fromRowsMap := make(map[string]map[string]interface{})
	if fromTable != nil {
		for _, row := range fromTable.Rows {
			key := fromTable.GenerateRowKey(row)
			fromRowsMap[key] = row
		}
	}

	toRowsMap := make(map[string]map[string]interface{})
	if toTable != nil {
		for _, row := range toTable.Rows {
			key := toTable.GenerateRowKey(row)
			toRowsMap[key] = row
		}
	}

	// 1. 检查新增行 (在 toTable 中但不在 fromTable)
	if toTable != nil {
		for _, row := range toTable.Rows {
			key := toTable.GenerateRowKey(row)
			if _, exists := fromRowsMap[key]; !exists {
				// 生成 INSERT 语句
				stmt := generateInsertStatement(tableName, toTable.ColumnNames, row)
				result.InsertStatements = append(result.InsertStatements, stmt)
				result.InsertCount++
			}
		}
	}

	// 2. 检查删除行 (在 fromTable 中但不在 toTable)
	if fromTable != nil {
		for _, row := range fromTable.Rows {
			key := fromTable.GenerateRowKey(row)
			if _, exists := toRowsMap[key]; !exists {
				// 生成 DELETE 语句
				stmt := generateDeleteStatement(tableName, pkCols, fromTable.ColumnNames, row)
				result.DeleteStatements = append(result.DeleteStatements, stmt)
				result.DeleteCount++
			}
		}
	}

	// 3. 检查修改行 (两边都存在同一个 rowKey，比对字段值)
	if toTable != nil && fromTable != nil {
		for _, toRow := range toTable.Rows {
			key := toTable.GenerateRowKey(rowKeySource(toTable, toRow))
			if fromRow, exists := fromRowsMap[key]; exists {
				changedCols := make(map[string]interface{})
				for _, col := range toTable.ColumnNames {
					// 忽略主键列自身
					if containsString(pkCols, col) {
						continue
					}
					fromVal := fromRow[col]
					toVal := toRow[col]
					if !isRowValueEqual(fromVal, toVal) {
						changedCols[col] = toVal
					}
				}

				if len(changedCols) > 0 {
					stmt := generateUpdateStatement(tableName, pkCols, changedCols, toRow)
					result.UpdateStatements = append(result.UpdateStatements, stmt)
					result.UpdateCount++
				}
			}
		}
	}

	return result
}

func rowKeySource(t *model.TableData, row map[string]interface{}) map[string]interface{} {
	return row
}

// generateInsertStatement 生成单行 INSERT 语句
func generateInsertStatement(tableName string, columns []string, row map[string]interface{}) string {
	var colNames []string
	var valStrs []string

	for _, col := range columns {
		val, exists := row[col]
		if !exists {
			continue
		}
		colNames = append(colNames, sqlutil.EscapeIdentifier(col))
		valStrs = append(valStrs, sqlutil.FormatSQLValue(val))
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		sqlutil.EscapeIdentifier(tableName),
		strings.Join(colNames, ", "),
		strings.Join(valStrs, ", "))
}

// generateDeleteStatement 生成单行 DELETE 语句
func generateDeleteStatement(tableName string, pkCols []string, allCols []string, row map[string]interface{}) string {
	var whereConditions []string

	// 优先根据主键生成 WHERE 条件
	if len(pkCols) > 0 {
		for _, pk := range pkCols {
			val := row[pk]
			if val == nil {
				whereConditions = append(whereConditions, fmt.Sprintf("%s IS NULL", sqlutil.EscapeIdentifier(pk)))
			} else {
				whereConditions = append(whereConditions, fmt.Sprintf("%s = %s", sqlutil.EscapeIdentifier(pk), sqlutil.FormatSQLValue(val)))
			}
		}
	} else {
		// 无主键表：以所有字段作为复合匹配条件
		for _, col := range allCols {
			val := row[col]
			if val == nil {
				whereConditions = append(whereConditions, fmt.Sprintf("%s IS NULL", sqlutil.EscapeIdentifier(col)))
			} else {
				whereConditions = append(whereConditions, fmt.Sprintf("%s = %s", sqlutil.EscapeIdentifier(col), sqlutil.FormatSQLValue(val)))
			}
		}
	}

	return fmt.Sprintf("DELETE FROM %s WHERE %s;",
		sqlutil.EscapeIdentifier(tableName),
		strings.Join(whereConditions, " AND "))
}

// generateUpdateStatement 生成单行 UPDATE 语句
func generateUpdateStatement(tableName string, pkCols []string, changedCols map[string]interface{}, fullRow map[string]interface{}) string {
	// 按列名字典序排序以保证确定性
	cols := make([]string, 0, len(changedCols))
	for col := range changedCols {
		cols = append(cols, col)
	}
	sort.Strings(cols)

	var setClauses []string
	for _, col := range cols {
		val := changedCols[col]
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", sqlutil.EscapeIdentifier(col), sqlutil.FormatSQLValue(val)))
	}

	var whereConditions []string
	if len(pkCols) > 0 {
		for _, pk := range pkCols {
			val := fullRow[pk]
			if val == nil {
				whereConditions = append(whereConditions, fmt.Sprintf("%s IS NULL", sqlutil.EscapeIdentifier(pk)))
			} else {
				whereConditions = append(whereConditions, fmt.Sprintf("%s = %s", sqlutil.EscapeIdentifier(pk), sqlutil.FormatSQLValue(val)))
			}
		}
	} else {
		// 无主键表退化为所有列
		for col, val := range fullRow {
			if _, changed := changedCols[col]; changed {
				continue
			}
			if val == nil {
				whereConditions = append(whereConditions, fmt.Sprintf("%s IS NULL", sqlutil.EscapeIdentifier(col)))
			} else {
				whereConditions = append(whereConditions, fmt.Sprintf("%s = %s", sqlutil.EscapeIdentifier(col), sqlutil.FormatSQLValue(val)))
			}
		}
	}

	return fmt.Sprintf("UPDATE %s SET %s WHERE %s;",
		sqlutil.EscapeIdentifier(tableName),
		strings.Join(setClauses, ", "),
		strings.Join(whereConditions, " AND "))
}

// isRowValueEqual 判断两列值是否等价
func isRowValueEqual(v1, v2 interface{}) bool {
	if v1 == nil && v2 == nil {
		return true
	}
	if v1 == nil || v2 == nil {
		return false
	}
	// 字符串比较
	return fmt.Sprintf("%v", v1) == fmt.Sprintf("%v", v2)
}

// containsString 判断切片中是否包含某字符串
func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
