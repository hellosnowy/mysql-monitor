package service

import (
	"context"
	"fmt"
	"time"

	"sql-monitor/internal/dialect"
	"sql-monitor/internal/filter"
	"sql-monitor/internal/model"
	"sql-monitor/internal/storage"
)

// MonitorService 聚合业务编排服务
type MonitorService struct {
	storage  storage.Storage
	registry *dialect.Registry
}

// NewMonitorService 创建业务服务实例
func NewMonitorService(s storage.Storage, registry *dialect.Registry) *MonitorService {
	return &MonitorService{
		storage:  s,
		registry: registry,
	}
}

// Vendors 返回所有内置数据库方言
func (s *MonitorService) Vendors() []dialect.DriverInfo {
	return s.registry.List()
}

// SaveConnection 保存或更新数据库连接
func (s *MonitorService) SaveConnection(cfg model.ConnectionConfig) error {
	cfg.Normalize()
	p, err := s.registry.Get(cfg.Vendor)
	if err != nil {
		return err
	}
	if err := p.Validate(&cfg); err != nil {
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
	conns, err := s.storage.ListConnections()
	if err != nil {
		return nil, err
	}
	for i := range conns {
		conns[i].Normalize()
	}
	return conns, nil
}

// DeleteConnection 删除数据库连接
func (s *MonitorService) DeleteConnection(id string) error {
	return s.storage.DeleteConnection(id)
}

// TestConnection 测试连接连通性并返回服务端版本与延迟
func (s *MonitorService) TestConnection(cfg model.ConnectionConfig) (*model.ConnectionTestResult, error) {
	cfg.Normalize()
	p, err := s.registry.Get(cfg.Vendor)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(&cfg); err != nil {
		return &model.ConnectionTestResult{Success: false, Message: err.Error()}, nil
	}
	return p.TestConnection(context.Background(), &cfg)
}

// ListSchemas 列出指定连接可用的数据库模式
func (s *MonitorService) ListSchemas(cfg model.ConnectionConfig) ([]string, error) {
	cfg.Normalize()
	p, err := s.registry.Get(cfg.Vendor)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(&cfg); err != nil {
		return nil, err
	}
	return p.ListSchemas(context.Background(), &cfg)
}

// CaptureSnapshot 抓取指定数据库的当前结构和数据快照
// 核心规则：若数据库历史版本为空，自动将其确立为基准版本（Baseline）
func (s *MonitorService) CaptureSnapshot(ctx context.Context, connID string, forceBaseline bool, description string) (*model.VersionMeta, error) {
	cfg, err := s.storage.GetConnection(connID)
	if err != nil {
		return nil, fmt.Errorf("找不到连接配置: %w", err)
	}
	cfg.Normalize()

	p, err := s.registry.Get(cfg.Vendor)
	if err != nil {
		return nil, err
	}

	// 1. 判断是否设为基准版本
	hasExisting, err := s.storage.HasSnapshots(connID)
	if err != nil {
		return nil, fmt.Errorf("检查历史快照失败: %w", err)
	}

	isBaseline := forceBaseline
	if !hasExisting {
		isBaseline = true
		if description == "" {
			description = "初始基准版本 (Baseline)"
		}
	}

	// 2. 由方言插件完成连接、结构提取与数据提取
	tableFilter := filter.NewTableFilter(cfg.Filter)
	schema, data, err := p.Capture(ctx, cfg, tableFilter)
	if err != nil {
		return nil, err
	}

	totalRows := 0
	if data != nil {
		for _, tData := range data.Tables {
			totalRows += tData.TotalRows
		}
	}

	// 3. 生成版本号并持久化
	now := time.Now()
	versionID := fmt.Sprintf("v%s_%03d", now.Format("20060102_150405"), now.Nanosecond()/int(time.Millisecond))
	meta := model.VersionMeta{
		VersionID:     versionID,
		ConnectionID:  connID,
		Vendor:        cfg.Vendor,
		DatabaseName:  cfg.Database,
		Schema:        cfg.EffectiveSchema(),
		FormatVersion: model.SnapshotFormatVersion,
		IsBaseline:    isBaseline,
		Description:   description,
		TableCount:    len(schema.Tables),
		RowCount:      totalRows,
		CreatedAt:     now,
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

// CompareVersions 比较历史版本中任意两个版本的差异，生成 DDL 与 DML SQL 脚本。
// direction 为 rollback 时生成由目标版本回滚到起始版本的 SQL。
func (s *MonitorService) CompareVersions(connID, fromVersionID, toVersionID string, direction model.DiffDirection) (*model.DiffResult, error) {
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
	fromSnap.Normalize()
	toSnap.Normalize()

	vendor := fromSnap.Meta.Vendor
	if vendor == "" {
		vendor = toSnap.Meta.Vendor
	}
	if vendor == "" {
		if cfg, cfgErr := s.storage.GetConnection(connID); cfgErr == nil {
			vendor = cfg.Vendor
		}
	}
	vendor = model.NormalizeVendor(vendor)
	if toSnap.Meta.Vendor != "" && toSnap.Meta.Vendor != vendor {
		return nil, fmt.Errorf("两个快照的数据库类型不一致: %s 与 %s", vendor.DisplayName(), toSnap.Meta.Vendor.DisplayName())
	}

	p, err := s.registry.Get(vendor)
	if err != nil {
		return nil, err
	}
	return p.Compare(fromSnap, toSnap, model.NormalizeDirection(direction))
}
