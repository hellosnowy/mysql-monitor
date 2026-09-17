package storage

import "mysql-monitor/internal/model"

// Storage 数据持久化抽象接口
type Storage interface {
	// SaveConnection 保存或更新数据库连接配置
	SaveConnection(cfg model.ConnectionConfig) error
	// GetConnection 根据连接 ID 获取数据库连接配置
	GetConnection(id string) (*model.ConnectionConfig, error)
	// ListConnections 获取所有已保存的数据库连接配置列表
	ListConnections() ([]model.ConnectionConfig, error)
	// DeleteConnection 删除指定的数据库连接配置
	DeleteConnection(id string) error

	// SaveSnapshot 保存完整的数据库快照
	SaveSnapshot(snapshot *model.Snapshot) error
	// GetSnapshot 根据连接 ID 与版本 ID 读取对应快照
	GetSnapshot(connID string, versionID string) (*model.Snapshot, error)
	// ListSnapshots 获取指定连接下的所有历史版本元数据（按时间倒序排列）
	ListSnapshots(connID string) ([]model.VersionMeta, error)
	// DeleteSnapshot 删除某个历史快照版本
	DeleteSnapshot(connID string, versionID string) error
	// HasSnapshots 判断指定连接下是否存在历史版本快照
	HasSnapshots(connID string) (bool, error)
}
