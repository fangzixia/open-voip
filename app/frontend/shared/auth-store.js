/**
 * 访问令牌本地持久化，三端共用同一 key 约定，避免各 app 重复实现。
 */

const STORAGE_KEY = "open_voip.access_token";

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
