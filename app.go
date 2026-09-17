package main

import (
	"context"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mysql-monitor/internal/model"
	"mysql-monitor/internal/service"
	"mysql-monitor/internal/storage"
)

// App 结构体作为 Wails 应用的核心桥接层，其中的公共方法将自动暴露给前端 JS 调用
type App struct {
	ctx     context.Context
	service *service.MonitorService
}

// NewApp 创建新的 App 实例并初始化存储与服务层
func NewApp() (*App, error) {
	// 默认存储在用户主目录下的 .mysql-monitor 或当前应用目录下的 data
	store, err := storage.NewFileStorage("./data")
	if err != nil {
		return nil, fmt.Errorf("初始化存储失败: %w", err)
	}

	srv := service.NewMonitorService(store)
	return &App{
		service: srv,
	}, nil
}

// startup 在 Wails 桌面窗口初始化完成时由运行时回调
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// --- 1. 数据库连接管理 API ---

// GetConnections 获取所有已配置的数据库连接
func (a *App) GetConnections() ([]model.ConnectionConfig, error) {
	return a.service.ListConnections()
}

// SaveConnection 新增或保存数据库连接配置
func (a *App) SaveConnection(cfg model.ConnectionConfig) error {
	return a.service.SaveConnection(cfg)
}

// DeleteConnection 删除指定的数据库连接
func (a *App) DeleteConnection(id string) error {
	return a.service.DeleteConnection(id)
}

// TestConnection 测试数据库连接连通性并探测版本与延迟
func (a *App) TestConnection(cfg model.ConnectionConfig) (*model.ConnectionTestResult, error) {
	return a.service.TestConnection(cfg)
}

// --- 2. 快照与版本管理 API ---

// CaptureSnapshot 抓取当前数据库结构与数据快照
// 若当前连接下历史版本为空，后端自动将其标记为基准版本（Baseline）
func (a *App) CaptureSnapshot(connID string, forceBaseline bool, description string) (*model.VersionMeta, error) {
	return a.service.CaptureSnapshot(a.ctx, connID, forceBaseline, description)
}

// ListSnapshots 获取指定连接的所有历史快照版本
func (a *App) ListSnapshots(connID string) ([]model.VersionMeta, error) {
	return a.service.ListSnapshots(connID)
}


// DeleteSnapshot 删除某个历史快照版本
func (a *App) DeleteSnapshot(connID, versionID string) error {
	return a.service.DeleteSnapshot(connID, versionID)
}

// --- 3. 版本差异比对与 SQL 生成 API ---

// CompareVersions 比对两个历史版本并生成 DDL 与 DML SQL 脚本
func (a *App) CompareVersions(connID, fromVersionID, toVersionID string) (*model.DiffResult, error) {
	return a.service.CompareVersions(connID, fromVersionID, toVersionID)
}

// ExportSQLFile 弹出系统原生文件保存对话框并导出 SQL 文件到用户指定路径
func (a *App) ExportSQLFile(defaultName string, content string) (string, error) {
	filePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "导出 SQL 脚本文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "SQL 脚本文件 (*.sql)",
				Pattern:     "*.sql",
			},
			{
				DisplayName: "所有文件 (*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("打开保存对话框失败: %w", err)
	}

	// 用户点击了取消
	if filePath == "" {
		return "", nil
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return filePath, nil
}
