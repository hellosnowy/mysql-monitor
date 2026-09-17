package diff

import (
	"strings"
	"testing"

	"mysql-monitor/internal/model"
)

func TestDDLDiff(t *testing.T) {
	// 版本 A 表结构
	defValZero := "0"
	fromSchema := &model.DatabaseSchema{
		DatabaseName: "testdb",
		Tables: map[string]*model.TableSchema{
			"t_user": {
				Name:   "t_user",
				Engine: "InnoDB",
				Columns: []*model.ColumnSchema{
					{
						Name:          "id",
						OrdinalPosition: 1,
						ColumnType:    "bigint(20)",
						IsNullable:    "NO",
						Extra:         "auto_increment",
					},
					{
						Name:          "username",
						OrdinalPosition: 2,
						ColumnType:    "varchar(50)",
						IsNullable:    "NO",
					},
					{
						Name:          "obsolete_col",
						OrdinalPosition: 3,
						ColumnType:    "varchar(100)",
						IsNullable:    "YES",
					},
				},
				PrimaryKey: &model.PrimaryKeySchema{
					Columns: []string{"id"},
				},
				Indexes: []*model.IndexSchema{
					{
						Name:      "idx_obsolete",
						NonUnique: true,
						Columns: []model.IndexColumn{
							{Name: "obsolete_col", SeqInIndex: 1},
						},
					},
				},
			},
			"t_legacy": {
				Name: "t_legacy",
				Columns: []*model.ColumnSchema{
					{Name: "id", ColumnType: "int", IsNullable: "NO"},
				},
			},
		},
	}

	// 版本 B 表结构：
	// 1. 删除 t_legacy
	// 2. 新增 t_order
	// 3. 修改 t_user: 删除 obsolete_col，新增 email，修改 username 长度为 varchar(100)，删除 idx_obsolete，新增 idx_username 唯一索引
	toSchema := &model.DatabaseSchema{
		DatabaseName: "testdb",
		Tables: map[string]*model.TableSchema{
			"t_user": {
				Name:   "t_user",
				Engine: "InnoDB",
				Columns: []*model.ColumnSchema{
					{
						Name:          "id",
						OrdinalPosition: 1,
						ColumnType:    "bigint(20)",
						IsNullable:    "NO",
						Extra:         "auto_increment",
					},
					{
						Name:          "username",
						OrdinalPosition: 2,
						ColumnType:    "varchar(100)",
						IsNullable:    "NO",
					},
					{
						Name:          "email",
						OrdinalPosition: 3,
						ColumnType:    "varchar(100)",
						IsNullable:    "YES",
					},
				},
				PrimaryKey: &model.PrimaryKeySchema{
					Columns: []string{"id"},
				},
				Indexes: []*model.IndexSchema{
					{
						Name:      "idx_username",
						NonUnique: false, // UNIQUE
						Columns: []model.IndexColumn{
							{Name: "username", SeqInIndex: 1},
						},
					},
				},
			},
			"t_order": {
				Name:   "t_order",
				Engine: "InnoDB",
				Columns: []*model.ColumnSchema{
					{
						Name:          "order_id",
						OrdinalPosition: 1,
						ColumnType:    "varchar(64)",
						IsNullable:    "NO",
					},
					{
						Name:          "status",
						OrdinalPosition: 2,
						ColumnType:    "int(11)",
						IsNullable:    "NO",
						ColumnDefault: &defValZero,
					},
				},
				PrimaryKey: &model.PrimaryKeySchema{
					Columns: []string{"order_id"},
				},
			},
		},
	}

	diffs, summary := DiffSchemas(fromSchema, toSchema)

	// 校验统计汇总
	if summary.AddedTables != 1 {
		t.Errorf("AddedTables = %d, want 1", summary.AddedTables)
	}
	if summary.DroppedTables != 1 {
		t.Errorf("DroppedTables = %d, want 1", summary.DroppedTables)
	}
	if summary.ModifiedTables != 1 {
		t.Errorf("ModifiedTables = %d, want 1", summary.ModifiedTables)
	}
	if summary.AddedColumns != 1 {
		t.Errorf("AddedColumns = %d, want 1 (email)", summary.AddedColumns)
	}
	if summary.DroppedColumns != 1 {
		t.Errorf("DroppedColumns = %d, want 1 (obsolete_col)", summary.DroppedColumns)
	}
	if summary.ModifiedColumns != 1 {
		t.Errorf("ModifiedColumns = %d, want 1 (username)", summary.ModifiedColumns)
	}
	if summary.AddedIndexes != 1 {
		t.Errorf("AddedIndexes = %d, want 1 (idx_username)", summary.AddedIndexes)
	}
	if summary.DroppedIndexes != 1 {
		t.Errorf("DroppedIndexes = %d, want 1 (idx_obsolete)", summary.DroppedIndexes)
	}

	// 校验 DDL 生成的关键字
	var allStatements []string
	for _, d := range diffs {
		allStatements = append(allStatements, d.Statements...)
	}
	joined := strings.Join(allStatements, "\n")

	if !strings.Contains(joined, "CREATE TABLE `t_order`") {
		t.Errorf("缺少新建表 t_order 语句: %s", joined)
	}
	if !strings.Contains(joined, "DROP TABLE IF EXISTS `t_legacy`;") {
		t.Errorf("缺少删除表 t_legacy 语句: %s", joined)
	}
	if !strings.Contains(joined, "DROP COLUMN `obsolete_col`") {
		t.Errorf("缺少删除字段语句: %s", joined)
	}
	if !strings.Contains(joined, "ADD COLUMN `email`") {
		t.Errorf("缺少新增字段语句: %s", joined)
	}
	if !strings.Contains(joined, "MODIFY COLUMN `username` varchar(100)") {
		t.Errorf("缺少修改字段语句: %s", joined)
	}
	if !strings.Contains(joined, "ADD UNIQUE INDEX `idx_username`") {
		t.Errorf("缺少新增唯一索引语句: %s", joined)
	}
}

func TestDMLDiff(t *testing.T) {
	fromData := &model.DatabaseData{
		Tables: map[string]*model.TableData{
			"t_user": {
				TableName:         "t_user",
				PrimaryKeyColumns: []string{"id"},
				ColumnNames:       []string{"id", "username", "status"},
				Rows: []map[string]interface{}{
					{"id": int64(1), "username": "alice", "status": 1},
					{"id": int64(2), "username": "bob", "status": 1}, // 将被删除
					{"id": int64(3), "username": "charlie", "status": 0}, // 将被修改
				},
			},
		},
	}

	toData := &model.DatabaseData{
		Tables: map[string]*model.TableData{
			"t_user": {
				TableName:         "t_user",
				PrimaryKeyColumns: []string{"id"},
				ColumnNames:       []string{"id", "username", "status"},
				Rows: []map[string]interface{}{
					{"id": int64(1), "username": "alice", "status": 1}, // 未变动
					{"id": int64(3), "username": "charlie", "status": 1}, // status 0 -> 1 修改
					{"id": int64(4), "username": "david", "status": 1}, // 新增
				},
			},
		},
	}

	diffs, summary := DiffData(fromData, toData)

	if summary.InsertCount != 1 {
		t.Errorf("InsertCount = %d, want 1", summary.InsertCount)
	}
	if summary.DeleteCount != 1 {
		t.Errorf("DeleteCount = %d, want 1", summary.DeleteCount)
	}
	if summary.UpdateCount != 1 {
		t.Errorf("UpdateCount = %d, want 1", summary.UpdateCount)
	}

	if len(diffs) != 1 {
		t.Fatalf("TableDMLDiffs count = %d, want 1", len(diffs))
	}

	userDiff := diffs[0]
	if len(userDiff.InsertStatements) != 1 || !strings.Contains(userDiff.InsertStatements[0], "david") {
		t.Errorf("生成的 INSERT 语句不匹配: %v", userDiff.InsertStatements)
	}
	if len(userDiff.DeleteStatements) != 1 || !strings.Contains(userDiff.DeleteStatements[0], "`id` = 2") {
		t.Errorf("生成的 DELETE 语句不匹配: %v", userDiff.DeleteStatements)
	}
	if len(userDiff.UpdateStatements) != 1 || !strings.Contains(userDiff.UpdateStatements[0], "`status` = 1") {
		t.Errorf("生成的 UPDATE 语句不匹配: %v", userDiff.UpdateStatements)
	}
}

func TestGenerateDiffResult(t *testing.T) {
	fromSnap := &model.Snapshot{
		Meta: model.VersionMeta{VersionID: "v1"},
		Schema: &model.DatabaseSchema{
			Tables: map[string]*model.TableSchema{
				"t1": {Name: "t1", Columns: []*model.ColumnSchema{{Name: "id", ColumnType: "int"}}},
			},
		},
		Data: &model.DatabaseData{
			Tables: map[string]*model.TableData{
				"t1": {
					TableName:         "t1",
					PrimaryKeyColumns: []string{"id"},
					ColumnNames:       []string{"id", "val"},
					Rows:              []map[string]interface{}{{"id": 1, "val": "old"}},
				},
			},
		},
	}

	toSnap := &model.Snapshot{
		Meta: model.VersionMeta{VersionID: "v2"},
		Schema: &model.DatabaseSchema{
			Tables: map[string]*model.TableSchema{
				"t1": {Name: "t1", Columns: []*model.ColumnSchema{{Name: "id", ColumnType: "int"}}},
			},
		},
		Data: &model.DatabaseData{
			Tables: map[string]*model.TableData{
				"t1": {
					TableName:         "t1",
					PrimaryKeyColumns: []string{"id"},
					ColumnNames:       []string{"id", "val"},
					Rows:              []map[string]interface{}{{"id": 1, "val": "new"}},
				},
			},
		},
	}

	res := GenerateDiffResult("dev_conn", "v1", "v2", fromSnap, toSnap)

	if res.Summary.DML.UpdateCount != 1 {
		t.Errorf("UpdateCount = %d, want 1", res.Summary.DML.UpdateCount)
	}
	if !strings.Contains(res.FullScript, "START TRANSACTION;") {
		t.Errorf("脚本未包含事务开启")
	}
	if !strings.Contains(res.FullScript, "COMMIT;") {
		t.Errorf("脚本未包含事务提交")
	}
}
