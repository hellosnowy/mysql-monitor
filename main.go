package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	// 初始化后端核心桥接服务
	appService, err := NewApp()
	if err != nil {
		log.Fatalf("初始化应用服务失败: %v", err)
	}

	// 创建 Wails v3 桌面应用实例
	wailsApp := application.New(application.Options{
		Name:        "SQL Monitor",
		Description: "Multi-database (MySQL, PostgreSQL, SQL Server) DDL and DML Monitor and Diff Tool",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	appService.setApplication(wailsApp)

	// 创建主应用窗口（保持与 v2 一致的尺寸、背景色与 Windows Mica 效果）
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "SQL Monitor - 多数据库 DDL & DML 监控比对工具",
		Width:            1280,
		Height:           820,
		MinWidth:         1024,
		MinHeight:        680,
		BackgroundType:   application.BackgroundTypeSolid,
		BackgroundColour: application.NewRGB(248, 250, 252),
		URL:              "/",
		Windows: application.WindowsWindow{
			BackdropType: application.Mica,
		},
	})

	// 运行 Wails 原生桌面应用
	if err := wailsApp.Run(); err != nil {
		log.Fatalf("启动桌面应用失败: %v", err)
	}
}
