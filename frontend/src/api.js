/**
 * Wails Go 后端桥接 API 封装模块
 */

// 检查是否处于 Wails 运行环境
const hasWails = () => {
  return typeof window !== 'undefined' && window.go && window.go.main && window.go.main.App;
};

export async function getConnections() {
  if (hasWails()) {
    const res = await window.go.main.App.GetConnections();
    return res || [];
  }
  const local = localStorage.getItem('mock_connections');
  return local ? JSON.parse(local) : [];
}

export async function saveConnection(cfg) {
  if (hasWails()) {
    return await window.go.main.App.SaveConnection(cfg);
  }
  const conns = await getConnections();
  const idx = conns.findIndex(c => c.id === cfg.id);
  if (idx >= 0) {
    conns[idx] = cfg;
  } else {
    conns.push(cfg);
  }
  localStorage.setItem('mock_connections', JSON.stringify(conns));
  return null;
}

export async function deleteConnection(id) {
  if (hasWails()) {
    return await window.go.main.App.DeleteConnection(id);
  }
  const conns = await getConnections();
  const filtered = conns.filter(c => c.id !== id);
  localStorage.setItem('mock_connections', JSON.stringify(filtered));
  return null;
}

export async function testConnection(cfg) {
  if (hasWails()) {
    return await window.go.main.App.TestConnection(cfg);
  }
  // 浏览器 Mock 演示
  return {
    success: true,
    message: "连接成功 (Mock 浏览器模式)",
    server_version: "8.0.32-MySQL Community Server",
    latency_ms: 18,
  };
}

export async function captureSnapshot(connID, forceBaseline, description) {
  if (hasWails()) {
    return await window.go.main.App.CaptureSnapshot(connID, forceBaseline, description);
  }
  const list = await listSnapshots(connID);
  const isBaseline = forceBaseline || list.length === 0;
  const ver = {
    version_id: `v${new Date().toISOString().replace(/[-:T.]/g, '').slice(0, 14)}`,
    connection_id: connID,
    database_name: "testdb",
    is_baseline: isBaseline,
    description: description || (isBaseline ? "初始基准版本 (Baseline)" : "快照"),
    table_count: 5,
    row_count: 120,
    created_at: new Date().toISOString(),
  };
  list.unshift(ver);
  localStorage.setItem(`mock_snapshots_${connID}`, JSON.stringify(list));
  return ver;
}

export async function listSnapshots(connID) {
  if (hasWails()) {
    const res = await window.go.main.App.ListSnapshots(connID);
    return res || [];
  }
  const local = localStorage.getItem(`mock_snapshots_${connID}`);
  return local ? JSON.parse(local) : [];
}

export async function deleteSnapshot(connID, versionID) {
  if (hasWails()) {
    return await window.go.main.App.DeleteSnapshot(connID, versionID);
  }
  const list = await listSnapshots(connID);
  const filtered = list.filter(v => v.version_id !== versionID);
  localStorage.setItem(`mock_snapshots_${connID}`, JSON.stringify(filtered));
  return null;
}

export async function compareVersions(connID, fromVersionID, toVersionID) {
  if (hasWails()) {
    return await window.go.main.App.CompareVersions(connID, fromVersionID, toVersionID);
  }
  // 浏览器 Mock 演示数据
  return {
    connection_id: connID,
    from_version_id: fromVersionID,
    to_version_id: toVersionID,
    compared_at: new Date().toISOString(),
    summary: {
      ddl: {
        added_tables: 1,
        dropped_tables: 0,
        modified_tables: 1,
        added_columns: 1,
        dropped_columns: 1,
        modified_columns: 1,
        added_indexes: 1,
        dropped_indexes: 0,
      },
      dml: {
        insert_count: 3,
        update_count: 2,
        delete_count: 1,
      }
    },
    ddl_script: `-- 表: t_user (ALTER_TABLE)\n--   * 新增字段 email (varchar(100))\n--   * 修改字段 username 定义\nALTER TABLE \`t_user\` ADD COLUMN \`email\` varchar(100) NULL AFTER \`username\`;\nALTER TABLE \`t_user\` MODIFY COLUMN \`username\` varchar(100) NOT NULL;`,
    dml_script: `-- 表: t_user (新增: 3, 更新: 2, 删除: 1)\nDELETE FROM \`t_user\` WHERE \`id\` = 105;\nUPDATE \`t_user\` SET \`status\` = 1 WHERE \`id\` = 101;\nINSERT INTO \`t_user\` (\`id\`, \`username\`, \`status\`) VALUES (108, 'new_user', 1);`,
    full_script: `-- MySQL Monitor 同步脚本 (Mock)\nSTART TRANSACTION;\nALTER TABLE \`t_user\` ADD COLUMN \`email\` varchar(100) NULL;\nCOMMIT;`,
  };
}

export async function exportSQLFile(defaultName, content) {
  if (hasWails()) {
    return await window.go.main.App.ExportSQLFile(defaultName, content);
  }
  // Web 模式下回退为浏览器 Blob 下载
  const blob = new Blob([content], { type: 'text/sql;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.setAttribute('href', url);
  link.setAttribute('download', defaultName);
  link.style.visibility = 'hidden';
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  return defaultName;
}
