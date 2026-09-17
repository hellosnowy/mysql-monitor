package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"mysql-monitor/internal/model"
	"mysql-monitor/pkg/crypto"
)

// FileStorage 基于本地文件系统的结构化数据持久化实现
type FileStorage struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStorage 创建文件存储实例，baseDir 为数据根目录（例如 ./data）
func NewFileStorage(baseDir string) (*FileStorage, error) {
	if baseDir == "" {
		baseDir = "./data"
	}
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("解析数据目录绝对路径失败: %w", err)
	}

	// 初始化 configs 和 snapshots 目录
	if err := os.MkdirAll(filepath.Join(absPath, "configs"), 0755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(absPath, "snapshots"), 0755); err != nil {
		return nil, fmt.Errorf("创建快照目录失败: %w", err)
	}

	// 初始化密码加解密秘钥
	if err := crypto.InitGlobalCipher(absPath); err != nil {
		return nil, fmt.Errorf("初始化存储加密组件失败: %w", err)
	}

	return &FileStorage{
		baseDir: absPath,
	}, nil
}

// connectionsFilePath 获取连接配置文件路径
func (s *FileStorage) connectionsFilePath() string {
	return filepath.Join(s.baseDir, "configs", "connections.json")
}

// SaveConnection 保存或更新数据库连接配置
func (s *FileStorage) SaveConnection(cfg model.ConnectionConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conns, err := s.loadConnectionsNoLock()
	if err != nil {
		return err
	}

	now := time.Now()
	found := false
	for i, c := range conns {
		if c.ID == cfg.ID {
			// 如果更新时未提供新密码，保留原有密码
			if cfg.Password == "" || cfg.Password == "••••••••" {
				cfg.Password = c.Password
			}
			cfg.CreatedAt = c.CreatedAt
			cfg.UpdatedAt = now
			conns[i] = cfg
			found = true
			break
		}
	}
	if !found {
		cfg.CreatedAt = now
		cfg.UpdatedAt = now
		conns = append(conns, cfg)
	}

	return s.saveConnectionsNoLock(conns)
}

// GetConnection 获取指定 ID 的数据库连接
func (s *FileStorage) GetConnection(id string) (*model.ConnectionConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conns, err := s.loadConnectionsNoLock()
	if err != nil {
		return nil, err
	}

	for _, c := range conns {
		if c.ID == id {
			copied := c
			return &copied, nil
		}
	}
	return nil, fmt.Errorf("未找到 ID 为 %s 的数据库连接", id)
}

// ListConnections 列出所有已保存的数据库连接
func (s *FileStorage) ListConnections() ([]model.ConnectionConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conns, err := s.loadConnectionsNoLock()
	if err != nil {
		return nil, err
	}
	if conns == nil {
		return []model.ConnectionConfig{}, nil
	}
	return conns, nil
}

// DeleteConnection 删除指定数据库连接
func (s *FileStorage) DeleteConnection(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conns, err := s.loadConnectionsNoLock()
	if err != nil {
		return err
	}

	newConns := make([]model.ConnectionConfig, 0, len(conns))
	found := false
	for _, c := range conns {
		if c.ID == id {
			found = true
			continue
		}
		newConns = append(newConns, c)
	}

	if !found {
		return fmt.Errorf("连接 ID %s 不存在", id)
	}

	return s.saveConnectionsNoLock(newConns)
}

// loadConnectionsNoLock 读取连接配置文件内部实现（自动将存储的加密密码解密）
func (s *FileStorage) loadConnectionsNoLock() ([]model.ConnectionConfig, error) {
	filePath := s.connectionsFilePath()
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []model.ConnectionConfig{}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取连接配置文件失败: %w", err)
	}

	if len(data) == 0 {
		return []model.ConnectionConfig{}, nil
	}

	var conns []model.ConnectionConfig
	if err := json.Unmarshal(data, &conns); err != nil {
		return nil, fmt.Errorf("解析连接配置文件失败: %w", err)
	}

	cipher := crypto.GetGlobalCipher()
	for i := range conns {
		if conns[i].Password != "" && crypto.IsEncrypted(conns[i].Password) && cipher != nil {
			plain, err := cipher.Decrypt(conns[i].Password)
			if err == nil {
				conns[i].Password = plain
			}
		}
	}

	if conns == nil {
		return []model.ConnectionConfig{}, nil
	}

	return conns, nil
}

// saveConnectionsNoLock 保存连接配置到文件（自动将密码加密存储为 ENC:...）
func (s *FileStorage) saveConnectionsNoLock(conns []model.ConnectionConfig) error {
	filePath := s.connectionsFilePath()

	cipher := crypto.GetGlobalCipher()
	toSave := make([]model.ConnectionConfig, len(conns))
	copy(toSave, conns)

	for i := range toSave {
		if toSave[i].Password != "" && !crypto.IsEncrypted(toSave[i].Password) && cipher != nil {
			enc, err := cipher.Encrypt(toSave[i].Password)
			if err == nil {
				toSave[i].Password = enc
			}
		}
	}

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化连接配置失败: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入连接配置文件失败: %w", err)
	}
	return nil
}

// snapshotDir 获取指定连接和版本的存储目录
func (s *FileStorage) snapshotDir(connID, versionID string) string {
	return filepath.Join(s.baseDir, "snapshots", connID, versionID)
}

// SaveSnapshot 保存数据库快照（元数据、表结构、数据）
func (s *FileStorage) SaveSnapshot(snapshot *model.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if snapshot == nil {
		return fmt.Errorf("快照实体不能为 nil")
	}

	dir := s.snapshotDir(snapshot.Meta.ConnectionID, snapshot.Meta.VersionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建快照目录失败: %w", err)
	}

	// 1. 保存元数据 meta.json
	metaData, err := json.MarshalIndent(snapshot.Meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化快照元数据失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), metaData, 0644); err != nil {
		return fmt.Errorf("保存快照元数据失败: %w", err)
	}

	// 2. 保存表结构 schema.json
	schemaData, err := json.MarshalIndent(snapshot.Schema, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化表结构数据失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.json"), schemaData, 0644); err != nil {
		return fmt.Errorf("保存表结构数据失败: %w", err)
	}

	// 3. 保存行数据 data.json
	dataBytes, err := json.MarshalIndent(snapshot.Data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化行数据快照失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), dataBytes, 0644); err != nil {
		return fmt.Errorf("保存行数据快照失败: %w", err)
	}

	return nil
}

// GetSnapshot 读取快照元数据、表结构及数据内容
func (s *FileStorage) GetSnapshot(connID, versionID string) (*model.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.snapshotDir(connID, versionID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("未找到连接 %s 的版本 %s 快照", connID, versionID)
	}

	// 读取 meta.json
	metaFile := filepath.Join(dir, "meta.json")
	metaBytes, err := os.ReadFile(metaFile)
	if err != nil {
		return nil, fmt.Errorf("读取快照元数据失败: %w", err)
	}
	var meta model.VersionMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, fmt.Errorf("解析快照元数据失败: %w", err)
	}

	// 读取 schema.json
	schemaFile := filepath.Join(dir, "schema.json")
	schemaBytes, err := os.ReadFile(schemaFile)
	if err != nil {
		return nil, fmt.Errorf("读取快照表结构失败: %w", err)
	}
	var schema model.DatabaseSchema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil, fmt.Errorf("解析快照表结构失败: %w", err)
	}

	// 读取 data.json
	dataFile := filepath.Join(dir, "data.json")
	dataBytes, err := os.ReadFile(dataFile)
	if err != nil {
		return nil, fmt.Errorf("读取快照行数据失败: %w", err)
	}
	var dbData model.DatabaseData
	if err := json.Unmarshal(dataBytes, &dbData); err != nil {
		return nil, fmt.Errorf("解析快照行数据失败: %w", err)
	}

	return &model.Snapshot{
		Meta:   meta,
		Schema: &schema,
		Data:   &dbData,
	}, nil
}

// ListSnapshots 获取指定连接的所有快照版本，按创建时间倒序排列
func (s *FileStorage) ListSnapshots(connID string) ([]model.VersionMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connDir := filepath.Join(s.baseDir, "snapshots", connID)
	if _, err := os.Stat(connDir); os.IsNotExist(err) {
		return []model.VersionMeta{}, nil
	}

	entries, err := os.ReadDir(connDir)
	if err != nil {
		return nil, fmt.Errorf("读取快照列表目录失败: %w", err)
	}

	list := make([]model.VersionMeta, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(connDir, entry.Name(), "meta.json")
		if bytes, err := os.ReadFile(metaPath); err == nil {
			var meta model.VersionMeta
			if jsonErr := json.Unmarshal(bytes, &meta); jsonErr == nil {
				list = append(list, meta)
			}
		}
	}

	// 按创建时间倒序排列
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	return list, nil
}

// DeleteSnapshot 删除某个历史版本快照
func (s *FileStorage) DeleteSnapshot(connID, versionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.snapshotDir(connID, versionID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("版本 %s 不存在", versionID)
	}

	return os.RemoveAll(dir)
}

// HasSnapshots 判断连接下是否已有快照
func (s *FileStorage) HasSnapshots(connID string) (bool, error) {
	list, err := s.ListSnapshots(connID)
	if err != nil {
		return false, err
	}
	return len(list) > 0, nil
}
