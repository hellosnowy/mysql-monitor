import { App, hasWails } from "./client.js";

/**
 * 历史版本差异比对与 SQL 导出接口模块
 */

// 比对两个历史版本并生成 DDL/DML 升级或回滚 SQL 脚本
export async function compareVersions(
  connID,
  fromVersionID,
  toVersionID,
  direction = "upgrade",
) {
  if (hasWails()) {
    return await App.CompareVersions(
      connID,
      fromVersionID,
      toVersionID,
      direction,
    );
  }
  return {
    connection_id: connID,
    vendor: "mysql",
    direction,
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
      },
    },
    ddl_script: `-- 表: t_user (ALTER_TABLE)\n--   * 新增字段 email (varchar(100))\n--   * 修改字段 username 定义\nALTER TABLE \`t_user\` ADD COLUMN \`email\` varchar(100) NULL AFTER \`username\`;\nALTER TABLE \`t_user\` MODIFY COLUMN \`username\` varchar(100) NOT NULL;`,
    dml_script: `-- 表: t_user (新增: 3, 更新: 2, 删除: 1)\nDELETE FROM \`t_user\` WHERE \`id\` = 105;\nUPDATE \`t_user\` SET \`status\` = 1 WHERE \`id\` = 101;\nINSERT INTO \`t_user\` (\`id\`, \`username\`, \`status\`) VALUES (108, 'new_user', 1);`,
    full_script: `-- MySQL Monitor 同步脚本 (Mock)\nSTART TRANSACTION;\nALTER TABLE \`t_user\` ADD COLUMN \`email\` varchar(100) NULL;\nCOMMIT;`,
  };
}

// 导出 SQL 文件（桌面环境弹出系统保存对话框，Web 预览回退为浏览器下载）
export async function exportSQLFile(defaultName, content) {
  if (hasWails()) {
    return await App.ExportSQLFile(defaultName, content);
  }
  const blob = new Blob([content], { type: "text/sql;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.setAttribute("href", url);
  link.setAttribute("download", defaultName);
  link.style.visibility = "hidden";
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
  return defaultName;
}
