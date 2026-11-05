package postgres

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"sql-monitor/internal/model"
)

// Emitter PostgreSQL SQL 发射器
type Emitter struct{}

// Vendor 返回方言标识
func (e *Emitter) Vendor() model.Vendor { return model.VendorPostgres }

// QuoteIdentifier 使用双引号包裹标识符
func (e *Emitter) QuoteIdentifier(name string) string { return quoteIdentifier(name) }

// QualifyTable PostgreSQL 使用 "schema"."table" 定位表
func (e *Emitter) QualifyTable(schema, table string) string { return quoteQualified(schema, table) }

// QuoteStringLiteral 生成 PostgreSQL 字符串字面量
func (e *Emitter) QuoteStringLiteral(s string) string { return quoteLiteral(s) }

// FormatValue 生成 PostgreSQL 值字面量
func (e *Emitter) FormatValue(v interface{}) string { return formatValue(v) }

// FormatColumnSpec 生成 PostgreSQL 列定义，不含注释（注释由独立 COMMIT 语句处理）
func (e *Emitter) FormatColumnSpec(col *model.ColumnSchema) string {
	if col == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(quoteIdentifier(col.Name))
	sb.WriteString(" ")

	colType := typeName(col)
	sb.WriteString(colType)

	if col.CollationName != "" && !strings.EqualFold(col.CollationName, "default") {
		sb.WriteString(" COLLATE ")
		sb.WriteString(quoteIdentifier(col.CollationName))
	}

	switch {
	case col.IsIdentity:
		sb.WriteString(" GENERATED ")
		sb.WriteString(identityKeyword(col.IdentityGeneration))
		sb.WriteString(" AS IDENTITY")
	case col.IsGenerated:
		// PostgreSQL 当前仅支持 STORED 生成列
		sb.WriteString(" GENERATED ALWAYS AS (")
		sb.WriteString(strings.TrimSpace(col.GenerationExpression))
		sb.WriteString(") STORED")
	case col.ColumnDefault != nil && strings.TrimSpace(*col.ColumnDefault) != "":
		sb.WriteString(" DEFAULT ")
		sb.WriteString(strings.TrimSpace(*col.ColumnDefault))
	}

	if isNotNull(col) {
		sb.WriteString(" NOT NULL")
	}

	return sb.String()
}

// CreateTable 依据元数据重建 PostgreSQL 建表语句
func (e *Emitter) CreateTable(t *model.TableSchema) []string {
	if t == nil {
		return nil
	}
	qualified := e.QualifyTable(t.Schema, t.Name)

	var sb strings.Builder
	sb.WriteString("CREATE TABLE ")
	sb.WriteString(qualified)
	sb.WriteString(" (\n")

	defs := make([]string, 0, len(t.Columns)+1)
	for _, col := range t.Columns {
		if col == nil {
			continue
		}
		defs = append(defs, "  "+e.FormatColumnSpec(col))
	}
	if t.PrimaryKey != nil && len(t.PrimaryKey.Columns) > 0 {
		var pk strings.Builder
		pk.WriteString("  ")
		if t.PrimaryKey.Name != "" {
			pk.WriteString("CONSTRAINT ")
			pk.WriteString(quoteIdentifier(t.PrimaryKey.Name))
			pk.WriteString(" ")
		}
		pk.WriteString("PRIMARY KEY (")
		pk.WriteString(joinQuoted(t.PrimaryKey.Columns))
		pk.WriteString(")")
		defs = append(defs, pk.String())
	}
	sb.WriteString(strings.Join(defs, ",\n"))
	sb.WriteString("\n);")

	statements := []string{sb.String()}

	// 索引单独建，避免在 CREATE TABLE 内混合约束与索引
	for _, idx := range t.Indexes {
		if idx == nil {
			continue
		}
		statements = append(statements, e.createIndexSQL(t, idx))
	}

	if strings.TrimSpace(t.Comment) != "" {
		statements = append(statements, fmt.Sprintf("COMMENT ON TABLE %s IS %s;",
			qualified, quoteLiteral(t.Comment)))
	}
	for _, col := range t.Columns {
		if col == nil || strings.TrimSpace(col.Comment) == "" {
			continue
		}
		statements = append(statements, e.columnCommentSQL(t, col.Name, col.Comment))
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

// AddColumn 新增字段，PostgreSQL 不支持指定列位置
func (e *Emitter) AddColumn(t *model.TableSchema, col *model.ColumnSchema) []string {
	if t == nil || col == nil {
		return nil
	}
	statements := []string{fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;",
		e.QualifyTable(t.Schema, t.Name), e.FormatColumnSpec(col))}
	if strings.TrimSpace(col.Comment) != "" {
		statements = append(statements, e.columnCommentSQL(t, col.Name, col.Comment))
	}
	return statements
}

// DropColumn 删除字段
func (e *Emitter) DropColumn(t *model.TableSchema, col *model.ColumnSchema) []string {
	if t == nil || col == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;",
		e.QualifyTable(t.Schema, t.Name), quoteIdentifier(col.Name))}
}

// ModifyColumn 将字段变更拆分为多条 ALTER COLUMN 语句
func (e *Emitter) ModifyColumn(t *model.TableSchema, from, to *model.ColumnSchema) []string {
	if t == nil || to == nil {
		return nil
	}

	qualified := e.QualifyTable(t.Schema, t.Name)
	col := quoteIdentifier(to.Name)
	prefix := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s ", qualified, col)
	var statements []string

	// 1. 类型变更
	if from != nil && !typeEquivalent(from, to) {
		alterType := alterTypeName(typeName(to))
		statements = append(statements, fmt.Sprintf("%sTYPE %s USING %s::%s;",
			prefix, alterType, col, alterType))
	}

	// 2. NULL 约束变更
	fromNull := from != nil && !isNotNull(from)
	toNull := !isNotNull(to)
	if from != nil && fromNull != toNull {
		if toNull {
			statements = append(statements, prefix+"DROP NOT NULL;")
		} else {
			statements = append(statements, prefix+"SET NOT NULL;")
		}
	}

	// 3. 默认值变更
	fromDefault := (*string)(nil)
	if from != nil {
		fromDefault = from.ColumnDefault
	}
	if !defaultEquivalent(fromDefault, to.ColumnDefault) {
		if to.ColumnDefault == nil || strings.TrimSpace(*to.ColumnDefault) == "" {
			statements = append(statements, prefix+"DROP DEFAULT;")
		} else {
			statements = append(statements, fmt.Sprintf("%sSET DEFAULT %s;",
				prefix, strings.TrimSpace(*to.ColumnDefault)))
		}
	}

	// 4. identity 变更
	fromIdentity := from != nil && from.IsIdentity
	toIdentity := to.IsIdentity
	switch {
	case !fromIdentity && toIdentity:
		statements = append(statements, fmt.Sprintf("%sADD GENERATED %s AS IDENTITY;",
			prefix, identityKeyword(to.IdentityGeneration)))
	case fromIdentity && !toIdentity:
		statements = append(statements, prefix+"DROP IDENTITY IF EXISTS;")
	case fromIdentity && toIdentity:
		if identityKeyword(from.IdentityGeneration) != identityKeyword(to.IdentityGeneration) {
			statements = append(statements, fmt.Sprintf("%sSET GENERATED %s;",
				prefix, identityKeyword(to.IdentityGeneration)))
		}
	}

	// 5. 生成列变更（SET EXPRESSION 需要 PostgreSQL 17+）
	fromGenerated := from != nil && from.IsGenerated
	switch {
	case fromGenerated && !to.IsGenerated:
		statements = append(statements, prefix+"DROP EXPRESSION;")
	case !fromGenerated && to.IsGenerated:
		statements = append(statements, fmt.Sprintf("%sSET EXPRESSION AS (%s);",
			prefix, strings.TrimSpace(to.GenerationExpression)))
	case fromGenerated && to.IsGenerated &&
		strings.TrimSpace(from.GenerationExpression) != strings.TrimSpace(to.GenerationExpression):
		statements = append(statements, fmt.Sprintf("%sSET EXPRESSION AS (%s);",
			prefix, strings.TrimSpace(to.GenerationExpression)))
	}

	// 6. 字段注释变更
	if from == nil || from.Comment != to.Comment {
		statements = append(statements, e.columnCommentSQL(t, to.Name, to.Comment))
	}

	return statements
}

// AddPrimaryKey 新增主键，优先使用约束名
func (e *Emitter) AddPrimaryKey(t *model.TableSchema) []string {
	if t == nil || t.PrimaryKey == nil || len(t.PrimaryKey.Columns) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("ALTER TABLE ")
	sb.WriteString(e.QualifyTable(t.Schema, t.Name))
	sb.WriteString(" ADD ")
	if t.PrimaryKey.Name != "" {
		sb.WriteString("CONSTRAINT ")
		sb.WriteString(quoteIdentifier(t.PrimaryKey.Name))
		sb.WriteString(" ")
	}
	sb.WriteString("PRIMARY KEY (")
	sb.WriteString(joinQuoted(t.PrimaryKey.Columns))
	sb.WriteString(");")
	return []string{sb.String()}
}

// DropPrimaryKey 按约束名删除主键
func (e *Emitter) DropPrimaryKey(t *model.TableSchema) []string {
	if t == nil {
		return nil
	}
	name := t.Name + "_pkey"
	if t.PrimaryKey != nil && t.PrimaryKey.Name != "" {
		name = t.PrimaryKey.Name
	}
	return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;",
		e.QualifyTable(t.Schema, t.Name), quoteIdentifier(name))}
}

// AddIndex 新增索引；约束型唯一索引使用 ADD CONSTRAINT
func (e *Emitter) AddIndex(t *model.TableSchema, idx *model.IndexSchema) []string {
	if t == nil || idx == nil {
		return nil
	}
	if idx.IsConstraint && !idx.NonUnique && strings.TrimSpace(idx.Filter) == "" {
		var sb strings.Builder
		sb.WriteString("ALTER TABLE ")
		sb.WriteString(e.QualifyTable(t.Schema, t.Name))
		sb.WriteString(" ADD CONSTRAINT ")
		sb.WriteString(quoteIdentifier(idx.Name))
		sb.WriteString(" UNIQUE (")
		sb.WriteString(joinIndexColumns(idx.Columns))
		sb.WriteString(")")
		if len(idx.IncludeColumns) > 0 {
			sb.WriteString(" INCLUDE (")
			sb.WriteString(joinIncludeColumns(idx.IncludeColumns))
			sb.WriteString(")")
		}
		sb.WriteString(";")
		return []string{sb.String()}
	}
	return []string{e.createIndexSQL(t, idx)}
}

// DropIndex 删除索引；约束型索引按约束删除
func (e *Emitter) DropIndex(t *model.TableSchema, idx *model.IndexSchema) []string {
	if t == nil || idx == nil {
		return nil
	}
	if idx.IsConstraint {
		return []string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;",
			e.QualifyTable(t.Schema, t.Name), quoteIdentifier(idx.Name))}
	}
	return []string{fmt.Sprintf("DROP INDEX %s;", e.QualifyTable(t.Schema, idx.Name))}
}

// SetTableComment 更新表注释，空注释写入 NULL
func (e *Emitter) SetTableComment(t *model.TableSchema, comment string) []string {
	if t == nil {
		return nil
	}
	qualified := e.QualifyTable(t.Schema, t.Name)
	if strings.TrimSpace(comment) == "" {
		return []string{fmt.Sprintf("COMMENT ON TABLE %s IS NULL;", qualified)}
	}
	return []string{fmt.Sprintf("COMMENT ON TABLE %s IS %s;", qualified, quoteLiteral(comment))}
}

// AlterTableOptions PostgreSQL 没有存储引擎等表级属性
func (e *Emitter) AlterTableOptions(from, to *model.TableSchema) []string { return nil }

// ColumnEquivalent 判断字段是否有实质性变更
func (e *Emitter) ColumnEquivalent(a, b *model.ColumnSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if !strings.EqualFold(typeName(a), typeName(b)) ||
		!strings.EqualFold(a.IsNullable, b.IsNullable) ||
		a.IsIdentity != b.IsIdentity ||
		identityKeyword(a.IdentityGeneration) != identityKeyword(b.IdentityGeneration) ||
		a.IsGenerated != b.IsGenerated ||
		strings.TrimSpace(a.GenerationExpression) != strings.TrimSpace(b.GenerationExpression) ||
		a.Comment != b.Comment {
		return false
	}
	return defaultEquivalent(a.ColumnDefault, b.ColumnDefault)
}

// IndexEquivalent 判断索引定义是否等价
func (e *Emitter) IndexEquivalent(a, b *model.IndexSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.NonUnique != b.NonUnique ||
		!strings.EqualFold(indexMethod(a), indexMethod(b)) ||
		len(a.Columns) != len(b.Columns) ||
		len(a.IncludeColumns) != len(b.IncludeColumns) {
		return false
	}
	for i := range a.Columns {
		if a.Columns[i].Name != b.Columns[i].Name {
			return false
		}
	}
	for i := range a.IncludeColumns {
		if a.IncludeColumns[i] != b.IncludeColumns[i] {
			return false
		}
	}
	return strings.TrimSpace(a.Filter) == strings.TrimSpace(b.Filter)
}

// InsertRow 生成单行 INSERT 语句，identity 列存在时覆盖系统生成值
func (e *Emitter) InsertRow(t *model.TableSchema, columns []string, row map[string]interface{}) string {
	var colNames, valStrs []string
	for _, col := range columns {
		val, exists := row[col]
		if !exists {
			continue
		}
		colNames = append(colNames, quoteIdentifier(col))
		valStrs = append(valStrs, formatValue(val))
	}

	table := ""
	if t != nil {
		table = e.QualifyTable(t.Schema, t.Name)
	}
	overriding := ""
	if hasIdentityColumn(t) {
		overriding = " OVERRIDING SYSTEM VALUE"
	}

	return fmt.Sprintf("INSERT INTO %s (%s)%s VALUES (%s);",
		table, strings.Join(colNames, ", "), overriding, strings.Join(valStrs, ", "))
}

// UpdateRow 生成单行 UPDATE 语句；无主键时按整行匹配
func (e *Emitter) UpdateRow(t *model.TableSchema, pkCols []string, changed map[string]interface{}, fullRow map[string]interface{}, allCols []string) string {
	cols := make([]string, 0, len(changed))
	for col := range changed {
		cols = append(cols, col)
	}
	sort.Strings(cols)

	setClauses := make([]string, 0, len(cols))
	for _, col := range cols {
		setClauses = append(setClauses, fmt.Sprintf("%s = %s",
			quoteIdentifier(col), formatValue(changed[col])))
	}

	var where []string
	if len(pkCols) > 0 {
		for _, pk := range pkCols {
			where = append(where, equality(quoteIdentifier(pk), fullRow[pk]))
		}
	} else {
		// 无主键表退化为所有列匹配，按快照列顺序保证确定性
		for _, col := range allCols {
			if _, isChanged := changed[col]; isChanged {
				continue
			}
			where = append(where, equality(quoteIdentifier(col), fullRow[col]))
		}
	}

	table := ""
	if t != nil {
		table = e.QualifyTable(t.Schema, t.Name)
	}
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s;",
		table, strings.Join(setClauses, ", "), strings.Join(where, " AND "))
}

// DeleteRow 生成单行 DELETE 语句；无主键表通过 ctid 精确定位一行
func (e *Emitter) DeleteRow(t *model.TableSchema, pkCols []string, allCols []string, row map[string]interface{}) string {
	table := ""
	if t != nil {
		table = e.QualifyTable(t.Schema, t.Name)
	}

	if len(pkCols) == 0 {
		// 无主键表可能有重复行，借助 ctid 只删除其中一行
		var conditions []string
		for _, col := range allCols {
			conditions = append(conditions, equality(quoteIdentifier(col), row[col]))
		}
		subQuery := "SELECT ctid FROM " + table
		if len(conditions) > 0 {
			subQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		subQuery += " LIMIT 1"
		return fmt.Sprintf("DELETE FROM %s WHERE ctid = (%s);", table, subQuery)
	}

	var where []string
	for _, pk := range pkCols {
		where = append(where, equality(quoteIdentifier(pk), row[pk]))
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s;", table, strings.Join(where, " AND "))
}

// TableDMLBegin PostgreSQL identity 通过 OVERRIDING SYSTEM VALUE 处理，无需单表包装
func (e *Emitter) TableDMLBegin(*model.TableSchema, bool) []string { return nil }

// TableDMLEnd PostgreSQL 无需单表包装
func (e *Emitter) TableDMLEnd(*model.TableSchema, bool) []string { return nil }

// SessionBegin 开启整个脚本的统一事务
func (e *Emitter) SessionBegin() []string { return []string{"BEGIN;"} }

// SessionEnd 提交整个脚本的事务
func (e *Emitter) SessionEnd() []string { return []string{"COMMIT;"} }

// DDLBegin PostgreSQL DDL 可参与事务，整脚本已由 SessionBegin 包裹
func (e *Emitter) DDLBegin() []string { return nil }

// DDLEnd PostgreSQL DDL 可参与事务
func (e *Emitter) DDLEnd() []string { return nil }

// DMLBegin 整脚本共用一个事务
func (e *Emitter) DMLBegin() []string { return nil }

// DMLEnd 整脚本共用一个事务
func (e *Emitter) DMLEnd() []string { return nil }

// createIndexSQL 生成普通 CREATE INDEX 语句
func (e *Emitter) createIndexSQL(t *model.TableSchema, idx *model.IndexSchema) string {
	var sb strings.Builder
	sb.WriteString("CREATE ")
	if !idx.NonUnique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX ")
	sb.WriteString(quoteIdentifier(idx.Name))
	sb.WriteString(" ON ")
	sb.WriteString(e.QualifyTable(t.Schema, t.Name))
	sb.WriteString(" USING ")
	sb.WriteString(quoteIdentifier(indexMethod(idx)))
	sb.WriteString(" (")
	sb.WriteString(joinIndexColumns(idx.Columns))
	sb.WriteString(")")
	if len(idx.IncludeColumns) > 0 {
		sb.WriteString(" INCLUDE (")
		sb.WriteString(joinIncludeColumns(idx.IncludeColumns))
		sb.WriteString(")")
	}
	if strings.TrimSpace(idx.Filter) != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.TrimSpace(idx.Filter))
	}
	sb.WriteString(";")
	return sb.String()
}

// columnCommentSQL 生成字段注释语句，空注释写入 NULL
func (e *Emitter) columnCommentSQL(t *model.TableSchema, column, comment string) string {
	target := e.QualifyTable(t.Schema, t.Name) + "." + quoteIdentifier(column)
	if strings.TrimSpace(comment) == "" {
		return fmt.Sprintf("COMMENT ON COLUMN %s IS NULL;", target)
	}
	return fmt.Sprintf("COMMENT ON COLUMN %s IS %s;", target, quoteLiteral(comment))
}

// quoteIdentifier 使用双引号包裹标识符并转义内部双引号
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteQualified 生成 "schema"."table"；schema 为空时只引用表名
func quoteQualified(schema, table string) string {
	if schema == "" {
		return quoteIdentifier(table)
	}
	return quoteIdentifier(schema) + "." + quoteIdentifier(table)
}

// quoteLiteral 使用单引号包裹字符串并双写内部单引号
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// formatValue 将任意 Go 类型值转换为 PostgreSQL 合法的 SQL 字面量
func formatValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}

	switch v := val.(type) {
	case json.Number:
		return v.String()
	case string:
		return quoteLiteral(v)
	case []byte:
		// 二进制数据使用 bytea 十六进制格式
		return fmt.Sprintf(`'\x%s'::bytea`, hex.EncodeToString(v))
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case time.Time:
		// PostgreSQL 可直接解析带时区偏移的时间戳
		return quoteLiteral(v.Format("2006-01-02 15:04:05.999999-07:00"))
	default:
		return quoteLiteral(fmt.Sprintf("%v", v))
	}
}

// equality 生成等值条件，NULL 使用 IS NULL
func equality(quotedCol string, val interface{}) string {
	if val == nil {
		return quotedCol + " IS NULL"
	}
	return fmt.Sprintf("%s = %s", quotedCol, formatValue(val))
}

// typeName 返回列的类型定义，优先 ColumnType
func typeName(col *model.ColumnSchema) string {
	if col == nil {
		return ""
	}
	if t := strings.TrimSpace(col.ColumnType); t != "" {
		return t
	}
	return strings.TrimSpace(col.DataType)
}

// alterTypeName 将 serial 伪类型转换为 ALTER COLUMN TYPE 可接受的真实类型
func alterTypeName(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "smallserial":
		return "smallint"
	case "serial":
		return "integer"
	case "bigserial":
		return "bigint"
	default:
		return t
	}
}

// typeEquivalent 比较类型定义，serial 与对应整数类型视为可原地转换
func typeEquivalent(a, b *model.ColumnSchema) bool {
	return strings.EqualFold(
		alterTypeName(typeName(a)),
		alterTypeName(typeName(b)))
}

// isNotNull 判断列是否为 NOT NULL
func isNotNull(col *model.ColumnSchema) bool {
	return col != nil && strings.EqualFold(col.IsNullable, "NO")
}

// identityKeyword 归一化 identity 生成方式
func identityKeyword(generation string) string {
	if strings.EqualFold(strings.TrimSpace(generation), "ALWAYS") {
		return "ALWAYS"
	}
	return "BY DEFAULT"
}

// indexMethod 返回索引方法，缺省为 btree
func indexMethod(idx *model.IndexSchema) string {
	if idx == nil {
		return "btree"
	}
	method := strings.TrimSpace(idx.Method)
	if method == "" {
		method = strings.TrimSpace(idx.IndexType)
	}
	if method == "" {
		return "btree"
	}
	return method
}

// hasIdentityColumn 判断表中是否存在 identity 列
func hasIdentityColumn(t *model.TableSchema) bool {
	if t == nil {
		return false
	}
	for _, col := range t.Columns {
		if col != nil && col.IsIdentity {
			return true
		}
	}
	return false
}

// joinQuoted 将列名列表引用后拼接
func joinQuoted(cols []string) string {
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		parts = append(parts, quoteIdentifier(col))
	}
	return strings.Join(parts, ", ")
}

// joinIndexColumns 拼接索引键列，表达式列按原样输出
func joinIndexColumns(cols []model.IndexColumn) string {
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		parts = append(parts, indexColumnExpression(col.Name))
	}
	return strings.Join(parts, ", ")
}

// joinIncludeColumns 拼接 INCLUDE 覆盖列
func joinIncludeColumns(cols []string) string {
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		parts = append(parts, indexColumnExpression(col))
	}
	return strings.Join(parts, ", ")
}

// indexColumnExpression 简单标识符加引号，表达式索引保持原样
func indexColumnExpression(name string) string {
	if isSimpleIdentifier(name) {
		return quoteIdentifier(name)
	}
	return name
}

// isSimpleIdentifier 判断是否为可直接加引号的普通标识符
func isSimpleIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9', r == '$':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// defaultEquivalent 比较两个默认值，忽略 PostgreSQL 类型转换等渲染差异
func defaultEquivalent(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	left, right := "", ""
	if a != nil {
		left = *a
	}
	if b != nil {
		right = *b
	}
	return normalizeDefault(left) == normalizeDefault(right)
}

// normalizeDefault 归一化默认值表达式，便于比较 'x'::text 与 'x' 等等价写法
func normalizeDefault(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}

	for {
		next := strings.TrimSpace(stripOuterParens(v))
		next = strings.TrimSpace(stripTrailingCast(next))
		if next == v {
			break
		}
		v = next
	}
	if v == "" {
		return ""
	}

	// 数字字面量：'0' 与 0 视为相同
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		inner := strings.ReplaceAll(v[1:len(v)-1], "''", "'")
		if _, err := strconv.ParseFloat(strings.TrimSpace(inner), 64); err == nil {
			return strings.TrimSpace(inner)
		}
		return v
	}

	// 不含字符串字面量时忽略大小写与空白，统一 now() 等时间函数
	if !strings.Contains(v, "'") {
		lower := strings.ToLower(v)
		switch lower {
		case "current_timestamp", "transaction_timestamp()", "now()":
			return "now()"
		}
		return strings.Join(strings.Fields(lower), "")
	}
	return v
}

// stripOuterParens 去掉完整包裹表达式的最外层括号
func stripOuterParens(s string) string {
	for len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' {
		depth := 0
		inQuote := false
		wraps := true
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c == '\'' {
				// 处理 '' 转义
				if inQuote && i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				inQuote = !inQuote
				continue
			}
			if inQuote {
				continue
			}
			switch c {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i != len(s)-1 {
					wraps = false
				}
			}
		}
		if !wraps || depth != 0 {
			break
		}
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

// stripTrailingCast 去掉作用于整个表达式的末尾 ::type 类型转换
func stripTrailingCast(s string) string {
	depth := 0
	inQuote := false
	lastCast := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			if inQuote && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch c {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ':':
			if depth == 0 && i+1 < len(s) && s[i+1] == ':' {
				lastCast = i
				i++
			}
		}
	}
	if lastCast <= 0 {
		return s
	}
	if looksLikeTypeName(s[lastCast+2:]) {
		return strings.TrimSpace(s[:lastCast])
	}
	return s
}

// looksLikeTypeName 粗略判断字符串是否只包含类型名允许的字符
func looksLikeTypeName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	first := s[0]
	if !(first >= 'a' && first <= 'z' || first >= 'A' && first <= 'Z' || first == '_' || first == '"') {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == ' ', r == '.', r == '"', r == '[', r == ']', r == '(', r == ')', r == ',':
		default:
			return false
		}
	}
	return true
}
