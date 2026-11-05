// Package builtin 组装所有内置数据库方言插件。
package builtin

import (
	"sql-monitor/internal/dialect"
	"sql-monitor/internal/dialect/mysql"
	"sql-monitor/internal/dialect/postgres"
	"sql-monitor/internal/dialect/sqlserver"
)

// NewRegistry 创建并注册全部内置方言
func NewRegistry() *dialect.Registry {
	r := dialect.NewRegistry()
	r.MustRegister(mysql.New())
	r.MustRegister(postgres.New())
	r.MustRegister(sqlserver.New())
	return r
}
