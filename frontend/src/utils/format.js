/**
 * 前端通用格式化与文本解析工具函数
 */

// 统一提取错误文本，兼容 Wails v3 RuntimeError 对象
export function formatError(err) {
  if (err && typeof err === "object" && "message" in err && err.message) {
    return err.message;
  }
  return String(err);
}

// 格式化 ISO 时间字符串为本地 24 小时制日期时间
export function formatDate(dateStr) {
  if (!dateStr) return "-";
  return new Date(dateStr).toLocaleString("zh-CN", { hour12: false });
}

// 解析按换行或英文逗号分隔的表名过滤规则列表
export function parseTableList(str) {
  if (!str) return [];
  return str
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}
