import { App, hasWails } from "./client.js";

/**
 * 数据库方言驱动与 Schema 元数据接口模块
 */

// 获取所有内置数据库方言与驱动信息
export async function getVendors() {
  if (hasWails()) {
    return (await App.GetVendors()) || [];
  }
  return [
    {
      vendor: "mysql",
      display_name: "MySQL",
      driver_package: "github.com/go-sql-driver/mysql",
      driver_version: "v1.8.1",
      built_in: true,
      default_port: 3306,
      default_schema: "",
      supports_schemas: false,
    },
    {
      vendor: "postgres",
      display_name: "PostgreSQL",
      driver_package: "github.com/jackc/pgx/v5",
      driver_version: "v5.12.0",
      built_in: true,
      default_port: 5432,
      default_schema: "public",
      supports_schemas: true,
    },
    {
      vendor: "sqlserver",
      display_name: "SQL Server",
      driver_package: "github.com/microsoft/go-mssqldb",
      driver_version: "v1.11.2",
      built_in: true,
      default_port: 1433,
      default_schema: "dbo",
      supports_schemas: true,
    },
  ];
}

// 列出指定连接下可用的数据库模式（Schema）
export async function listSchemas(cfg) {
  if (hasWails()) {
    return (await App.ListSchemas(cfg)) || [];
  }
  if (cfg && cfg.vendor === "postgres") return ["public", "app"];
  if (cfg && cfg.vendor === "sqlserver") return ["dbo", "app"];
  return ["testdb"];
}
