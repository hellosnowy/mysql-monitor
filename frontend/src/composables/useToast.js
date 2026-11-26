import { reactive } from "vue";

/**
 * 全局单例 Toast 提示状态管理
 */

const toast = reactive({
  visible: false,
  message: "",
  type: "info",
});

let hideTimer = null;

// 展示全局消息提示并在 3 秒后自动收起
export function showToast(message, type = "info") {
  if (hideTimer) {
    clearTimeout(hideTimer);
  }
  toast.message = message;
  toast.type = type;
  toast.visible = true;
  hideTimer = setTimeout(() => {
    toast.visible = false;
    hideTimer = null;
  }, 3000);
}

// 返回响应式的 Toast 状态对象与触发方法
export function useToast() {
  return {
    toast,
    showToast,
  };
}
