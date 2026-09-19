/**
 * 访问令牌本地持久化。三端使用不同 key，避免同一浏览器串用 JWT。
 */

function appScope() {
  try {
    const p = String(location.pathname || "").replace(/\\/g, "/");
    if (p.includes("/admin")) return "admin";
    if (p.includes("/guest")) return "guest";
  } catch {
    /* ignore */
  }
  return "agent";
}

const STORAGE_KEY = `open_voip.access_token.${appScope()}`;

export function getAccessToken() {
  try {
    return sessionStorage.getItem(STORAGE_KEY) || localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

/** @param {string|null} token */
export function setAccessToken(token, { persist = false } = {}) {
  try {
    sessionStorage.removeItem(STORAGE_KEY);
    localStorage.removeItem(STORAGE_KEY);
    if (!token) return;
    if (persist) {
      localStorage.setItem(STORAGE_KEY, token);
    } else {
      sessionStorage.setItem(STORAGE_KEY, token);
    }
  } catch {
    /* 隐私模式等 */
  }
}

export function clearAccessToken() {
  setAccessToken(null);
}
