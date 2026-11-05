package sqlserver

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"sql-monitor/internal/model"
)

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func assertStatements(t *testing.T, name string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got: %#v\nwant: %#v", name, got, want)
	}
}

func TestEmitterQuotingAndValues(t *testing.T) {
	e := &Emitter{}
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"方括号转义", e.QuoteIdentifier("a]b"), "[a]]b]"},
		{"两段式表名", e.QualifyTable("dbo", "orders"), "[dbo].[orders]"},
		{"空模式默认 dbo", e.QualifyTable("", "orders"), "[dbo].[orders]"},
		{"Unicode 字符串", e.QuoteStringLiteral("O'Brien"), "N'O''Brien'"},
		{"NULL", e.FormatValue(nil), "NULL"},
		{"字符串", e.FormatValue("it's"), "N'it''s'"},
		{"二进制", e.FormatValue([]byte{0x0a, 0xff}), "0x0aff"},
		{"布尔真", e.FormatValue(true), "1"},
		{"布尔假", e.FormatValue(false), "0"},
		{"json.Number", e.FormatValue(json.Number("12.50")), "12.50"},
		{"整数", e.FormatValue(int64(42)), "42"},
		{"浮点数", e.FormatValue(3.14), "3.14"},
		{"时间", e.FormatValue(time.Date(2024, 1, 2, 3, 4, 5, 123456700, time.UTC)), "'2024-01-02T03:04:05.1234567'"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestEmitterColumnDDL(t *testing.T) {
	e := &Emitter{}
	table := &model.TableSchema{Name: "orders", Schema: "dbo"}

	tests := []struct {
		name string
		run  func() []string
		want []string
	}{
		{
			name: "新增带默认约束的字段",
			run: func() []string {
				return e.AddColumn(table, &model.ColumnSchema{
					Name:                  "status",
					ColumnType:            "int",
					IsNullable:            "NO",
					ColumnDefault:         strPtr("((0))"),
					DefaultConstraintName: "DF_orders_status",
				})
			},
			want: []string{"ALTER TABLE [dbo].[orders] ADD [status] int NOT NULL CONSTRAINT [DF_orders_status] DEFAULT ((0));"},
		},
		{
			name: "删除带默认约束的字段",
			run: func() []string {
				return e.DropColumn(table, &model.ColumnSchema{
					Name:                  "status",
					DefaultConstraintName: "DF_orders_status",
				})
			},
			want: []string{
				"ALTER TABLE [dbo].[orders] DROP CONSTRAINT [DF_orders_status];",
				"ALTER TABLE [dbo].[orders] DROP COLUMN [status];",
			},
		},
		{
			name: "修改字段类型与可空性",
			run: func() []string {
				return e.ModifyColumn(table,
					&model.ColumnSchema{Name: "name", ColumnType: "nvarchar(50)", IsNullable: "YES"},
					&model.ColumnSchema{Name: "name", ColumnType: "nvarchar(100)", IsNullable: "NO"})
			},
			want: []string{"ALTER TABLE [dbo].[orders] ALTER COLUMN [name] nvarchar(100) NOT NULL;"},
		},
		{
			name: "修改默认约束",
			run: func() []string {
				return e.ModifyColumn(table,
					&model.ColumnSchema{
						Name: "status", ColumnType: "int", IsNullable: "NO",
						ColumnDefault: strPtr("((0))"), DefaultConstraintName: "DF_orders_status_old",
					},
					&model.ColumnSchema{
						Name: "status", ColumnType: "int", IsNullable: "NO",
						ColumnDefault: strPtr("((1))"), DefaultConstraintName: "DF_orders_status_new",
					})
			},
			want: []string{
				"ALTER TABLE [dbo].[orders] DROP CONSTRAINT [DF_orders_status_old];",
				"ALTER TABLE [dbo].[orders] ADD CONSTRAINT [DF_orders_status_new] DEFAULT ((1)) FOR [status];",
			},
		},
		{
			name: "默认值仅外层括号不同视为等价",
			run: func() []string {
				return e.ModifyColumn(table,
					&model.ColumnSchema{Name: "status", ColumnType: "int", IsNullable: "NO", ColumnDefault: strPtr("((0))")},
					&model.ColumnSchema{Name: "status", ColumnType: "int", IsNullable: "NO", ColumnDefault: strPtr("(0)")})
			},
			want: nil,
		},
		{
			name: "IDENTITY 变更输出警告",
			run: func() []string {
				return e.ModifyColumn(table,
					&model.ColumnSchema{Name: "id", ColumnType: "int", IsNullable: "NO"},
					&model.ColumnSchema{Name: "id", ColumnType: "int", IsNullable: "NO", IsIdentity: true})
			},
			want: []string{"-- 警告: SQL Server 不支持通过 ALTER COLUMN 修改列 [id] 的 IDENTITY 属性，请手动重建该列;"},
		},
		{
			name: "计算列变更输出警告",
			run: func() []string {
				return e.ModifyColumn(table,
					&model.ColumnSchema{Name: "total", ColumnType: "int", IsNullable: "YES"},
					&model.ColumnSchema{Name: "total", ColumnType: "int", IsNullable: "YES", IsGenerated: true, GenerationExpression: "([qty]*[price])"})
			},
			want: []string{"-- 警告: SQL Server 不支持通过 ALTER COLUMN 修改列 [total] 的计算列表达式，请手动重建该列;"},
		},
	}

	for _, tt := range tests {
		assertStatements(t, tt.name, tt.run(), tt.want)
	}
}

func TestEmitterPrimaryKeyAndIndexDDL(t *testing.T) {
	e := &Emitter{}
	table := &model.TableSchema{
		Name:   "orders",
		Schema: "dbo",
		PrimaryKey: &model.PrimaryKeySchema{
			Name:        "PK_orders",
			IsClustered: boolPtr(true),
			Columns:     []string{"id", "line"},
		},
	}
	regularIndex := &model.IndexSchema{
		Name:           "IX_orders_name",
		NonUnique:      false,
		IsClustered:    boolPtr(false),
		Columns:        []model.IndexColumn{{Name: "name"}, {Name: "id"}},
		IncludeColumns: []string{"status"},
		Filter:         "[name] IS NOT NULL",
	}
	uniqueConstraint := &model.IndexSchema{
		Name:           "UQ_orders_code",
		NonUnique:      false,
		IsConstraint:   true,
		ConstraintType: "UNIQUE",
		IsClustered:    boolPtr(false),
		Columns:        []model.IndexColumn{{Name: "code"}},
	}

	tests := []struct {
		name string
		run  func() []string
		want []string
	}{
		{
			name: "新增主键",
			run:  func() []string { return e.AddPrimaryKey(table) },
			want: []string{"ALTER TABLE [dbo].[orders] ADD CONSTRAINT [PK_orders] PRIMARY KEY CLUSTERED ([id], [line]);"},
		},
		{
			name: "删除主键",
			run:  func() []string { return e.DropPrimaryKey(table) },
			want: []string{"ALTER TABLE [dbo].[orders] DROP CONSTRAINT [PK_orders];"},
		},
		{
			name: "新增普通索引",
			run:  func() []string { return e.AddIndex(table, regularIndex) },
			want: []string{"CREATE UNIQUE NONCLUSTERED INDEX [IX_orders_name] ON [dbo].[orders] ([name], [id]) INCLUDE ([status]) WHERE [name] IS NOT NULL;"},
		},
		{
			name: "删除普通索引",
			run:  func() []string { return e.DropIndex(table, regularIndex) },
			want: []string{"DROP INDEX [IX_orders_name] ON [dbo].[orders];"},
		},
		{
			name: "新增唯一约束",
			run:  func() []string { return e.AddIndex(table, uniqueConstraint) },
			want: []string{"ALTER TABLE [dbo].[orders] ADD CONSTRAINT [UQ_orders_code] UNIQUE NONCLUSTERED ([code]);"},
		},
		{
			name: "删除唯一约束",
			run:  func() []string { return e.DropIndex(table, uniqueConstraint) },
			want: []string{"ALTER TABLE [dbo].[orders] DROP CONSTRAINT [UQ_orders_code];"},
		},
	}
	for _, tt := range tests {
		assertStatements(t, tt.name, tt.run(), tt.want)
	}
}

func TestEmitterCreateTable(t *testing.T) {
	e := &Emitter{}
	table := &model.TableSchema{
		Name:    "orders",
		Schema:  "dbo",
		Comment: "订单",
		Columns: []*model.ColumnSchema{
			{Name: "id", ColumnType: "int", IsNullable: "NO", IsIdentity: true, Extra: "identity"},
			{
				Name: "name", ColumnType: "nvarchar(50)", IsNullable: "YES", Comment: "名称",
				ColumnDefault: strPtr("('x')"), DefaultConstraintName: "DF_orders_name",
			},
			{Name: "total", ColumnType: "int", IsNullable: "YES", IsGenerated: true, GenerationExpression: "([qty]*[price])"},
		},
		PrimaryKey: &model.PrimaryKeySchema{
			Name:        "PK_orders",
			IsClustered: boolPtr(true),
			Columns:     []string{"id"},
		},
		Indexes: []*model.IndexSchema{
			{
				Name:           "IX_orders_name",
				NonUnique:      true,
				IsClustered:    boolPtr(false),
				Columns:        []model.IndexColumn{{Name: "name"}},
				IncludeColumns: []string{"id"},
				Filter:         "[name] IS NOT NULL",
			},
			{
				Name:           "UQ_orders_name",
				NonUnique:      false,
				IsConstraint:   true,
				ConstraintType: "UNIQUE",
				IsClustered:    boolPtr(false),
				Columns:        []model.IndexColumn{{Name: "name"}},
			},
		},
	}

	want := []string{
		"CREATE TABLE [dbo].[orders] (\n" +
			"  [id] int IDENTITY(1,1) NOT NULL,\n" +
			"  [name] nvarchar(50) NULL CONSTRAINT [DF_orders_name] DEFAULT ('x'),\n" +
			"  [total] AS ([qty]*[price]),\n" +
			"  CONSTRAINT [PK_orders] PRIMARY KEY CLUSTERED ([id]),\n" +
			"  CONSTRAINT [UQ_orders_name] UNIQUE NONCLUSTERED ([name])\n" +
			");",
		"CREATE NONCLUSTERED INDEX [IX_orders_name] ON [dbo].[orders] ([name]) INCLUDE ([id]) WHERE [name] IS NOT NULL;",
		"IF EXISTS (SELECT 1 FROM sys.extended_properties WHERE class = 1 AND major_id = OBJECT_ID(N'[dbo].[orders]') AND minor_id = 0 AND name = N'MS_Description') EXEC sp_updateextendedproperty N'MS_Description', N'订单', N'SCHEMA', N'dbo', N'TABLE', N'orders' ELSE EXEC sp_addextendedproperty N'MS_Description', N'订单', N'SCHEMA', N'dbo', N'TABLE', N'orders';",
		"IF EXISTS (SELECT 1 FROM sys.extended_properties WHERE class = 1 AND major_id = OBJECT_ID(N'[dbo].[orders]') AND minor_id = COLUMNPROPERTY(OBJECT_ID(N'[dbo].[orders]'), N'name', 'ColumnId') AND name = N'MS_Description') EXEC sp_updateextendedproperty N'MS_Description', N'名称', N'SCHEMA', N'dbo', N'TABLE', N'orders', N'COLUMN', N'name' ELSE EXEC sp_addextendedproperty N'MS_Description', N'名称', N'SCHEMA', N'dbo', N'TABLE', N'orders', N'COLUMN', N'name';",
	}
	assertStatements(t, "建表语句", e.CreateTable(table), want)
}

func TestEmitterDML(t *testing.T) {
	e := &Emitter{}
	identityTable := &model.TableSchema{
		Name:   "orders",
		Schema: "dbo",
		Columns: []*model.ColumnSchema{
			{Name: "id", IsIdentity: true},
			{Name: "name"},
		},
	}
	plainTable := &model.TableSchema{Name: "logs", Schema: "dbo"}

	tests := []struct {
		name string
		run  func() []string
		want []string
	}{
		{
			name: "插入行",
			run: func() []string {
				return []string{e.InsertRow(identityTable, []string{"id", "name"}, map[string]interface{}{"id": int64(1), "name": "alice"})}
			},
			want: []string{"INSERT INTO [dbo].[orders] ([id], [name]) VALUES (1, N'alice');"},
		},
		{
			name: "开启 IDENTITY_INSERT",
			run:  func() []string { return e.TableDMLBegin(identityTable, true) },
			want: []string{"SET IDENTITY_INSERT [dbo].[orders] ON;"},
		},
		{
			name: "无需插入时不开启 IDENTITY_INSERT",
			run:  func() []string { return e.TableDMLBegin(identityTable, false) },
			want: nil,
		},
		{
			name: "无 IDENTITY 列不开启 IDENTITY_INSERT",
			run:  func() []string { return e.TableDMLBegin(plainTable, true) },
			want: nil,
		},
		{
			name: "关闭 IDENTITY_INSERT",
			run:  func() []string { return e.TableDMLEnd(identityTable, true) },
			want: []string{"SET IDENTITY_INSERT [dbo].[orders] OFF;"},
		},
		{
			name: "按主键更新",
			run: func() []string {
				return []string{e.UpdateRow(plainTable, []string{"id"},
					map[string]interface{}{"b": "x", "a": 1},
					map[string]interface{}{"id": "k"}, []string{"id", "a", "b"})}
			},
			want: []string{"UPDATE [dbo].[logs] SET [a] = 1, [b] = N'x' WHERE [id] = N'k';"},
		},
		{
			name: "无主键按全部列更新",
			run: func() []string {
				return []string{e.UpdateRow(plainTable, nil,
					map[string]interface{}{"b": "x"},
					map[string]interface{}{"a": 1, "b": "x", "c": nil}, []string{"a", "b", "c"})}
			},
			want: []string{"UPDATE [dbo].[logs] SET [b] = N'x' WHERE [a] = 1 AND [c] IS NULL;"},
		},
		{
			name: "按主键删除",
			run: func() []string {
				return []string{e.DeleteRow(plainTable, []string{"id"}, []string{"id", "a"},
					map[string]interface{}{"id": int64(1), "a": "x"})}
			},
			want: []string{"DELETE FROM [dbo].[logs] WHERE [id] = 1;"},
		},
		{
			name: "无主键使用 DELETE TOP (1)",
			run: func() []string {
				return []string{e.DeleteRow(plainTable, nil, []string{"a", "b"},
					map[string]interface{}{"a": int64(1), "b": "x"})}
			},
			want: []string{"DELETE TOP (1) FROM [dbo].[logs] WHERE [a] = 1 AND [b] = N'x';"},
		},
	}
	for _, tt := range tests {
		assertStatements(t, tt.name, tt.run(), tt.want)
	}
}

func TestEmitterSessionWrapping(t *testing.T) {
	e := &Emitter{}
	assertStatements(t, "会话开始", e.SessionBegin(), []string{"SET XACT_ABORT ON;", "BEGIN TRANSACTION;"})
	assertStatements(t, "会话结束", e.SessionEnd(), []string{"COMMIT;"})
	assertStatements(t, "DDL 开始", e.DDLBegin(), nil)
	assertStatements(t, "DDL 结束", e.DDLEnd(), nil)
	assertStatements(t, "DML 开始", e.DMLBegin(), nil)
	assertStatements(t, "DML 结束", e.DMLEnd(), nil)
}

func TestEmitterEquivalence(t *testing.T) {
	e := &Emitter{}

	if !e.ColumnEquivalent(
		&model.ColumnSchema{Name: "c", ColumnType: "int", IsNullable: "NO", ColumnDefault: strPtr("((0))")},
		&model.ColumnSchema{Name: "c", ColumnType: "INT", IsNullable: "no", ColumnDefault: strPtr("(0)")},
	) {
		t.Error("类型大小写与默认值外层括号差异应视为等价")
	}
	if e.ColumnEquivalent(
		&model.ColumnSchema{Name: "c", ColumnType: "int", IsNullable: "NO"},
		&model.ColumnSchema{Name: "c", ColumnType: "bigint", IsNullable: "NO"},
	) {
		t.Error("类型变化不应视为等价")
	}
	if e.ColumnEquivalent(
		&model.ColumnSchema{Name: "c", ColumnType: "int", Comment: "旧"},
		&model.ColumnSchema{Name: "c", ColumnType: "int", Comment: "新"},
	) {
		t.Error("注释变化不应视为等价")
	}

	base := &model.IndexSchema{
		Name:           "IX_c",
		NonUnique:      false,
		IsClustered:    boolPtr(false),
		Columns:        []model.IndexColumn{{Name: "a"}, {Name: "b"}},
		IncludeColumns: []string{"c"},
		Filter:         "[a] IS NOT NULL",
	}
	equivalent := &model.IndexSchema{
		Name:           "IX_c",
		NonUnique:      false,
		IsClustered:    nil,
		IndexType:      "NONCLUSTERED",
		Columns:        []model.IndexColumn{{Name: "a"}, {Name: "b"}},
		IncludeColumns: []string{"c"},
		Filter:         "[a] IS NOT NULL",
	}
	if !e.IndexEquivalent(base, equivalent) {
		t.Error("聚集方式与列定义相同的索引应视为等价")
	}
	changedInclude := *equivalent
	changedInclude.IncludeColumns = []string{"d"}
	if e.IndexEquivalent(base, &changedInclude) {
		t.Error("包含列变化不应视为等价")
	}
	changedFilter := *equivalent
	changedFilter.Filter = "[a] IS NULL"
	if e.IndexEquivalent(base, &changedFilter) {
		t.Error("筛选条件变化不应视为等价")
	}
}
