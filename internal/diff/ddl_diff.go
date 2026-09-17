package diff

import (
	"fmt"
	"sort"
	"strings"

	"mysql-monitor/internal/model"
	"mysql-monitor/pkg/sqlutil"
)

// DiffSchemas 对比两个版本的数据库表结构，生成 DDL 变更语句列表及汇总统计
func DiffSchemas(fromSchema, toSchema *model.DatabaseSchema) ([]*model.TableDDLDiff, model.DDLDiffSummary) {
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

	// 收集所有涉及的表名并排序
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

		// 1. 新增表：在 toSchema 中存在，但 fromSchema 中不存在
		if !fromExists && toExists {
			summary.AddedTables++
			createSQL := toTable.CreateTableSQL
			if createSQL == "" {
				createSQL = buildCreateTableSQL(toTable)
			}
			if !strings.HasSuffix(strings.TrimSpace(createSQL), ";") {
				createSQL = strings.TrimSpace(createSQL) + ";"
			}

			diffs = append(diffs, &model.TableDDLDiff{
				TableName:  tableName,
				DiffType:   "CREATE_TABLE",
				Statements: []string{createSQL},
				Details:    []string{fmt.Sprintf("新建表 %s", tableName)},
			})
			continue
		}

		// 2. 删除表：在 fromSchema 中存在，但在 toSchema 中不存在
		if fromExists && !toExists {
			summary.DroppedTables++
			dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s;", sqlutil.EscapeIdentifier(tableName))
			diffs = append(diffs, &model.TableDDLDiff{
				TableName:  tableName,
				DiffType:   "DROP_TABLE",
				Statements: []string{dropSQL},
				Details:    []string{fmt.Sprintf("删除表 %s", tableName)},
			})
			continue
		}

		// 3. 表结构发生修改：两版本中均存在
		tableDiff, tableSummary := diffSingleTable(fromTable, toTable)
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

// diffSingleTable 比对两张同名表的字段、主键、索引与表属性变动
func diffSingleTable(fromTable, toTable *model.TableSchema) (*model.TableDDLDiff, model.DDLDiffSummary) {
	var statements []string
	var details []string
	var summary model.DDLDiffSummary

	tableName := toTable.Name
	escapedTable := sqlutil.EscapeIdentifier(tableName)

	// --- 1. 字段比对 ---
	fromCols := make(map[string]*model.ColumnSchema)
	for _, c := range fromTable.Columns {
		fromCols[c.Name] = c
	}
	toCols := make(map[string]*model.ColumnSchema)
	for _, c := range toTable.Columns {
		toCols[c.Name] = c
	}

	// 1.1 检查删除的字段
	for _, fromCol := range fromTable.Columns {
		if _, exists := toCols[fromCol.Name]; !exists {
			summary.DroppedColumns++
			stmt := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", escapedTable, sqlutil.EscapeIdentifier(fromCol.Name))
			statements = append(statements, stmt)
			details = append(details, fmt.Sprintf("删除字段 %s", fromCol.Name))
		}
	}

	// 1.2 检查新增的字段和修改的字段（按 toTable 中的顺序比对）
	for i, toCol := range toTable.Columns {
		fromCol, exists := fromCols[toCol.Name]
		if !exists {
			// 新增字段
			summary.AddedColumns++
			colSpec := sqlutil.FormatColumnSpec(toCol)
			positionSpec := ""
			if i == 0 {
				positionSpec = " FIRST"
			} else {
				prevColName := toTable.Columns[i-1].Name
				positionSpec = fmt.Sprintf(" AFTER %s", sqlutil.EscapeIdentifier(prevColName))
			}
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s%s;", escapedTable, colSpec, positionSpec)
			statements = append(statements, stmt)
			details = append(details, fmt.Sprintf("新增字段 %s (%s)", toCol.Name, toCol.ColumnType))
		} else {
			// 检查现有字段是否发生变化
			if isColumnModified(fromCol, toCol) {
				summary.ModifiedColumns++
				colSpec := sqlutil.FormatColumnSpec(toCol)
				stmt := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s;", escapedTable, colSpec)
				statements = append(statements, stmt)
				details = append(details, fmt.Sprintf("修改字段 %s 定义 (%s -> %s)", toCol.Name, fromCol.ColumnType, toCol.ColumnType))
			}
		}
	}

	// --- 2. 主键比对 ---
	fromPKCols := getPKColumns(fromTable)
	toPKCols := getPKColumns(toTable)

	if !equalStringSlices(fromPKCols, toPKCols) {
		if len(fromPKCols) > 0 && len(toPKCols) == 0 {
			// 删除主键
			stmt := fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY;", escapedTable)
			statements = append(statements, stmt)
			details = append(details, "删除主键")
		} else if len(fromPKCols) == 0 && len(toPKCols) > 0 {
			// 新增主键
			var escapedPKs []string
			for _, pk := range toPKCols {
				escapedPKs = append(escapedPKs, sqlutil.EscapeIdentifier(pk))
			}
			stmt := fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", escapedTable, strings.Join(escapedPKs, ", "))
			statements = append(statements, stmt)
			details = append(details, fmt.Sprintf("新增主键 (%s)", strings.Join(toPKCols, ", ")))
		} else {
			// 修改主键（先 DROP 后 ADD）
			var escapedPKs []string
			for _, pk := range toPKCols {
				escapedPKs = append(escapedPKs, sqlutil.EscapeIdentifier(pk))
			}
			stmt := fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY, ADD PRIMARY KEY (%s);", escapedTable, strings.Join(escapedPKs, ", "))
			statements = append(statements, stmt)
			details = append(details, fmt.Sprintf("变更主键为 (%s)", strings.Join(toPKCols, ", ")))
		}
	}

	// --- 3. 索引比对（不含主键） ---
	fromIndexes := make(map[string]*model.IndexSchema)
	for _, idx := range fromTable.Indexes {
		fromIndexes[idx.Name] = idx
	}
	toIndexes := make(map[string]*model.IndexSchema)
	for _, idx := range toTable.Indexes {
		toIndexes[idx.Name] = idx
	}

	// 3.1 检查删除的索引
	for _, fromIdx := range fromTable.Indexes {
		if _, exists := toIndexes[fromIdx.Name]; !exists {
			summary.DroppedIndexes++
			stmt := fmt.Sprintf("ALTER TABLE %s DROP INDEX %s;", escapedTable, sqlutil.EscapeIdentifier(fromIdx.Name))
			statements = append(statements, stmt)
			details = append(details, fmt.Sprintf("删除索引 %s", fromIdx.Name))
		}
	}

	// 3.2 检查新增与修改的索引
	for _, toIdx := range toTable.Indexes {
		fromIdx, exists := fromIndexes[toIdx.Name]
		if !exists {
			// 新增索引
			summary.AddedIndexes++
			idxSpec := formatIndexSpec(toIdx)
			stmt := fmt.Sprintf("ALTER TABLE %s ADD %s;", escapedTable, idxSpec)
			statements = append(statements, stmt)
			details = append(details, fmt.Sprintf("新增索引 %s", toIdx.Name))
		} else if isIndexModified(fromIdx, toIdx) {
			// 修改索引（先删后加）
			summary.DroppedIndexes++
			summary.AddedIndexes++
			idxSpec := formatIndexSpec(toIdx)
			stmt := fmt.Sprintf("ALTER TABLE %s DROP INDEX %s, ADD %s;", escapedTable, sqlutil.EscapeIdentifier(toIdx.Name), idxSpec)
			statements = append(statements, stmt)
			details = append(details, fmt.Sprintf("变更索引 %s 定义", toIdx.Name))
		}
	}

	// --- 4. 表属性比对（Engine, Comment） ---
	if toTable.Engine != "" && fromTable.Engine != "" && !strings.EqualFold(fromTable.Engine, toTable.Engine) {
		stmt := fmt.Sprintf("ALTER TABLE %s ENGINE = %s;", escapedTable, toTable.Engine)
		statements = append(statements, stmt)
		details = append(details, fmt.Sprintf("变更存储引擎 %s -> %s", fromTable.Engine, toTable.Engine))
	}

	if toTable.Comment != fromTable.Comment && toTable.Comment != "" {
		stmt := fmt.Sprintf("ALTER TABLE %s COMMENT = %s;", escapedTable, sqlutil.EscapeStringLiteral(toTable.Comment))
		statements = append(statements, stmt)
		details = append(details, "更新表注释")
	}

	return &model.TableDDLDiff{
		TableName:  tableName,
		DiffType:   "ALTER_TABLE",
		Statements: statements,
		Details:    details,
	}, summary
}

// isColumnModified 判断字段是否有实质性变更
func isColumnModified(c1, c2 *model.ColumnSchema) bool {
	if !strings.EqualFold(c1.ColumnType, c2.ColumnType) {
		return true
	}
	if !strings.EqualFold(c1.IsNullable, c2.IsNullable) {
		return true
	}
	if !equalDefault(c1.ColumnDefault, c2.ColumnDefault) {
		return true
	}
	if !strings.EqualFold(c1.Extra, c2.Extra) {
		return true
	}
	if c1.Comment != c2.Comment {
		return true
	}
	return false
}

// equalDefault 比较两个默认值指针是否等价
func equalDefault(d1, d2 *string) bool {
	if d1 == nil && d2 == nil {
		return true
	}
	if d1 == nil || d2 == nil {
		return false
	}
	return *d1 == *d2
}

// isIndexModified 判断两个索引定义是否发生变动
func isIndexModified(i1, i2 *model.IndexSchema) bool {
	if i1.NonUnique != i2.NonUnique {
		return true
	}
	if len(i1.Columns) != len(i2.Columns) {
		return true
	}
	for idx := range i1.Columns {
		c1 := i1.Columns[idx]
		c2 := i2.Columns[idx]
		if c1.Name != c2.Name {
			return true
		}
		if (c1.SubPart == nil) != (c2.SubPart == nil) {
			return true
		}
		if c1.SubPart != nil && c2.SubPart != nil && *c1.SubPart != *c2.SubPart {
			return true
		}
	}
	return false
}

// formatIndexSpec 格式化索引规格定义
func formatIndexSpec(idx *model.IndexSchema) string {
	var sb strings.Builder
	if !idx.NonUnique {
		sb.WriteString("UNIQUE INDEX ")
	} else if strings.EqualFold(idx.IndexType, "FULLTEXT") {
		sb.WriteString("FULLTEXT INDEX ")
	} else {
		sb.WriteString("INDEX ")
	}
	sb.WriteString(sqlutil.EscapeIdentifier(idx.Name))
	sb.WriteString(" (")

	var colParts []string
	for _, col := range idx.Columns {
		colStr := sqlutil.EscapeIdentifier(col.Name)
		if col.SubPart != nil && *col.SubPart > 0 {
			colStr = fmt.Sprintf("%s(%d)", colStr, *col.SubPart)
		}
		colParts = append(colParts, colStr)
	}
	sb.WriteString(strings.Join(colParts, ", "))
	sb.WriteString(")")

	return sb.String()
}

// getPKColumns 获取表的主键列列表
func getPKColumns(t *model.TableSchema) []string {
	if t.PrimaryKey == nil {
		return nil
	}
	return t.PrimaryKey.Columns
}

// equalStringSlices 比较两个字符串切片是否完全相等
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// buildCreateTableSQL 当无原生 SHOW CREATE TABLE 时，根据字段和索引构造 CREATE TABLE 语句
func buildCreateTableSQL(table *model.TableSchema) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", sqlutil.EscapeIdentifier(table.Name)))

	var defs []string
	for _, col := range table.Columns {
		defs = append(defs, "  "+sqlutil.FormatColumnSpec(col))
	}

	if table.PrimaryKey != nil && len(table.PrimaryKey.Columns) > 0 {
		var pkCols []string
		for _, col := range table.PrimaryKey.Columns {
			pkCols = append(pkCols, sqlutil.EscapeIdentifier(col))
		}
		defs = append(defs, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	for _, idx := range table.Indexes {
		defs = append(defs, "  "+formatIndexSpec(idx))
	}

	sb.WriteString(strings.Join(defs, ",\n"))
	sb.WriteString("\n)")

	if table.Engine != "" {
		sb.WriteString(fmt.Sprintf(" ENGINE=%s", table.Engine))
	}
	if table.Collation != "" {
		sb.WriteString(fmt.Sprintf(" COLLATE=%s", table.Collation))
	}
	if table.Comment != "" {
		sb.WriteString(fmt.Sprintf(" COMMENT=%s", sqlutil.EscapeStringLiteral(table.Comment)))
	}
	sb.WriteString(";")

	return sb.String()
}
