package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"sql-monitor/internal/dialect"
	"sql-monitor/internal/dialect/builtin"
	"sql-monitor/internal/model"
	"sql-monitor/internal/service"
	"sql-monitor/internal/storage"
)

// App 结构体作为 Wails 应用的核心桥接层，其中的公共方法将自动暴露给前端 JS 调用
type App struct {
	ctx      context.Context
	wailsApp *application.App
	service  *service.MonitorService
	registry *dialect.Registry
}

// NewApp 创建新的 App 实例并初始化存储与服务层
func NewApp() (*App, error) {
	// 数据目录保持 ./data，兼容 mysql-monitor 老版本配置与快照
	store, err := storage.NewFileStorage("./data")
	if err != nil {
		return nil, fmt.Errorf("初始化存储失败: %w", err)
	}

	registry := builtin.NewRegistry()
	srv := service.NewMonitorService(store, registry)
	return &App{
		service:  srv,
		registry: registry,
	}, nil
}

// setApplication 注入 Wails v3 应用实例，供原生对话框等能力使用
func (a *App) setApplication(wailsApp *application.App) {
	a.wailsApp = wailsApp
}

// GetVendors 获取所有内置数据库方言与驱动信息
func (a *App) GetVendors() []dialect.DriverInfo {
	return a.service.Vendors()
}

// ListSchemas 列出指定连接可用的数据库模式
func (a *App) ListSchemas(cfg model.ConnectionConfig) ([]string, error) {
	return a.service.ListSchemas(cfg)
}

// ServiceStartup 在 Wails v3 服务启动阶段由框架回调，保存应用生命周期上下文
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx
	return nil
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
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.service.CaptureSnapshot(ctx, connID, forceBaseline, description)
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
// direction: upgrade（起始→目标）或 rollback（目标→起始）
func (a *App) CompareVersions(connID, fromVersionID, toVersionID, direction string) (*model.DiffResult, error) {
	return a.service.CompareVersions(connID, fromVersionID, toVersionID, model.DiffDirection(direction))
}

// ExportSQLFile 弹出系统原生文件保存对话框并导出 SQL 文件到用户指定路径
func (a *App) ExportSQLFile(defaultName string, content string) (string, error) {
	wailsApp := a.wailsApp
	if wailsApp == nil {
		wailsApp = application.Get()
	}
	if wailsApp == nil {
		return "", fmt.Errorf("Wails 应用尚未初始化")
	}

	dialog := wailsApp.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:                "导出 SQL 脚本文件",
		Filename:             defaultName,
		CanCreateDirectories: true,
		Filters: []application.FileFilter{
			{
				DisplayName: "SQL 脚本文件 (*.sql)",
				Pattern:     "*.sql",
			},
			{
				DisplayName: "所有文件 (*.*)",
				Pattern:     "*.*",
			},
		},
		Window: wailsApp.Window.Current(),
	})

	filePath, err := dialog.PromptForSingleSelection()
	if err != nil {
		// Windows Common File Dialog 在用户取消时会返回 "cancelled by user"
		if strings.Contains(strings.ToLower(err.Error()), "cancel") {
			return "", nil
		}
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
