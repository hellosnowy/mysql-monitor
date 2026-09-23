package storage

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"mysql-monitor/internal/model"
)

func TestFileStorage_Connections(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mysql-monitor-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewFileStorage(tempDir)
	if err != nil {
		t.Fatalf("初始化存储失败: %v", err)
	}

	cfg := model.ConnectionConfig{
		ID:       "dev_db",
		Name:     "开发环境数据库",
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Password: "password",
		Database: "mall",
		Filter: model.TableFilterConfig{
			IncludeTables: []string{"t_user", "t_order"},
			MonitorMode:   "schema_and_data",
		},
	}

	// 1. 保存连接
	if err := store.SaveConnection(cfg); err != nil {
		t.Fatalf("保存连接失败: %v", err)
	}

	// 2. 获取连接
	got, err := store.GetConnection("dev_db")
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	if got.Name != cfg.Name {
		t.Errorf("连接名称不符: got %s, want %s", got.Name, cfg.Name)
	}

	// 3. 列出连接
	list, err := store.ListConnections()
	if err != nil {
		t.Fatalf("列出连接失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("期望连接列表为 1, 实际为 %d", len(list))
	}

	// 4. 删除连接
	if err := store.DeleteConnection("dev_db"); err != nil {
		t.Fatalf("删除连接失败: %v", err)
	}
	listAfter, err := store.ListConnections()
	if err != nil {
		t.Fatalf("列出连接失败: %v", err)
	}
	if len(listAfter) != 0 {
		t.Fatalf("期望删除后连接列表为 0, 实际为 %d", len(listAfter))
	}
}

func TestFileStorage_Snapshots(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mysql-monitor-snapshot-test-*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewFileStorage(tempDir)
	if err != nil {
		t.Fatalf("初始化存储失败: %v", err)
	}

	has, err := store.HasSnapshots("dev_db")
	if err != nil {
		t.Fatalf("检查快照失败: %v", err)
	}
	if has {
		t.Errorf("初始应当无快照")
	}

	// 构造快照
	snapshot := &model.Snapshot{
		Meta: model.VersionMeta{
			VersionID:    "v20260917_100000",
			ConnectionID: "dev_db",
			DatabaseName: "mall",
			IsBaseline:   true,
			Description:  "基准版本测试",
			TableCount:   1,
			RowCount:     2,
			CreatedAt:    time.Now(),
		},
		Schema: &model.DatabaseSchema{
			DatabaseName: "mall",
			Tables: map[string]*model.TableSchema{
				"t_user": {
					Name:   "t_user",
					Engine: "InnoDB",
					Columns: []*model.ColumnSchema{
						{
							Name:       "id",
							ColumnType: "bigint(20)",
							IsNullable: "NO",
							Extra:      "auto_increment",
						},
					},
					PrimaryKey: &model.PrimaryKeySchema{
						Columns: []string{"id"},
					},
				},
			},
		},
		Data: &model.DatabaseData{
			Tables: map[string]*model.TableData{
				"t_user": {
					TableName:         "t_user",
					PrimaryKeyColumns: []string{"id"},
					ColumnNames:       []string{"id"},
					Rows: []map[string]interface{}{
						{"id": int64(1)},
						{"id": int64(9007199254740993)},
					},
					TotalRows: 2,
				},
			},
		},
	}

	// 保存快照
	if err := store.SaveSnapshot(snapshot); err != nil {
		t.Fatalf("保存快照失败: %v", err)
	}

	hasAfter, err := store.HasSnapshots("dev_db")
	if err != nil || !hasAfter {
		t.Fatalf("应当检测到已有快照")
	}

	// 读取快照
	loaded, err := store.GetSnapshot("dev_db", "v20260917_100000")
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	if loaded.Meta.Description != snapshot.Meta.Description {
		t.Errorf("快照描述不匹配: got %s, want %s", loaded.Meta.Description, snapshot.Meta.Description)
	}
	if len(loaded.Schema.Tables) != 1 {
		t.Errorf("快照表数量不匹配")
	}
	if len(loaded.Data.Tables["t_user"].Rows) != 2 {
		t.Errorf("快照行数据不匹配")
	}
	if got := loaded.Data.Tables["t_user"].Rows[1]["id"]; got != json.Number("9007199254740993") {
		t.Errorf("大整数精度丢失: got %v (%T)", got, got)
	}

	// 删除快照
	if err := store.DeleteSnapshot("dev_db", "v20260917_100000"); err != nil {
		t.Fatalf("删除快照失败: %v", err)
	}
}
