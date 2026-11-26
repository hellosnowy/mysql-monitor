import * as App from "../../bindings/sql-monitor/app.js";

/**
 * Wails v3 桌面客户端绑定与运行环境检测模块
 */

// 判断当前是否处于 Wails v3 桌面运行环境（避免浏览器 Mock 预览误判）
export function hasWails() {
  if (typeof window === "undefined") return false;
  return Boolean(
    window._wails?.environment?.OS ||
      window.chrome?.webview?.postMessage ||
      window.webkit?.messageHandlers?.external?.postMessage,
  );
}

export { App };
