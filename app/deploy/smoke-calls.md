# Phase 1 并发 smoke（NFR-04：≥10 路语音）

不替代真实双浏览器通话。用于确认排队、振铃锁与 status 计数。

## 手工

1. 启动 Postgres 与 `open-voip`。
2. 管理端再创建 10 个坐席并绑定语音队列，或复制 10 个浏览器配置文件登录不同坐席。
3. 打开 10 个访客 tab 选择「语音服务」入队。
4. 坐席依次接听（或保持排队观察 `GET /api/v1/status` 的 `active_calls` / `ws_connections`）。
5. 确认无双振铃：同一坐席不会同时两个 `call.ringing`。

## 脚本（仅入队，不建立媒体）

在已登录管理会话或使用种子库的前提下，对 `POST /api/v1/guest/join` 循环 10 次：

```bash
for i in $(seq 1 10); do
  curl -s -X POST http://127.0.0.1:8080/api/v1/guest/join \
    -H "Content-Type: application/json" \
    -d "{\"queue_id\":\"$QUEUE_ID\",\"session_type\":\"audio\"}"
  echo
done
curl -s http://127.0.0.1:8080/api/v1/status
```

`QUEUE_ID` 来自 `GET /api/v1/guest/queues`。若只有一名空闲坐席，其余呼叫应保持 `queued`。

## MicroSIP（本机软电话）

1. `sip.enabled: true`、`local_registrar: true`、`external_ip: "127.0.0.1"`，且已有 DID `8001`。
2. `allowed_cidrs` 含 `127.0.0.1/32`。MicroSIP 账号：服务器为本机 `sip.listen`（示例 `127.0.0.1:50600`），UDP，编解码 PCMU/PCMA，密码为 `sip.registrar_password`（默认 `changeme`）。
3. 坐席 `agent1` 签入「语音服务」并空闲。
4. MicroSIP 拨 `8001` → 坐席振铃 → 接听 → 双方说话 → 任一侧挂断。

## 运营商 SIP 中继（直连）

1. `local_registrar: false`，`external_ip` 为固定公网 IP，`from_user` 为报备 DID。
2. trunk 填运营商 `host/port`、`username/password`，`register: true`（IP 互信可将 register 设 false 但仍须 `allowed_cidrs`）。
3. 管理端 DID 写入真实被叫号码并绑定队列。
4. 防火墙仅对 SBC 放行 SIP UDP 与 `rtp_port_min`–`rtp_port_max`。
5. 坐席外呼 11 位手机号；呼入真实 DID → 队列/IVR → 坐席。
