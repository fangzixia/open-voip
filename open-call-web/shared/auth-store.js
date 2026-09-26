/**
 * 访问令牌本地持久化。三端使用不同 key，避免同一浏览器串用 JWT。
 */

function appScope() {
  try {
    const p = String(location.pathname || "").replace(/\\/g, "/");
    if (p.includes("/admin")) return "admin";
    if (p.includes("/guest")) return "guest";
  } catch {
    /* 忽略该异常，继续执行后续操作。 */
  }
  return "agent";
}

const STORAGE_KEY = `open_voip.access_token.${appScope()}`;
const REFRESH_KEY = `open_voip.refresh_token.${appScope()}`;

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
  try { sessionStorage.removeItem(REFRESH_KEY); localStorage.removeItem(REFRESH_KEY); } catch { /* 忽略该异常，继续执行后续操作。 */ }
}

export function getRefreshToken() {
  try { return sessionStorage.getItem(REFRESH_KEY) || localStorage.getItem(REFRESH_KEY); } catch { return null; }
}

export function setAuthTokens(tokens, { persist = false } = {}) {
  setAccessToken(tokens.access_token, { persist });
  try {
    sessionStorage.removeItem(REFRESH_KEY);
    localStorage.removeItem(REFRESH_KEY);
    if (tokens.refresh_token) (persist ? localStorage : sessionStorage).setItem(REFRESH_KEY, tokens.refresh_token);
  } catch { /* 忽略该异常，继续执行后续操作。 */ }
}
