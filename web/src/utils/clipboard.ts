/**
 * 复制文本
 */
export async function copy(text: string): Promise<boolean> {
  // navigator.clipboard 仅在安全上下文（HTTPS）或 localhost 中可用
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (e) {
      console.error("使用 navigator.clipboard 复制失败:", e);
    }
  }

  // 后备方案：使用已弃用但兼容性更好的 execCommand
  try {
    const input = document.createElement("input");
    input.style.position = "fixed";
    input.style.opacity = "0";
    input.value = text;
    document.body.appendChild(input);
    input.select();
    const result = document.execCommand("copy");
    document.body.removeChild(input);

    if (!result) {
      console.error("使用 execCommand 复制失败");
      return false;
    }
    return true;
  } catch (e) {
    console.error("后备复制方法执行出错:", e);
    return false;
  }
}
