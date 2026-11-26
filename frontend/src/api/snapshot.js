import { App, hasWails } from "./client.js";

/**
 * 数据库快照与基准版本接口模块
 */

const mockSnapshotsKey = (connID) => `mock_snapshots_${connID}`;

// 抓取指定数据库的当前结构与数据快照
export async function captureSnapshot(connID, forceBaseline, description) {
  if (hasWails()) {
    return await App.CaptureSnapshot(connID, forceBaseline, description);
  }
  const list = await listSnapshots(connID);
  const isBaseline = forceBaseline || list.length === 0;
  const ver = {
    version_id: `v${new Date()
      .toISOString()
      .replace(/[-:T.]/g, "")
      .slice(0, 14)}`,
    connection_id: connID,
    database_name: "testdb",
    is_baseline: isBaseline,
    description:
      description || (isBaseline ? "初始基准版本 (Baseline)" : "快照"),
    table_count: 5,
    row_count: 120,
    created_at: new Date().toISOString(),
  };
  list.unshift(ver);
  localStorage.setItem(mockSnapshotsKey(connID), JSON.stringify(list));
  return ver;
}

// 获取指定连接的所有历史快照版本
export async function listSnapshots(connID) {
  if (hasWails()) {
    const res = await App.ListSnapshots(connID);
    return res || [];
  }
  const local = localStorage.getItem(mockSnapshotsKey(connID));
  return local ? JSON.parse(local) : [];
}

// 删除指定连接下的某个历史快照版本
export async function deleteSnapshot(connID, versionID) {
  if (hasWails()) {
    return await App.DeleteSnapshot(connID, versionID);
  }
  const list = await listSnapshots(connID);
  const filtered = list.filter((v) => v.version_id !== versionID);
  localStorage.setItem(mockSnapshotsKey(connID), JSON.stringify(filtered));
  return null;
}
