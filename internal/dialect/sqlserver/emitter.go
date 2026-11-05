package sqlserver

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"sql-monitor/internal/model"
	"sql-monitor/internal/sqlgen"
)

// Emitter Microsoft SQL Server SQL 发射器
type Emitter struct{}

var _ sqlgen.Emitter = (*Emitter)(nil)

// Vendor 返回方言标识
func (e *Emitter) Vendor() model.Vendor { return model.VendorSQLServer }

// QuoteIdentifier 使用方括号包裹标识符，内部右方括号双写转义
func (e *Emitter) QuoteIdentifier(name string) string { return quoteIdentifier(name) }

// QualifyTable 生成 [schema].[table] 形式的两段式表名
func (e *Emitter) QualifyTable(schema, table string) string { return qualifyTable(schema, table) }

// QuoteStringLiteral 生成 SQL Server Unicode 字符串字面量
func (e *Emitter) QuoteStringLiteral(s string) string { return quoteStringLiteral(s) }

// FormatValue 生成 SQL Server 值字面量
func (e *Emitter) FormatValue(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case json.Number:
		// 保持原始数值文本，避免浮点精度丢失
		return val.String()
	case string:
		return quoteStringLiteral(val)
	case []byte:
		return "0x" + hex.EncodeToString(val)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%v", val)
	case time.Time:
		return fmt.Sprintf("'%s'", val.Format("2006-01-02T15:04:05.9999999"))
	default:
		return quoteStringLiteral(fmt.Sprintf("%v", val))
	}
}

// FormatColumnSpec 生成 SQL Server 列定义
func (e *Emitter) FormatColumnSpec(col *model.ColumnSchema) string {
	if col == nil {
		return ""
	}

	// 计算列使用 [col] AS (表达式)，不能再声明类型与默认值
	if col.IsGenerated && strings.TrimSpace(col.GenerationExpression) != "" {
		return fmt.Sprintf("%s AS %s", quoteIdentifier(col.Name), col.GenerationExpression)
	}

	var sb strings.Builder
	sb.WriteString(quoteIdentifier(col.Name))
	sb.WriteString(" ")
	sb.WriteString(col.ColumnType)
	if col.CollationName != "" {
		sb.WriteString(" COLLATE ")
		sb.WriteString(col.CollationName)
	}
	if col.IsIdentity {
		sb.WriteString(" IDENTITY(1,1)")
	}
	if strings.EqualFold(col.IsNullable, "NO") {
		sb.WriteString(" NOT NULL")
	} else {
		sb.WriteString(" NULL")
	}
	if col.ColumnDefault != nil {
		if col.DefaultConstraintName != "" {
			sb.WriteString(" CONSTRAINT ")
			sb.WriteString(quoteIdentifier(col.DefaultConstraintName))
		}
		sb.WriteString(" DEFAULT ")
		sb.WriteString(*col.ColumnDefault)
	}
	return sb.String()
}

// CreateTable 依据元数据重建建表语句，索引与注释作为独立语句输出
func (e *Emitter) CreateTable(t *model.TableSchema) []string {
	if t == nil {
		return nil
	}

	defs := make([]string, 0, len(t.Columns)+1+len(t.Indexes))
	for _, col := range t.Columns {
		defs = append(defs, "  "+e.FormatColumnSpec(col))
	}
	if t.PrimaryKey != nil && len(t.PrimaryKey.Columns) > 0 {
		defs = append(defs, "  "+e.primaryKeySpec(t))
	}
	// 唯一约束属于表级约束，随建表语句一起创建
	for _, idx := range t.Indexes {
		if idx != nil && idx.IsConstraint {
			defs = append(defs, "  "+e.uniqueConstraintSpec(idx))
		}
	}

	statements := []string{fmt.Sprintf("CREATE TABLE %s (\n%s\n);",
		e.QualifyTable(t.Schema, t.Name), strings.Join(defs, ",\n"))}

	// 普通索引必须使用独立的 CREATE INDEX 语句
	for _, idx := range t.Indexes {
		if idx == nil || idx.IsConstraint {
			continue
		}
		statements = append(statements, e.createIndexStatement(t, idx))
	}

	if t.Comment != "" {
		statements = append(statements, e.tableCommentStatements(t, t.Comment)...)
	}
	for _, col := range t.Columns {
		if col != nil && col.Comment != "" {
			statements = append(statements, e.columnCommentStatements(t, col.Name, col.Comment)...)
		}
	}
	return statements
}

// DropTable 删除表
func (e *Emitter) DropTable(t *model.TableSchema) []string {
	if t == nil {
		return nil
	}
	return []string{fmt.Sprintf("DROP TABLE IF EXISTS %s;", e.QualifyTable(t.Schema, t.Name))}
}

// AddColumn 新增字段（SQL Server 只能追加到表末尾）
func (e *Emitter) AddColumn(t *model.TableSchema, col *model.ColumnSchema) []string {
	if t == nil || col == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ADD %s;",
		e.QualifyTable(t.Schema, t.Name), e.FormatColumnSpec(col))}
}

// DropColumn 删除字段；存在命名默认约束时先删除约束
func (e *Emitter) DropColumn(t *model.TableSchema, col *model.ColumnSchema) []string {
	if t == nil || col == nil {
		return nil
	}
	table := e.QualifyTable(t.Schema, t.Name)
	var statements []string
	if col.DefaultConstraintName != "" {
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;",
			table, e.QuoteIdentifier(col.DefaultConstraintName)))
	}
	statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;",
		table, e.QuoteIdentifier(col.Name)))
	return statements
}

// ModifyColumn 修改字段类型、可空性、默认约束、注释；IDENTITY/计算列变更给出警告
func (e *Emitter) ModifyColumn(t *model.TableSchema, from, to *model.ColumnSchema) []string {
	if t == nil || to == nil {
		return nil
	}
	table := e.QualifyTable(t.Schema, t.Name)
	var statements []string

	if from != nil && from.IsIdentity != to.IsIdentity {
		statements = append(statements, fmt.Sprintf(
			"-- 警告: SQL Server 不支持通过 ALTER COLUMN 修改列 %s 的 IDENTITY 属性，请手动重建该列;",
			e.QuoteIdentifier(to.Name)))
	}

	generatedChanged := from != nil && (from.IsGenerated != to.IsGenerated ||
		strings.TrimSpace(from.GenerationExpression) != strings.TrimSpace(to.GenerationExpression))
	if generatedChanged {
		statements = append(statements, fmt.Sprintf(
			"-- 警告: SQL Server 不支持通过 ALTER COLUMN 修改列 %s 的计算列表达式，请手动重建该列;",
			e.QuoteIdentifier(to.Name)))
	}

	typeChanged := from == nil || !strings.EqualFold(strings.TrimSpace(from.ColumnType), strings.TrimSpace(to.ColumnType)) ||
		!strings.EqualFold(from.CollationName, to.CollationName)
	nullableChanged := from == nil || nullableValue(from.IsNullable) != nullableValue(to.IsNullable)
	// 计算列不能直接 ALTER COLUMN，变更时只输出警告
	if (typeChanged || nullableChanged) && !to.IsGenerated && !generatedChanged {
		nullability := " NULL"
		if strings.EqualFold(to.IsNullable, "NO") {
			nullability = " NOT NULL"
		}
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s%s;",
			table, e.QuoteIdentifier(to.Name), e.columnTypeClause(to), nullability))
	}

	// 默认约束变更：先删除旧约束，再按目标定义重建
	if !defaultsEqual(columnDefault(from), to.ColumnDefault) {
		if from != nil && from.ColumnDefault != nil {
			if from.DefaultConstraintName != "" {
				statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;",
					table, e.QuoteIdentifier(from.DefaultConstraintName)))
			} else {
				statements = append(statements, fmt.Sprintf(
					"-- 警告: 无法确定列 %s 的默认约束名称，请手动删除旧默认约束;",
					e.QuoteIdentifier(to.Name)))
			}
		}
		if to.ColumnDefault != nil {
			if to.DefaultConstraintName != "" {
				statements = append(statements, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s DEFAULT %s FOR %s;",
					table, e.QuoteIdentifier(to.DefaultConstraintName), *to.ColumnDefault, e.QuoteIdentifier(to.Name)))
			} else {
				statements = append(statements, fmt.Sprintf("ALTER TABLE %s ADD DEFAULT %s FOR %s;",
					table, *to.ColumnDefault, e.QuoteIdentifier(to.Name)))
			}
		}
	}

	if from != nil && from.Comment != to.Comment {
		statements = append(statements, e.columnCommentStatements(t, to.Name, to.Comment)...)
	}
	return statements
}

// AddPrimaryKey 新增主键约束
func (e *Emitter) AddPrimaryKey(t *model.TableSchema) []string {
	if t == nil || t.PrimaryKey == nil || len(t.PrimaryKey.Columns) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ADD %s;",
		e.QualifyTable(t.Schema, t.Name), e.primaryKeySpec(t))}
}

// DropPrimaryKey 删除主键约束
func (e *Emitter) DropPrimaryKey(t *model.TableSchema) []string {
	if t == nil || t.PrimaryKey == nil {
		return nil
	}
	name := t.PrimaryKey.Name
	if name == "" {
		name = "PK_" + t.Name
	}
	return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;",
		e.QualifyTable(t.Schema, t.Name), e.QuoteIdentifier(name))}
}

// AddIndex 新增索引；唯一约束使用 ALTER TABLE ADD CONSTRAINT
func (e *Emitter) AddIndex(t *model.TableSchema, idx *model.IndexSchema) []string {
	if t == nil || idx == nil {
		return nil
	}
	if idx.IsConstraint {
		return []string{fmt.Sprintf("ALTER TABLE %s ADD %s;",
			e.QualifyTable(t.Schema, t.Name), e.uniqueConstraintSpec(idx))}
	}
	return []string{e.createIndexStatement(t, idx)}
}

// DropIndex 删除索引；唯一约束使用 ALTER TABLE DROP CONSTRAINT
func (e *Emitter) DropIndex(t *model.TableSchema, idx *model.IndexSchema) []string {
	if t == nil || idx == nil {
		return nil
	}
	table := e.QualifyTable(t.Schema, t.Name)
	if idx.IsConstraint {
		return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;",
			table, e.QuoteIdentifier(idx.Name))}
	}
	return []string{fmt.Sprintf("DROP INDEX %s ON %s;", e.QuoteIdentifier(idx.Name), table)}
}

// SetTableComment 通过扩展属性维护表注释
func (e *Emitter) SetTableComment(t *model.TableSchema, comment string) []string {
	if t == nil {
		return nil
	}
	return e.tableCommentStatements(t, comment)
}

// AlterTableOptions SQL Server 无存储引擎等表级选项
func (e *Emitter) AlterTableOptions(from, to *model.TableSchema) []string { return nil }

// ColumnEquivalent 判断字段是否有实质性变更
func (e *Emitter) ColumnEquivalent(a, b *model.ColumnSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if !strings.EqualFold(strings.TrimSpace(a.ColumnType), strings.TrimSpace(b.ColumnType)) ||
		!strings.EqualFold(a.CollationName, b.CollationName) ||
		nullableValue(a.IsNullable) != nullableValue(b.IsNullable) ||
		a.IsIdentity != b.IsIdentity ||
		a.IsGenerated != b.IsGenerated ||
		strings.TrimSpace(a.GenerationExpression) != strings.TrimSpace(b.GenerationExpression) ||
		a.Comment != b.Comment {
		return false
	}
	return defaultsEqual(a.ColumnDefault, b.ColumnDefault)
}

// IndexEquivalent 判断索引定义是否等价
func (e *Emitter) IndexEquivalent(a, b *model.IndexSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.NonUnique != b.NonUnique ||
		indexClustered(a) != indexClustered(b) ||
		a.IsConstraint != b.IsConstraint ||
		len(a.Columns) != len(b.Columns) {
		return false
	}
	for i := range a.Columns {
		if a.Columns[i].Name != b.Columns[i].Name {
			return false
		}
	}
	if !equalStrings(a.IncludeColumns, b.IncludeColumns) {
		return false
	}
	return strings.TrimSpace(a.Filter) == strings.TrimSpace(b.Filter)
}

// InsertRow 生成单行 INSERT 语句
func (e *Emitter) InsertRow(t *model.TableSchema, columns []string, row map[string]interface{}) string {
	if t == nil {
		return ""
	}
	var colNames, valStrs []string
	for _, col := range columns {
		val, exists := row[col]
		if !exists {
			continue
		}
		colNames = append(colNames, e.QuoteIdentifier(col))
		valStrs = append(valStrs, e.FormatValue(val))
	}

	table := e.QualifyTable(t.Schema, t.Name)
	if len(colNames) == 0 {
		return fmt.Sprintf("INSERT INTO %s DEFAULT VALUES;", table)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		table, strings.Join(colNames, ", "), strings.Join(valStrs, ", "))
}

// UpdateRow 生成单行 UPDATE 语句；无主键时按快照列顺序匹配全部列
func (e *Emitter) UpdateRow(t *model.TableSchema, pkCols []string, changed map[string]interface{}, fullRow map[string]interface{}, allCols []string) string {
	if t == nil {
		return ""
	}
	cols := make([]string, 0, len(changed))
	for col := range changed {
		cols = append(cols, col)
	}
	sort.Strings(cols)

	setClauses := make([]string, 0, len(cols))
	for _, col := range cols {
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", e.QuoteIdentifier(col), e.FormatValue(changed[col])))
	}

	var where []string
	if len(pkCols) > 0 {
		for _, pk := range pkCols {
			where = append(where, e.equality(e.QuoteIdentifier(pk), fullRow[pk]))
		}
	} else {
		for _, col := range allCols {
			if _, isChanged := changed[col]; isChanged {
				continue
			}
			where = append(where, e.equality(e.QuoteIdentifier(col), fullRow[col]))
		}
	}

	if len(setClauses) == 0 || len(where) == 0 {
		return ""
	}
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s;",
		e.QualifyTable(t.Schema, t.Name), strings.Join(setClauses, ", "), strings.Join(where, " AND "))
}

// DeleteRow 生成单行 DELETE 语句；无主键时使用 DELETE TOP (1) 保证只删除一行
func (e *Emitter) DeleteRow(t *model.TableSchema, pkCols []string, allCols []string, row map[string]interface{}) string {
	if t == nil {
		return ""
	}
	var where []string
	if len(pkCols) > 0 {
		for _, pk := range pkCols {
			where = append(where, e.equality(e.QuoteIdentifier(pk), row[pk]))
		}
	} else {
		for _, col := range allCols {
			where = append(where, e.equality(e.QuoteIdentifier(col), row[col]))
		}
	}
	if len(where) == 0 {
		return ""
	}

	table := e.QualifyTable(t.Schema, t.Name)
	if len(pkCols) == 0 {
		return fmt.Sprintf("DELETE TOP (1) FROM %s WHERE %s;", table, strings.Join(where, " AND "))
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s;", table, strings.Join(where, " AND "))
}

// TableDMLBegin 显式插入 IDENTITY 列时需要开启 IDENTITY_INSERT
func (e *Emitter) TableDMLBegin(t *model.TableSchema, hasInserts bool) []string {
	if !hasInserts || t == nil || !hasIdentityColumn(t) {
		return nil
	}
	return []string{fmt.Sprintf("SET IDENTITY_INSERT %s ON;", e.QualifyTable(t.Schema, t.Name))}
}

// TableDMLEnd 关闭 IDENTITY_INSERT
func (e *Emitter) TableDMLEnd(t *model.TableSchema, hasInserts bool) []string {
	if !hasInserts || t == nil || !hasIdentityColumn(t) {
		return nil
	}
	return []string{fmt.Sprintf("SET IDENTITY_INSERT %s OFF;", e.QualifyTable(t.Schema, t.Name))}
}

// SessionBegin 开启会话级事务保护
func (e *Emitter) SessionBegin() []string {
	return []string{"SET XACT_ABORT ON;", "BEGIN TRANSACTION;"}
}

// SessionEnd 提交事务
func (e *Emitter) SessionEnd() []string { return []string{"COMMIT;"} }

// DDLBegin SQL Server DDL 已由 SessionBegin 包裹事务
func (e *Emitter) DDLBegin() []string { return nil }

// DDLEnd SQL Server DDL 已由 SessionEnd 提交
func (e *Emitter) DDLEnd() []string { return nil }

// DMLBegin SQL Server DML 已由 SessionBegin 包裹事务
func (e *Emitter) DMLBegin() []string { return nil }

// DMLEnd SQL Server DML 已由 SessionEnd 提交
func (e *Emitter) DMLEnd() []string { return nil }

// quoteIdentifier 使用方括号包裹标识符，内部右方括号双写转义
func quoteIdentifier(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

// qualifyTable 生成 [schema].[table]；未指定模式时默认 dbo
func qualifyTable(schema, table string) string {
	if strings.TrimSpace(schema) == "" {
		schema = "dbo"
	}
	return quoteIdentifier(schema) + "." + quoteIdentifier(table)
}

// quoteStringLiteral 生成 Unicode 字符串字面量，单引号双写转义
func quoteStringLiteral(s string) string {
	return "N'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// equality 生成等值条件，NULL 使用 IS NULL
func (e *Emitter) equality(quotedCol string, val interface{}) string {
	if val == nil {
		return quotedCol + " IS NULL"
	}
	return fmt.Sprintf("%s = %s", quotedCol, e.FormatValue(val))
}

// columnTypeClause 生成 ALTER COLUMN 使用的类型与排序规则片段
func (e *Emitter) columnTypeClause(col *model.ColumnSchema) string {
	clause := col.ColumnType
	if col.CollationName != "" {
		clause += " COLLATE " + col.CollationName
	}
	return clause
}

// primaryKeySpec 生成主键约束定义
func (e *Emitter) primaryKeySpec(t *model.TableSchema) string {
	name := t.PrimaryKey.Name
	if name == "" {
		name = "PK_" + t.Name
	}
	clustering := "CLUSTERED"
	if t.PrimaryKey.IsClustered != nil && !*t.PrimaryKey.IsClustered {
		clustering = "NONCLUSTERED"
	}
	return fmt.Sprintf("CONSTRAINT %s PRIMARY KEY %s (%s)",
		e.QuoteIdentifier(name), clustering, e.indexKeyColumns(t.PrimaryKey.Columns))
}

// uniqueConstraintSpec 生成唯一约束定义
func (e *Emitter) uniqueConstraintSpec(idx *model.IndexSchema) string {
	var sb strings.Builder
	sb.WriteString("CONSTRAINT ")
	sb.WriteString(e.QuoteIdentifier(idx.Name))
	sb.WriteString(" UNIQUE")
	if keyword := indexClusteredKeyword(idx); keyword != "" {
		sb.WriteString(" ")
		sb.WriteString(keyword)
	}
	sb.WriteString(" (")
	sb.WriteString(e.indexColumnNames(idx.Columns))
	sb.WriteString(")")
	return sb.String()
}

// createIndexStatement 生成独立 CREATE INDEX 语句
func (e *Emitter) createIndexStatement(t *model.TableSchema, idx *model.IndexSchema) string {
	var sb strings.Builder
	sb.WriteString("CREATE ")
	if !idx.NonUnique {
		sb.WriteString("UNIQUE ")
	}
	if keyword := indexClusteredKeyword(idx); keyword != "" {
		sb.WriteString(keyword)
		sb.WriteString(" ")
	}
	sb.WriteString("INDEX ")
	sb.WriteString(e.QuoteIdentifier(idx.Name))
	sb.WriteString(" ON ")
	sb.WriteString(e.QualifyTable(t.Schema, t.Name))
	sb.WriteString(" (")
	sb.WriteString(e.indexColumnNames(idx.Columns))
	sb.WriteString(")")
	if len(idx.IncludeColumns) > 0 {
		include := make([]string, 0, len(idx.IncludeColumns))
		for _, col := range idx.IncludeColumns {
			include = append(include, e.QuoteIdentifier(col))
		}
		sb.WriteString(" INCLUDE (")
		sb.WriteString(strings.Join(include, ", "))
		sb.WriteString(")")
	}
	if strings.TrimSpace(idx.Filter) != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(idx.Filter)
	}
	sb.WriteString(";")
	return sb.String()
}

// indexColumnNames 将索引列名转换为带方括号的列表
func (e *Emitter) indexColumnNames(columns []model.IndexColumn) string {
	names := make([]string, 0, len(columns))
	for _, col := range columns {
		names = append(names, e.QuoteIdentifier(col.Name))
	}
	return strings.Join(names, ", ")
}

// indexKeyColumns 将主键列名转换为带方括号的列表
func (e *Emitter) indexKeyColumns(columns []string) string {
	names := make([]string, 0, len(columns))
	for _, col := range columns {
		names = append(names, e.QuoteIdentifier(col))
	}
	return strings.Join(names, ", ")
}

// indexClusteredKeyword 返回索引的聚集方式关键字，未知时返回空串
func indexClusteredKeyword(idx *model.IndexSchema) string {
	if idx.IsClustered != nil {
		if *idx.IsClustered {
			return "CLUSTERED"
		}
		return "NONCLUSTERED"
	}
	switch strings.ToUpper(strings.TrimSpace(idx.IndexType)) {
	case "CLUSTERED":
		return "CLUSTERED"
	case "NONCLUSTERED":
		return "NONCLUSTERED"
	}
	return ""
}

// indexClustered 返回索引是否聚集（默认非聚集）
func indexClustered(idx *model.IndexSchema) bool {
	if idx.IsClustered != nil {
		return *idx.IsClustered
	}
	return strings.EqualFold(strings.TrimSpace(idx.IndexType), "CLUSTERED")
}

// tableCommentStatements 生成表注释的条件维护语句
func (e *Emitter) tableCommentStatements(t *model.TableSchema, comment string) []string {
	schema := t.Schema
	if strings.TrimSpace(schema) == "" {
		schema = "dbo"
	}
	objectName := e.QualifyTable(t.Schema, t.Name)
	objectLiteral := e.QuoteStringLiteral(objectName)
	valueLiteral := e.QuoteStringLiteral(comment)
	schemaLiteral := e.QuoteStringLiteral(schema)
	tableLiteral := e.QuoteStringLiteral(t.Name)
	return []string{fmt.Sprintf(
		"IF EXISTS (SELECT 1 FROM sys.extended_properties WHERE class = 1 AND major_id = OBJECT_ID(%s) AND minor_id = 0 AND name = N'MS_Description') "+
			"EXEC sp_updateextendedproperty N'MS_Description', %s, N'SCHEMA', %s, N'TABLE', %s "+
			"ELSE EXEC sp_addextendedproperty N'MS_Description', %s, N'SCHEMA', %s, N'TABLE', %s;",
		objectLiteral, valueLiteral, schemaLiteral, tableLiteral,
		valueLiteral, schemaLiteral, tableLiteral)}
}

// columnCommentStatements 生成列注释的条件维护语句
func (e *Emitter) columnCommentStatements(t *model.TableSchema, column, comment string) []string {
	schema := t.Schema
	if strings.TrimSpace(schema) == "" {
		schema = "dbo"
	}
	objectName := e.QualifyTable(t.Schema, t.Name)
	objectLiteral := e.QuoteStringLiteral(objectName)
	valueLiteral := e.QuoteStringLiteral(comment)
	schemaLiteral := e.QuoteStringLiteral(schema)
	tableLiteral := e.QuoteStringLiteral(t.Name)
	columnLiteral := e.QuoteStringLiteral(column)
	return []string{fmt.Sprintf(
		"IF EXISTS (SELECT 1 FROM sys.extended_properties WHERE class = 1 AND major_id = OBJECT_ID(%s) AND minor_id = COLUMNPROPERTY(OBJECT_ID(%s), %s, 'ColumnId') AND name = N'MS_Description') "+
			"EXEC sp_updateextendedproperty N'MS_Description', %s, N'SCHEMA', %s, N'TABLE', %s, N'COLUMN', %s "+
			"ELSE EXEC sp_addextendedproperty N'MS_Description', %s, N'SCHEMA', %s, N'TABLE', %s, N'COLUMN', %s;",
		objectLiteral, objectLiteral, columnLiteral,
		valueLiteral, schemaLiteral, tableLiteral, columnLiteral,
		valueLiteral, schemaLiteral, tableLiteral, columnLiteral)}
}

// hasIdentityColumn 判断表中是否存在 IDENTITY 列
func hasIdentityColumn(t *model.TableSchema) bool {
	for _, col := range t.Columns {
		if col != nil && col.IsIdentity {
			return true
		}
	}
	return false
}

// nullableValue 将可空性归一化为 YES/NO
func nullableValue(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "NO") {
		return "NO"
	}
	return "YES"
}

// columnDefault 安全读取列默认值
func columnDefault(c *model.ColumnSchema) *string {
	if c == nil {
		return nil
	}
	return c.ColumnDefault
}

// defaultsEqual 比较两个默认值，忽略最外层括号差异
func defaultsEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return normalizeDefault(*a) == normalizeDefault(*b)
}

// normalizeDefault 去除默认值表达式外层的多余括号
func normalizeDefault(s string) string {
	s = strings.TrimSpace(s)
	for len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' {
		depth := 0
		enclosed := true
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i != len(s)-1 {
					enclosed = false
				}
			}
			if !enclosed {
				break
			}
		}
		if !enclosed || depth != 0 {
			break
		}
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

// equalStrings 判断两个字符串切片是否完全一致
func equalStrings(a, b []string) bool {
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
