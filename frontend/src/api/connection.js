import { App, hasWails } from "./client.js";

/**
 * 数据库连接管理接口模块
 */

const MOCK_CONNECTIONS_KEY = "mock_connections";

// 获取全部已保存的数据库连接配置
export async function getConnections() {
  if (hasWails()) {
    const res = await App.GetConnections();
    return res || [];
  }
  const local = localStorage.getItem(MOCK_CONNECTIONS_KEY);
  return local ? JSON.parse(local) : [];
}

// 新增或更新数据库连接配置
export async function saveConnection(cfg) {
  if (hasWails()) {
    return await App.SaveConnection(cfg);
  }
  const conns = await getConnections();
  const idx = conns.findIndex((c) => c.id === cfg.id);
  if (idx >= 0) {
    conns[idx] = cfg;
  } else {
    conns.push(cfg);
  }
  localStorage.setItem(MOCK_CONNECTIONS_KEY, JSON.stringify(conns));
  return null;
}

// 删除指定 ID 的数据库连接配置
export async function deleteConnection(id) {
  if (hasWails()) {
    return await App.DeleteConnection(id);
  }
  const conns = await getConnections();
  const filtered = conns.filter((c) => c.id !== id);
  localStorage.setItem(MOCK_CONNECTIONS_KEY, JSON.stringify(filtered));
  return null;
}

// 测试数据库连接连通性并返回版本与延迟
export async function testConnection(cfg) {
  if (hasWails()) {
    return await App.TestConnection(cfg);
  }
  const versionMap = {
    mysql: "8.0.32-MySQL Community Server",
    postgres: "PostgreSQL 16.3",
    sqlserver: "Microsoft SQL Server 2022",
  };
  return {
    success: true,
    message: "连接成功 (Mock 浏览器模式)",
    server_version: versionMap[cfg && cfg.vendor] || versionMap.mysql,
    latency_ms: 18,
  };
}
