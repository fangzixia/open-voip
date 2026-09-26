// 本文件验证http client的关键行为。
import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";

globalThis.window = { location: { origin: "http://localhost:5173" } };
globalThis.location = { pathname: "/admin/" };
const storage = () => { const map = new Map(); return { getItem: k => map.get(k) ?? null, setItem: (k,v) => map.set(k,v), removeItem: k => map.delete(k), clear: () => map.clear() }; };
globalThis.sessionStorage = storage();
globalThis.localStorage = storage();
const { request, apiEvents } = await import("./http-client.js");
const { setCallContext } = await import("./call-context.js");
const { getAccessToken, setAccessToken } = await import("./auth-store.js");
beforeEach(() => { sessionStorage.clear(); localStorage.clear(); });
const response = (data, status = 200) => new Response(JSON.stringify({ code: status < 400 ? "OK" : "UNAUTHORIZED", message: "消息", data, request_id: "r1" }), { status, headers: { "Content-Type": "application/json", "X-Trace-ID": "server-trace" } });

test("unwrap JSON, authenticate, and preserve empty success", async () => {
  setAccessToken("token");
  setCallContext({ trace_id: "trace-1", call_id: "call-1", leg_id: "leg-1" });
  globalThis.fetch = async (url, init) => {
    assert.equal(String(url), "http://localhost:5173/api/v1/users");
    assert.equal(init.headers.get("Authorization"), "Bearer token");
    assert.equal(init.headers.get("X-Trace-ID"), "trace-1");
    assert.match(init.headers.get("X-Request-ID"), /^[0-9a-f-]{36}$/);
    assert.equal(init.headers.get("X-Call-ID"), "call-1");
    assert.equal(init.headers.get("X-Leg-ID"), "leg-1");
    return response({ items: [1] });
  };
  assert.deepEqual(await request("/api/v1/users"), { items: [1] });
  globalThis.fetch = async () => response(null);
  assert.equal(await request("/api/v1/action", { method: "POST" }), null);
});
test("401 expires session once and anonymous login does not expire it", async () => {
  let events = 0;
  const handler = () => events++;
  apiEvents.addEventListener("unauthorized", handler);
  try {
    setAccessToken("token");
    globalThis.fetch = async () => response(null, 401);
    await assert.rejects(request("/login", { auth: false }), { status: 401 });
    assert.equal(getAccessToken(), "token");
    await assert.rejects(request("/users"), { status: 401, requestId: "r1" });
    assert.equal(getAccessToken(), null);
    assert.equal(events, 1);
  } finally { apiEvents.removeEventListener("unauthorized", handler); }
});
test("old request cannot clear a new session", async () => {
  setAccessToken("old");
  globalThis.fetch = async () => { setAccessToken("new"); return response(null, 401); };
  await assert.rejects(request("/users"));
  assert.equal(getAccessToken(), "new");
});
test("timeouts, cancellation and network errors have stable codes", async () => {
  globalThis.fetch = async (_url, {signal}) => new Promise((_, reject) => {
    const abort = () => reject(new DOMException("aborted", "AbortError"));
    if (signal.aborted) abort(); else signal.addEventListener("abort", abort, {once:true});
  });
  await assert.rejects(request("/slow", {timeoutMs: 5}), { code: "TIMEOUT" });
  const controller = new AbortController(); controller.abort();
  await assert.rejects(request("/cancel", {signal: controller.signal}), { code: "CANCELED" });
  globalThis.fetch = async () => { throw new TypeError("fetch failed"); };
  await assert.rejects(request("/offline"), { code: "NETWORK_ERROR" });
});
test("file downloads share error handling without wrapping bytes", async () => {
  globalThis.fetch = async () => new Response("recording bytes");
  assert.equal(await (await request("/file", {responseType:"blob"})).text(), "recording bytes");
  globalThis.fetch = async () => response(null,403);
  await assert.rejects(request("/file", {responseType:"blob"}), {status:403});
});
test("invalid JSON and business errors are surfaced", async () => {
  globalThis.fetch = async () => new Response("<html>gateway</html>");
  await assert.rejects(request("/users"), {code:"INVALID_RESPONSE"});
  globalThis.fetch = async () => new Response(JSON.stringify({code:"AGENT_BUSY",message:"坐席忙",data:null,request_id:"r2"}));
  await assert.rejects(request("/call"), {code:"AGENT_BUSY",requestId:"r2"});
  await assert.rejects(request("https://other.example/users"), {code:"INVALID_URL"});
});
