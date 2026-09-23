package diff

import (
	"encoding/json"
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
	result := &model.TableDMLDiff{TableName: tableName}

	var pkCols []string
	if toTable != nil && len(toTable.PrimaryKeyColumns) > 0 {
		pkCols = toTable.PrimaryKeyColumns
	} else if fromTable != nil {
		pkCols = fromTable.PrimaryKeyColumns
	}
	if fromTable != nil && toTable != nil && !equalStringSlices(fromTable.PrimaryKeyColumns, toTable.PrimaryKeyColumns) {
		// 主键定义变化时不能用任一版本的主键错误配对，退化为整行比对。
		pkCols = nil
	}

	// 无主键表按行内容计数，不能用单值 map 吞掉重复行。
	keyTable := &model.TableData{PrimaryKeyColumns: pkCols}
	fromRows := make(map[string][]map[string]interface{})
	if fromTable != nil {
		for _, row := range fromTable.Rows {
			key := keyTable.GenerateRowKey(row)
			fromRows[key] = append(fromRows[key], row)
		}
	}

	matched := make(map[string]int, len(fromRows))
	pkSet := make(map[string]struct{}, len(pkCols))
	for _, col := range pkCols {
		pkSet[col] = struct{}{}
	}

	if toTable != nil {
		for _, toRow := range toTable.Rows {
			key := keyTable.GenerateRowKey(toRow)
			index := matched[key]
			if index >= len(fromRows[key]) {
				result.InsertStatements = append(result.InsertStatements, generateInsertStatement(tableName, toTable.ColumnNames, toRow))
				result.InsertCount++
				continue
			}

			fromRow := fromRows[key][index]
			matched[key] = index + 1
			if len(pkCols) == 0 {
				continue
			}

			changedCols := make(map[string]interface{})
			for _, col := range toTable.ColumnNames {
				if _, isPK := pkSet[col]; isPK {
					continue
				}
				if !isRowValueEqual(fromRow[col], toRow[col]) {
					changedCols[col] = toRow[col]
				}
			}
			if len(changedCols) > 0 {
				result.UpdateStatements = append(result.UpdateStatements, generateUpdateStatement(tableName, pkCols, changedCols, toRow))
				result.UpdateCount++
			}
		}
	}

	// 按来源快照顺序输出 DELETE，保持脚本可重复生成。
	if fromTable != nil {
		used := make(map[string]int, len(matched))
		for _, row := range fromTable.Rows {
			key := keyTable.GenerateRowKey(row)
			if used[key] < matched[key] {
				used[key]++
				continue
			}
			result.DeleteStatements = append(result.DeleteStatements, generateDeleteStatement(tableName, pkCols, fromTable.ColumnNames, row))
			result.DeleteCount++
		}
	}

	return result
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

	statement := fmt.Sprintf("DELETE FROM %s WHERE %s",
		sqlutil.EscapeIdentifier(tableName),
		strings.Join(whereConditions, " AND "))
	if len(pkCols) == 0 {
		// 无主键表可能有重复行，每条差异只删除一行。
		statement += " LIMIT 1"
	}
	return statement + ";"
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
	// 与行键使用同一规范，区分数字、字符串、NULL 等值。
	left, leftErr := json.Marshal(v1)
	right, rightErr := json.Marshal(v2)
	if leftErr == nil && rightErr == nil {
		return string(left) == string(right)
	}
	return fmt.Sprintf("%T:%#v", v1, v1) == fmt.Sprintf("%T:%#v", v2, v2)
}
