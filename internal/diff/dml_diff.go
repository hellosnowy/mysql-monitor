package diff

import (
	"encoding/json"
	"fmt"
	"sort"

	"sql-monitor/internal/model"
	"sql-monitor/internal/sqlgen"
)

// DiffData 对比两个版本的数据快照，生成 INSERT/UPDATE/DELETE 语句与汇总统计。
// targetSchema 提供目标版本的表结构与模式名，供方言生成限定名语句。
func DiffData(fromData, toData *model.DatabaseData, targetSchema *model.DatabaseSchema, em sqlgen.Emitter) ([]*model.TableDMLDiff, model.DMLDiffSummary) {
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
		if fromTable == nil && toTable == nil {
			continue
		}

		tableSchema := lookupTableSchema(targetSchema, tableName)
		tableDML := diffSingleTableData(tableName, fromTable, toTable, tableSchema, em)
		if tableDML.InsertCount > 0 || tableDML.UpdateCount > 0 || tableDML.DeleteCount > 0 {
			summary.InsertCount += tableDML.InsertCount
			summary.UpdateCount += tableDML.UpdateCount
			summary.DeleteCount += tableDML.DeleteCount
			diffs = append(diffs, tableDML)
		}
	}

	return diffs, summary
}

// lookupTableSchema 查找表结构；缺失时返回仅含表名的最小结构
func lookupTableSchema(schema *model.DatabaseSchema, tableName string) *model.TableSchema {
	if schema != nil {
		if t, ok := schema.Tables[tableName]; ok && t != nil {
			return t
		}
	}
	return &model.TableSchema{Name: tableName}
}

// diffSingleTableData 对比单张表在两个版本间的数据行差异
func diffSingleTableData(tableName string, fromTable, toTable *model.TableData, tableSchema *model.TableSchema, em sqlgen.Emitter) *model.TableDMLDiff {
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
				result.InsertStatements = append(result.InsertStatements, em.InsertRow(tableSchema, toTable.ColumnNames, toRow))
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
				result.UpdateStatements = append(result.UpdateStatements,
					em.UpdateRow(tableSchema, pkCols, changedCols, toRow, toTable.ColumnNames))
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
			result.DeleteStatements = append(result.DeleteStatements,
				em.DeleteRow(tableSchema, pkCols, fromTable.ColumnNames, row))
			result.DeleteCount++
		}
	}

	return result
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
