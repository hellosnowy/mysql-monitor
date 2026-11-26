package main

import (
	"context"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"sql-monitor/internal/dialect/builtin"
	"sql-monitor/internal/service"
	"sql-monitor/internal/storage"
)

// TestAppServiceLifecycle 验证 App 在无 GUI 运行时环境下仍可正常初始化并调用基础业务方法
func TestAppServiceLifecycle(t *testing.T) {
	store, err := storage.NewFileStorage(t.TempDir())
	if err != nil {
		t.Fatalf("初始化临时存储失败: %v", err)
	}

	registry := builtin.NewRegistry()
	app := &App{
		service:  service.NewMonitorService(store, registry),
		registry: registry,
	}

	// 验证 Wails v3 ServiceStartup 生命周期钩子
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := app.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup 返回异常: %v", err)
	}
	if app.ctx != ctx {
		t.Fatalf("ServiceStartup 未保存应用上下文")
	}

	// 验证内置方言列表
	vendors := app.GetVendors()
	if len(vendors) == 0 {
		t.Fatalf("GetVendors 返回空列表")
	}

	// 验证空数据目录读取连接列表
	conns, err := app.GetConnections()
	if err != nil {
		t.Fatalf("GetConnections 返回异常: %v", err)
	}
	if len(conns) != 0 {
		t.Fatalf("期望空连接列表，实际数量: %d", len(conns))
	}

	// 验证未注入 Wails 应用时导出方法返回明确错误而不是 panic
	if _, err := app.ExportSQLFile("test.sql", "SELECT 1;"); err == nil {
		t.Fatalf("期望未初始化 Wails 应用时 ExportSQLFile 返回错误")
	}
}
