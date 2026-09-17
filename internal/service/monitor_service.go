package service

import (
	"context"
	"fmt"
	"time"

	"mysql-monitor/internal/db"
	"mysql-monitor/internal/diff"
	"mysql-monitor/internal/filter"
	"mysql-monitor/internal/model"
	"mysql-monitor/internal/storage"
)

// MonitorService 聚合业务编排服务
type MonitorService struct {
	storage storage.Storage
}

// NewMonitorService 创建业务服务实例
func NewMonitorService(s storage.Storage) *MonitorService {
	return &MonitorService{
		storage: s,
	}
}

// SaveConnection 保存或更新数据库连接
func (s *MonitorService) SaveConnection(cfg model.ConnectionConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return s.storage.SaveConnection(cfg)
}

// GetConnection 获取单个数据库连接
func (s *MonitorService) GetConnection(id string) (*model.ConnectionConfig, error) {
	return s.storage.GetConnection(id)
}

// ListConnections 获取所有保存的数据库连接
func (s *MonitorService) ListConnections() ([]model.ConnectionConfig, error) {
	return s.storage.ListConnections()
}

// DeleteConnection 删除数据库连接
func (s *MonitorService) DeleteConnection(id string) error {
	return s.storage.DeleteConnection(id)
}

// TestConnection 测试连接连通性并返回 MySQL 版本与延迟
func (s *MonitorService) TestConnection(cfg model.ConnectionConfig) (*model.ConnectionTestResult, error) {
	return db.TestConnection(&cfg)
}

// CaptureSnapshot 抓取指定数据库的当前结构和数据快照
// 核心规则：若数据库历史版本为空，自动将其确立为基准版本（Baseline）
func (s *MonitorService) CaptureSnapshot(ctx context.Context, connID string, forceBaseline bool, description string) (*model.VersionMeta, error) {
	cfg, err := s.storage.GetConnection(connID)
	if err != nil {
		return nil, fmt.Errorf("找不到连接配置: %w", err)
	}

	// 1. 判断是否设为基准版本
	hasExisting, err := s.storage.HasSnapshots(connID)
	if err != nil {
		return nil, fmt.Errorf("检查历史快照失败: %w", err)
	}

	isBaseline := forceBaseline
	if !hasExisting {
		// 历史版本为空时，自动标记为基准版本
		isBaseline = true
		if description == "" {
			description = "初始基准版本 (Baseline)"
		}
	}

	// 2. 连接目标 MySQL
	client, err := db.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("连接目标数据库失败: %w", err)
	}
	defer client.Close()

	// 3. 构建表过滤器
	tableFilter := filter.NewTableFilter(cfg.Filter)

	// 4. 提取表结构 (DDL)
	schemaExtractor := db.NewSchemaExtractor(client.DB(), cfg.Database, tableFilter)
	schema, err := schemaExtractor.ExtractDatabaseSchema(ctx)
	if err != nil {
		return nil, fmt.Errorf("抓取表结构失败: %w", err)
	}

	// 5. 提取表数据 (DML)
	dataExtractor := db.NewDataExtractor(client.DB(), cfg.Database, cfg.Filter)
	data, err := dataExtractor.ExtractDatabaseData(ctx, schema)
	if err != nil {
		return nil, fmt.Errorf("抓取表数据快照失败: %w", err)
	}

	// 统计数据总量
	totalRows := 0
	if data != nil {
		for _, tData := range data.Tables {
			totalRows += tData.TotalRows
		}
	}

	// 6. 生成版本号并持久化
	versionID := fmt.Sprintf("v%s", time.Now().Format("20060102_150405"))
	meta := model.VersionMeta{
		VersionID:    versionID,
		ConnectionID: connID,
		DatabaseName: cfg.Database,
		IsBaseline:   isBaseline,
		Description:  description,
		TableCount:   len(schema.Tables),
		RowCount:     totalRows,
		CreatedAt:    time.Now(),
	}

	snapshot := &model.Snapshot{
		Meta:   meta,
		Schema: schema,
		Data:   data,
	}

	if err := s.storage.SaveSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("保存快照失败: %w", err)
	}

	return &meta, nil
}

// ListSnapshots 获取指定连接的所有快照版本列表
func (s *MonitorService) ListSnapshots(connID string) ([]model.VersionMeta, error) {
	return s.storage.ListSnapshots(connID)
}

// GetSnapshot 获取指定版本的快照
func (s *MonitorService) GetSnapshot(connID, versionID string) (*model.Snapshot, error) {
	return s.storage.GetSnapshot(connID, versionID)
}

// DeleteSnapshot 删除指定版本的快照
func (s *MonitorService) DeleteSnapshot(connID, versionID string) error {
	return s.storage.DeleteSnapshot(connID, versionID)
}

// CompareVersions 比较历史版本中任意两个版本的差异，生成 DDL 与 DML SQL 脚本
func (s *MonitorService) CompareVersions(connID, fromVersionID, toVersionID string) (*model.DiffResult, error) {
	if fromVersionID == "" || toVersionID == "" {
		return nil, fmt.Errorf("起始版本和目标版本均不能为空")
	}

	fromSnap, err := s.storage.GetSnapshot(connID, fromVersionID)
	if err != nil {
		return nil, fmt.Errorf("读取起始版本快照失败: %w", err)
	}

	toSnap, err := s.storage.GetSnapshot(connID, toVersionID)
	if err != nil {
		return nil, fmt.Errorf("读取目标版本快照失败: %w", err)
	}

	result := diff.GenerateDiffResult(connID, fromVersionID, toVersionID, fromSnap, toSnap)
	return result, nil
}
