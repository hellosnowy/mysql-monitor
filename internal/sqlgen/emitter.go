package sqlgen

import "sql-monitor/internal/model"

// Emitter 定义某一种数据库方言的 SQL 语句生成策略。
// 与方言无关的差异算法只负责判断“改了什么”，由 Emitter 决定“SQL 怎么写”。
type Emitter interface {
	Vendor() model.Vendor

	// 词法处理
	QuoteIdentifier(name string) string
	QualifyTable(schema, table string) string
	QuoteStringLiteral(s string) string
	FormatValue(v interface{}) string
	FormatColumnSpec(col *model.ColumnSchema) string

	// DDL
	CreateTable(t *model.TableSchema) []string
	DropTable(t *model.TableSchema) []string
	AddColumn(t *model.TableSchema, col *model.ColumnSchema) []string
	DropColumn(t *model.TableSchema, col *model.ColumnSchema) []string
	ModifyColumn(t *model.TableSchema, from, to *model.ColumnSchema) []string
	AddPrimaryKey(t *model.TableSchema) []string
	DropPrimaryKey(t *model.TableSchema) []string
	AddIndex(t *model.TableSchema, idx *model.IndexSchema) []string
	DropIndex(t *model.TableSchema, idx *model.IndexSchema) []string
	SetTableComment(t *model.TableSchema, comment string) []string
	AlterTableOptions(from, to *model.TableSchema) []string

	// 方言相关的等价判断（默认值/自增/类型渲染归一化）
	ColumnEquivalent(a, b *model.ColumnSchema) bool
	IndexEquivalent(a, b *model.IndexSchema) bool

	// DML
	InsertRow(t *model.TableSchema, columns []string, row map[string]interface{}) string
	UpdateRow(t *model.TableSchema, pkCols []string, changed map[string]interface{}, fullRow map[string]interface{}, allCols []string) string
	DeleteRow(t *model.TableSchema, pkCols []string, allCols []string, row map[string]interface{}) string

	// 单表 DML 包装（SQL Server 的 IDENTITY_INSERT）
	TableDMLBegin(t *model.TableSchema, hasInserts bool) []string
	TableDMLEnd(t *model.TableSchema, hasInserts bool) []string

	// 脚本包装
	SessionBegin() []string
	SessionEnd() []string
	DDLBegin() []string
	DDLEnd() []string
	DMLBegin() []string
	DMLEnd() []string
}
