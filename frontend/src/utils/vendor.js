/**
 * 数据库方言默认配置与展示文案辅助函数
 */

export const VENDOR_DEFAULTS = {
  mysql: { port: 3306, schema: "", charset: "utf8mb4" },
  postgres: { port: 5432, schema: "public", charset: "" },
  sqlserver: { port: 1433, schema: "dbo", charset: "" },
};

// 获取指定方言的默认 Schema 名称
export function getDefaultSchema(vendor) {
  return (VENDOR_DEFAULTS[vendor] || VENDOR_DEFAULTS.mysql).schema;
}

// 根据内置方言列表解析可读厂商名称
export function getVendorLabel(vendors, vendor) {
  const target = vendor || "mysql";
  const matched = Array.isArray(vendors)
    ? vendors.find((item) => item.vendor === target)
    : null;
  if (matched) return matched.display_name;
  if (target === "postgres") return "PostgreSQL";
  if (target === "sqlserver") return "SQL Server";
  return "MySQL";
}
