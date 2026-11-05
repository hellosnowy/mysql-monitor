package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sql-monitor/internal/model"
)

// TestLegacyMySQLDataLoads 验证 mysql-monitor 老版本（无 vendor 字段）的配置与快照可直接读取
func TestLegacyMySQLDataLoads(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStorage(dir)
	if err != nil {
		t.Fatalf("初始化存储失败: %v", err)
	}

	// 老版本 connections.json：没有 vendor / schema 字段
	connections := []map[string]interface{}{
		{
			"id":              "legacy",
			"name":            "老版本连接",
			"host":            "127.0.0.1",
			"port":            3306,
			"user":            "root",
			"password":        "",
			"database":        "mall",
			"charset":         "utf8mb4",
			"timeout_seconds": 10,
			"filter": map[string]interface{}{
				"include_tables":     []string{},
				"exclude_tables":     []string{},
				"monitor_mode":       "schema_and_data",
				"max_rows_per_table": 0,
			},
		},
	}
	connBytes, _ := json.Marshal(connections)
	if err := os.WriteFile(filepath.Join(dir, "configs", "connections.json"), connBytes, 0644); err != nil {
		t.Fatalf("写入老配置失败: %v", err)
	}

	// 老版本快照：meta/schema 均无 vendor 字段
	versionDir := filepath.Join(dir, "snapshots", "legacy", "v20260101_000000")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("创建老快照目录失败: %v", err)
	}
	meta := `{
		"version_id": "v20260101_000000",
		"connection_id": "legacy",
		"database_name": "mall",
		"is_baseline": true,
		"description": "老版本基准",
		"table_count": 1,
		"row_count": 1,
		"created_at": "2026-01-01T00:00:00+08:00"
	}`
	schema := `{
		"database_name": "mall",
		"tables": {
			"t_user": {
				"name": "t_user",
				"engine": "InnoDB",
				"collation": "utf8mb4_general_ci",
				"comment": "",
				"create_table_sql": "CREATE TABLE ` + "`t_user`" + ` (` + "`id`" + ` bigint NOT NULL AUTO_INCREMENT, PRIMARY KEY (` + "`id`" + `)) ENGINE=InnoDB;",
				"columns": [
					{"name": "id", "ordinal_position": 1, "column_type": "bigint", "data_type": "bigint", "is_nullable": "NO", "column_default": null, "extra": "auto_increment", "comment": "", "character_set_name": "", "collation_name": ""}
				],
				"indexes": [],
				"primary_key": {"columns": ["id"]}
			}
		}
	}`
	data := `{
		"tables": {
			"t_user": {
				"table_name": "t_user",
				"primary_key_columns": ["id"],
				"column_names": ["id"],
				"rows": [{"id": 9007199254740993}],
				"total_rows": 1
			}
		}
	}`
	for name, content := range map[string]string{"meta.json": meta, "schema.json": schema, "data.json": data} {
		if err := os.WriteFile(filepath.Join(versionDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("写入老快照 %s 失败: %v", name, err)
		}
	}

	// 1. 连接配置自动归一化为 MySQL
	conns, err := store.ListConnections()
	if err != nil {
		t.Fatalf("读取老连接失败: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("连接数量不符: %d", len(conns))
	}
	if conns[0].Vendor != model.VendorMySQL {
		t.Errorf("老连接 vendor = %q, want mysql", conns[0].Vendor)
	}
	if conns[0].Port != 3306 || conns[0].Charset != "utf8mb4" {
		t.Errorf("老连接默认值未补齐: port=%d charset=%s", conns[0].Port, conns[0].Charset)
	}

	// 2. 快照自动归一化，且大整数精度保持
	snapshot, err := store.GetSnapshot("legacy", "v20260101_000000")
	if err != nil {
		t.Fatalf("读取老快照失败: %v", err)
	}
	if snapshot.Meta.Vendor != model.VendorMySQL {
		t.Errorf("老快照 vendor = %q, want mysql", snapshot.Meta.Vendor)
	}
	if snapshot.Meta.FormatVersion != 1 {
		t.Errorf("老快照格式版本 = %d, want 1", snapshot.Meta.FormatVersion)
	}
	if snapshot.Schema.Vendor != model.VendorMySQL {
		t.Errorf("老结构 vendor = %q, want mysql", snapshot.Schema.Vendor)
	}
	table := snapshot.Schema.Tables["t_user"]
	if table.PrimaryKey == nil || table.PrimaryKey.Name != "PRIMARY" {
		t.Errorf("老主键名称未补齐: %+v", table.PrimaryKey)
	}
	if len(table.Columns) != 1 || !table.Columns[0].IsIdentity {
		t.Errorf("auto_increment 未转换为 IsIdentity")
	}
	if got := snapshot.Data.Tables["t_user"].Rows[0]["id"]; got != json.Number("9007199254740993") {
		t.Errorf("大整数精度丢失: got %v (%T)", got, got)
	}

	// 3. 列表中的元数据同样归一化
	list, err := store.ListSnapshots("legacy")
	if err != nil || len(list) != 1 {
		t.Fatalf("读取老快照列表失败: %v", err)
	}
	if list[0].Vendor != model.VendorMySQL {
		t.Errorf("列表元数据 vendor = %q, want mysql", list[0].Vendor)
	}
}
