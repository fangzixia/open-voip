# Phase 1 并发 smoke（NFR-04：≥10 路语音）

不替代真实双浏览器通话。用于确认排队、振铃锁与 status 计数。

## 手工

1. 启动 PostgreSQL、`open-switch` 与 `open-call`。
2. 管理端再创建 10 个坐席并绑定语音队列，或复制 10 个浏览器配置文件登录不同坐席。
3. 打开 10 个访客 tab 选择「语音服务」入队。
4. 坐席依次接听（或保持排队观察 `GET /api/v1/status` 的 `active_calls` / `ws_connections`）。
5. 确认无双振铃：同一坐席不会同时两个 `call.ringing`。

## 脚本（仅入队，不建立媒体）

生产配置默认禁止公开队列直连。先由已登录员工调用 `POST /api/v1/guest/sessions` 为每路呼叫签发一次性短期令牌，再调用 `POST /api/v1/guest/join`。只有显式演示配置 `public.allow_direct_join: true` 才能直接提交队列 ID。

```bash
for i in $(seq 1 10); do
  curl -s -X POST http://127.0.0.1:8080/api/v1/guest/join \
    -H "Content-Type: application/json" \
    -d "{\"token\":\"$GUEST_TOKEN\",\"session_type\":\"audio\"}"
  echo
done
curl -s http://127.0.0.1:8080/api/v1/status
```

若只有一名空闲坐席，其余呼叫应保持 `queued`。令牌再次使用必须返回冲突或未授权，且不能创建第二路呼叫。

## MicroSIP（本机软电话）

1. 仅在 **open-switch** 配置 `sip.enabled: true`、`local_registrar: true`、`external_ip: "127.0.0.1"`，并在 open-call 建立 DID `8001` 路由。
2. open-switch 的 `allowed_cidrs` 含 `127.0.0.1/32`。MicroSIP 服务器指向 open-switch 的 `sip.listen`；每个分机必须使用 `sip.devices` 中独立的用户名和强密码，系统不再支持共用 `registrar_password`。
3. 坐席 `agent1` 签入「语音服务」并空闲。
4. MicroSIP 拨 `8001` → 坐席振铃 → 接听 → 双方说话 → 任一侧挂断。

## 运营商 SIP 中继（直连）

1. 在 open-switch 设置 `local_registrar: false`，`external_ip` 为固定公网 IP，`from_user` 为报备 DID。
2. trunk 填运营商 `host/port`、`username/password`，`register: true`（IP 互信可将 register 设 false 但仍须 `allowed_cidrs`）。
3. 管理端 DID 写入真实被叫号码并绑定队列。
4. 防火墙仅对 SBC 放行 SIP UDP 与 `rtp_port_min`–`rtp_port_max`。
5. 坐席外呼 11 位手机号；呼入真实 DID → 队列/IVR → 坐席。
