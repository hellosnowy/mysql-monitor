package mysql

import (
	"fmt"
	"sort"
	"strings"

	"sql-monitor/internal/model"
	"sql-monitor/pkg/sqlutil"
)

// Emitter MySQL SQL 发射器
type Emitter struct{}

// Vendor 返回方言标识
func (e *Emitter) Vendor() model.Vendor { return model.VendorMySQL }

// QuoteIdentifier 使用反引号包裹标识符
func (e *Emitter) QuoteIdentifier(name string) string { return sqlutil.EscapeIdentifier(name) }

// QualifyTable MySQL 通过连接的目标库定位表，语句中不追加库名前缀
func (e *Emitter) QualifyTable(schema, table string) string { return sqlutil.EscapeIdentifier(table) }

// QuoteStringLiteral 生成 MySQL 字符串字面量
func (e *Emitter) QuoteStringLiteral(s string) string { return sqlutil.EscapeStringLiteral(s) }

// FormatValue 生成 MySQL 值字面量
func (e *Emitter) FormatValue(v interface{}) string { return sqlutil.FormatSQLValue(v) }

// FormatColumnSpec 生成 MySQL 列定义
func (e *Emitter) FormatColumnSpec(col *model.ColumnSchema) string {
	return sqlutil.FormatColumnSpec(col)
}

// CreateTable 优先复用 SHOW CREATE TABLE 的原生语句
func (e *Emitter) CreateTable(t *model.TableSchema) []string {
	if t == nil {
		return nil
	}
	if sqlText := strings.TrimSpace(t.CreateTableSQL); sqlText != "" {
		if !strings.HasSuffix(sqlText, ";") {
			sqlText += ";"
		}
		return []string{sqlText}
	}
	return []string{e.buildCreateTableSQL(t)}
}

// DropTable 删除表
func (e *Emitter) DropTable(t *model.TableSchema) []string {
	if t == nil {
		return nil
	}
	return []string{fmt.Sprintf("DROP TABLE IF EXISTS %s;", e.QuoteIdentifier(t.Name))}
}

// AddColumn 新增字段，尽量保持目标列位置
func (e *Emitter) AddColumn(t *model.TableSchema, col *model.ColumnSchema) []string {
	if t == nil || col == nil {
		return nil
	}
	spec := e.FormatColumnSpec(col)
	position := ""
	for i, c := range t.Columns {
		if c.Name != col.Name {
			continue
		}
		if i == 0 {
			position = " FIRST"
		} else {
			position = fmt.Sprintf(" AFTER %s", e.QuoteIdentifier(t.Columns[i-1].Name))
		}
		break
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s%s;", e.QuoteIdentifier(t.Name), spec, position)}
}

// DropColumn 删除字段
func (e *Emitter) DropColumn(t *model.TableSchema, col *model.ColumnSchema) []string {
	if t == nil || col == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", e.QuoteIdentifier(t.Name), e.QuoteIdentifier(col.Name))}
}

// ModifyColumn 修改字段定义
func (e *Emitter) ModifyColumn(t *model.TableSchema, from, to *model.ColumnSchema) []string {
	if t == nil || to == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s;", e.QuoteIdentifier(t.Name), e.FormatColumnSpec(to))}
}

// AddPrimaryKey 新增主键
func (e *Emitter) AddPrimaryKey(t *model.TableSchema) []string {
	if t == nil || t.PrimaryKey == nil || len(t.PrimaryKey.Columns) == 0 {
		return nil
	}
	cols := make([]string, 0, len(t.PrimaryKey.Columns))
	for _, col := range t.PrimaryKey.Columns {
		cols = append(cols, e.QuoteIdentifier(col))
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", e.QuoteIdentifier(t.Name), strings.Join(cols, ", "))}
}

// DropPrimaryKey 删除主键
func (e *Emitter) DropPrimaryKey(t *model.TableSchema) []string {
	if t == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY;", e.QuoteIdentifier(t.Name))}
}

// AddIndex 新增索引
func (e *Emitter) AddIndex(t *model.TableSchema, idx *model.IndexSchema) []string {
	if t == nil || idx == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s ADD %s;", e.QuoteIdentifier(t.Name), e.formatIndexSpec(idx))}
}

// DropIndex 删除索引
func (e *Emitter) DropIndex(t *model.TableSchema, idx *model.IndexSchema) []string {
	if t == nil || idx == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s DROP INDEX %s;", e.QuoteIdentifier(t.Name), e.QuoteIdentifier(idx.Name))}
}

// SetTableComment 更新表注释（空注释同样会覆盖旧值）
func (e *Emitter) SetTableComment(t *model.TableSchema, comment string) []string {
	if t == nil {
		return nil
	}
	return []string{fmt.Sprintf("ALTER TABLE %s COMMENT = %s;", e.QuoteIdentifier(t.Name), e.QuoteStringLiteral(comment))}
}

// AlterTableOptions 处理 MySQL 存储引擎变更
func (e *Emitter) AlterTableOptions(from, to *model.TableSchema) []string {
	if from == nil || to == nil {
		return nil
	}
	if to.Engine != "" && from.Engine != "" && !strings.EqualFold(from.Engine, to.Engine) {
		return []string{fmt.Sprintf("ALTER TABLE %s ENGINE = %s;", e.QuoteIdentifier(to.Name), to.Engine)}
	}
	return nil
}

// ColumnEquivalent 判断字段是否有实质性变更
func (e *Emitter) ColumnEquivalent(a, b *model.ColumnSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if !strings.EqualFold(a.ColumnType, b.ColumnType) ||
		!strings.EqualFold(a.IsNullable, b.IsNullable) ||
		!strings.EqualFold(a.Extra, b.Extra) ||
		a.Comment != b.Comment ||
		a.IsIdentity != b.IsIdentity {
		return false
	}
	return equalDefault(a.ColumnDefault, b.ColumnDefault)
}

// IndexEquivalent 判断索引定义是否等价
func (e *Emitter) IndexEquivalent(a, b *model.IndexSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.NonUnique != b.NonUnique || len(a.Columns) != len(b.Columns) {
		return false
	}
	for i := range a.Columns {
		c1, c2 := a.Columns[i], b.Columns[i]
		if c1.Name != c2.Name {
			return false
		}
		if (c1.SubPart == nil) != (c2.SubPart == nil) {
			return false
		}
		if c1.SubPart != nil && c2.SubPart != nil && *c1.SubPart != *c2.SubPart {
			return false
		}
	}
	return true
}

// InsertRow 生成单行 INSERT 语句
func (e *Emitter) InsertRow(t *model.TableSchema, columns []string, row map[string]interface{}) string {
	var colNames, valStrs []string
	for _, col := range columns {
		val, exists := row[col]
		if !exists {
			continue
		}
		colNames = append(colNames, e.QuoteIdentifier(col))
		valStrs = append(valStrs, e.FormatValue(val))
	}
	table := ""
	if t != nil {
		table = t.Name
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		e.QuoteIdentifier(table), strings.Join(colNames, ", "), strings.Join(valStrs, ", "))
}

// UpdateRow 生成单行 UPDATE 语句
func (e *Emitter) UpdateRow(t *model.TableSchema, pkCols []string, changed map[string]interface{}, fullRow map[string]interface{}, allCols []string) string {
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
		// 无主键表退化为所有列匹配，按快照列顺序保证确定性
		for _, col := range allCols {
			if _, isChanged := changed[col]; isChanged {
				continue
			}
			where = append(where, e.equality(e.QuoteIdentifier(col), fullRow[col]))
		}
	}

	table := ""
	if t != nil {
		table = t.Name
	}
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s;",
		e.QuoteIdentifier(table), strings.Join(setClauses, ", "), strings.Join(where, " AND "))
}

// DeleteRow 生成单行 DELETE 语句
func (e *Emitter) DeleteRow(t *model.TableSchema, pkCols []string, allCols []string, row map[string]interface{}) string {
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

	table := ""
	if t != nil {
		table = t.Name
	}
	statement := fmt.Sprintf("DELETE FROM %s WHERE %s", e.QuoteIdentifier(table), strings.Join(where, " AND "))
	if len(pkCols) == 0 {
		// 无主键表可能有重复行，每条差异只删除一行
		statement += " LIMIT 1"
	}
	return statement + ";"
}

func (e *Emitter) equality(quotedCol string, val interface{}) string {
	if val == nil {
		return quotedCol + " IS NULL"
	}
	return fmt.Sprintf("%s = %s", quotedCol, e.FormatValue(val))
}

// TableDMLBegin MySQL 不需要单表包装
func (e *Emitter) TableDMLBegin(*model.TableSchema, bool) []string { return nil }

// TableDMLEnd MySQL 不需要单表包装
func (e *Emitter) TableDMLEnd(*model.TableSchema, bool) []string { return nil }

// SessionBegin 会话级设置
func (e *Emitter) SessionBegin() []string {
	return []string{"SET NAMES utf8mb4;", "SET FOREIGN_KEY_CHECKS = 0;"}
}

// SessionEnd 恢复会话设置
func (e *Emitter) SessionEnd() []string { return []string{"SET FOREIGN_KEY_CHECKS = 1;"} }

// DDLBegin MySQL DDL 自动提交，不包事务
func (e *Emitter) DDLBegin() []string { return nil }

// DDLEnd MySQL DDL 自动提交
func (e *Emitter) DDLEnd() []string { return nil }

// DMLBegin 开启 DML 事务
func (e *Emitter) DMLBegin() []string { return []string{"START TRANSACTION;"} }

// DMLEnd 提交 DML 事务
func (e *Emitter) DMLEnd() []string { return []string{"COMMIT;"} }

func (e *Emitter) buildCreateTableSQL(table *model.TableSchema) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", e.QuoteIdentifier(table.Name)))

	var defs []string
	for _, col := range table.Columns {
		defs = append(defs, "  "+e.FormatColumnSpec(col))
	}
	if table.PrimaryKey != nil && len(table.PrimaryKey.Columns) > 0 {
		pkCols := make([]string, 0, len(table.PrimaryKey.Columns))
		for _, col := range table.PrimaryKey.Columns {
			pkCols = append(pkCols, e.QuoteIdentifier(col))
		}
		defs = append(defs, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}
	for _, idx := range table.Indexes {
		defs = append(defs, "  "+e.formatIndexSpec(idx))
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
		sb.WriteString(fmt.Sprintf(" COMMENT=%s", e.QuoteStringLiteral(table.Comment)))
	}
	sb.WriteString(";")
	return sb.String()
}

func (e *Emitter) formatIndexSpec(idx *model.IndexSchema) string {
	var sb strings.Builder
	if !idx.NonUnique {
		sb.WriteString("UNIQUE INDEX ")
	} else if strings.EqualFold(idx.IndexType, "FULLTEXT") {
		sb.WriteString("FULLTEXT INDEX ")
	} else {
		sb.WriteString("INDEX ")
	}
	sb.WriteString(e.QuoteIdentifier(idx.Name))
	sb.WriteString(" (")

	parts := make([]string, 0, len(idx.Columns))
	for _, col := range idx.Columns {
		colStr := e.QuoteIdentifier(col.Name)
		if col.SubPart != nil && *col.SubPart > 0 {
			colStr = fmt.Sprintf("%s(%d)", colStr, *col.SubPart)
		}
		parts = append(parts, colStr)
	}
	sb.WriteString(strings.Join(parts, ", "))
	sb.WriteString(")")
	return sb.String()
}

func equalDefault(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
