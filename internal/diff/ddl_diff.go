package diff

import (
	"fmt"
	"sort"
	"strings"

	"sql-monitor/internal/model"
	"sql-monitor/internal/sqlgen"
)

// DiffSchemas 对比两个版本的数据库表结构，生成 DDL 变更语句列表及汇总统计。
// SQL 语法由方言 Emitter 决定。
func DiffSchemas(fromSchema, toSchema *model.DatabaseSchema, em sqlgen.Emitter) ([]*model.TableDDLDiff, model.DDLDiffSummary) {
	var diffs []*model.TableDDLDiff
	var summary model.DDLDiffSummary

	if fromSchema == nil && toSchema == nil {
		return diffs, summary
	}
	if fromSchema == nil {
		fromSchema = &model.DatabaseSchema{Tables: make(map[string]*model.TableSchema)}
	}
	if toSchema == nil {
		toSchema = &model.DatabaseSchema{Tables: make(map[string]*model.TableSchema)}
	}

	// 收集所有涉及的表名并排序，保证脚本顺序稳定
	allTableNamesMap := make(map[string]struct{})
	for name := range fromSchema.Tables {
		allTableNamesMap[name] = struct{}{}
	}
	for name := range toSchema.Tables {
		allTableNamesMap[name] = struct{}{}
	}

	var allTableNames []string
	for name := range allTableNamesMap {
		allTableNames = append(allTableNames, name)
	}
	sort.Strings(allTableNames)

	for _, tableName := range allTableNames {
		fromTable, fromExists := fromSchema.Tables[tableName]
		toTable, toExists := toSchema.Tables[tableName]

		// 1. 新增表
		if !fromExists && toExists {
			summary.AddedTables++
			statements := em.CreateTable(toTable)
			if len(statements) > 0 {
				diffs = append(diffs, &model.TableDDLDiff{
					TableName:  tableName,
					DiffType:   "CREATE_TABLE",
					Statements: statements,
					Details:    []string{fmt.Sprintf("新建表 %s", tableName)},
				})
			}
			continue
		}

		// 2. 删除表
		if fromExists && !toExists {
			summary.DroppedTables++
			statements := em.DropTable(fromTable)
			if len(statements) > 0 {
				diffs = append(diffs, &model.TableDDLDiff{
					TableName:  tableName,
					DiffType:   "DROP_TABLE",
					Statements: statements,
					Details:    []string{fmt.Sprintf("删除表 %s", tableName)},
				})
			}
			continue
		}

		// 3. 表结构发生修改
		tableDiff, tableSummary := diffSingleTable(fromTable, toTable, em)
		if len(tableDiff.Statements) > 0 {
			summary.ModifiedTables++
			summary.AddedColumns += tableSummary.AddedColumns
			summary.DroppedColumns += tableSummary.DroppedColumns
			summary.ModifiedColumns += tableSummary.ModifiedColumns
			summary.AddedIndexes += tableSummary.AddedIndexes
			summary.DroppedIndexes += tableSummary.DroppedIndexes

			diffs = append(diffs, tableDiff)
		}
	}

	return diffs, summary
}

// diffSingleTable 比对两张同名表的字段、主键、索引与表属性变动。
// 语句顺序固定为：删索引 → 删主键 → 删列 → 增/改列 → 加主键 → 加索引 → 表属性/注释，
// 避免索引或主键引用已删除的列。
func diffSingleTable(fromTable, toTable *model.TableSchema, em sqlgen.Emitter) (*model.TableDDLDiff, model.DDLDiffSummary) {
	var statements []string
	var details []string
	var summary model.DDLDiffSummary

	fromCols := make(map[string]*model.ColumnSchema, len(fromTable.Columns))
	for _, c := range fromTable.Columns {
		fromCols[c.Name] = c
	}
	toCols := make(map[string]*model.ColumnSchema, len(toTable.Columns))
	for _, c := range toTable.Columns {
		toCols[c.Name] = c
	}
	fromIndexes := make(map[string]*model.IndexSchema, len(fromTable.Indexes))
	for _, idx := range fromTable.Indexes {
		fromIndexes[idx.Name] = idx
	}
	toIndexes := make(map[string]*model.IndexSchema, len(toTable.Indexes))
	for _, idx := range toTable.Indexes {
		toIndexes[idx.Name] = idx
	}

	// --- 1. 删除或变更的索引（先于列删除，避免索引仍引用被删列） ---
	for _, fromIdx := range fromTable.Indexes {
		toIdx, exists := toIndexes[fromIdx.Name]
		if exists && em.IndexEquivalent(fromIdx, toIdx) {
			continue
		}
		summary.DroppedIndexes++
		statements = append(statements, em.DropIndex(fromTable, fromIdx)...)
		if !exists {
			details = append(details, fmt.Sprintf("删除索引 %s", fromIdx.Name))
		} else {
			details = append(details, fmt.Sprintf("变更索引 %s 定义", fromIdx.Name))
		}
	}

	// --- 2. 主键变更：先删除旧主键 ---
	pkChanged := !primaryKeyEquivalent(fromTable.PrimaryKey, toTable.PrimaryKey)
	if pkChanged && fromTable.PrimaryKey != nil && len(fromTable.PrimaryKey.Columns) > 0 {
		statements = append(statements, em.DropPrimaryKey(fromTable)...)
		details = append(details, "删除旧主键")
	}

	// --- 3. 删除字段 ---
	for _, fromCol := range fromTable.Columns {
		if _, exists := toCols[fromCol.Name]; exists {
			continue
		}
		summary.DroppedColumns++
		statements = append(statements, em.DropColumn(fromTable, fromCol)...)
		details = append(details, fmt.Sprintf("删除字段 %s", fromCol.Name))
	}

	// --- 4. 新增字段（按目标表列顺序，MySQL 会保留位置） ---
	for _, toCol := range toTable.Columns {
		if _, exists := fromCols[toCol.Name]; exists {
			continue
		}
		summary.AddedColumns++
		statements = append(statements, em.AddColumn(toTable, toCol)...)
		details = append(details, fmt.Sprintf("新增字段 %s (%s)", toCol.Name, toCol.ColumnType))
	}

	// --- 5. 修改字段定义 ---
	for _, toCol := range toTable.Columns {
		fromCol, exists := fromCols[toCol.Name]
		if !exists || em.ColumnEquivalent(fromCol, toCol) {
			continue
		}
		summary.ModifiedColumns++
		statements = append(statements, em.ModifyColumn(toTable, fromCol, toCol)...)
		details = append(details, fmt.Sprintf("修改字段 %s 定义 (%s -> %s)", toCol.Name, fromCol.ColumnType, toCol.ColumnType))
	}

	// --- 6. 新增或变更后的主键 ---
	if pkChanged && toTable.PrimaryKey != nil && len(toTable.PrimaryKey.Columns) > 0 {
		statements = append(statements, em.AddPrimaryKey(toTable)...)
		details = append(details, fmt.Sprintf("新增/变更主键 (%s)", strings.Join(toTable.PrimaryKey.Columns, ", ")))
	}

	// --- 7. 新增或变更后的索引 ---
	for _, toIdx := range toTable.Indexes {
		fromIdx, exists := fromIndexes[toIdx.Name]
		if exists && em.IndexEquivalent(fromIdx, toIdx) {
			continue
		}
		summary.AddedIndexes++
		statements = append(statements, em.AddIndex(toTable, toIdx)...)
		if !exists {
			details = append(details, fmt.Sprintf("新增索引 %s", toIdx.Name))
		} else {
			details = append(details, fmt.Sprintf("变更索引 %s 定义", toIdx.Name))
		}
	}

	// --- 8. 表级属性（存储引擎等，方言决定是否有语句） ---
	statements = append(statements, em.AlterTableOptions(fromTable, toTable)...)

	// --- 9. 表注释 ---
	if fromTable.Comment != toTable.Comment {
		statements = append(statements, em.SetTableComment(toTable, toTable.Comment)...)
		details = append(details, "更新表注释")
	}

	return &model.TableDDLDiff{
		TableName:  toTable.Name,
		DiffType:   "ALTER_TABLE",
		Statements: statements,
		Details:    details,
	}, summary
}

// primaryKeyEquivalent 比较两个主键定义是否等价（列顺序与约束名）
func primaryKeyEquivalent(a, b *model.PrimaryKeySchema) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Name != "" && b.Name != "" && a.Name != b.Name {
		return false
	}
	return equalStringSlices(a.Columns, b.Columns)
}
