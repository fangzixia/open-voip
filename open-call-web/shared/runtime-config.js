/**
 * 运行时配置：生产由服务端 HTML 注入 window.__OPEN_VOIP__；
 * 开发环境 Vite 使用同源 + proxy，或显式覆盖 apiBase。
 */

const injected = typeof window !== "undefined" ? window.__OPEN_VOIP__ : undefined;

/** @type {string|undefined} */
const viteApiBase = import.meta.env.VITE_API_BASE;

/** @returns {{ apiBase: string, wsPath: string, publicUrl: string }} */
export function getRuntimeConfig() {
  const origin =
    typeof window !== "undefined" && window.location?.origin
      ? window.location.origin
      : "";
  const fromEnv = viteApiBase && String(viteApiBase).trim();
  const apiBase = injected?.apiBase ?? (fromEnv || origin);
  return {
    apiBase: apiBase.replace(/\/$/, ""),
    wsPath: injected?.wsPath ?? "/api/v1/ws",
    publicUrl: injected?.publicUrl ?? origin,
  };
}
