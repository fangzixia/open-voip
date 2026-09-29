# Switch 独立部署与 integrator 登记

Switch 配置**不含**业务租户。任意业务系统（含 open-call）在自身配置中声明 `switch_base_url`、`application_id`、`events_callback_url`，并通过登记 API 获取 Switch 签发的 `secret`。

## 1. 启动 Switch

使用 [open-switch/deploy/config.example.yml](../open-switch/deploy/config.example.yml)，配置 `integration.register_token` 与数据库等，**不要**配置 `applications[]`。

## 2. 登记 integrator

```bash
curl -sS -X POST "http://127.0.0.1:8082/switch/v2/integrations/register" \
  -H "Content-Type: application/json" \
  -H "X-Register-Token: <register_token>" \
  -d '{
    "application_id": "cc-prod",
    "events_callback_url": "http://127.0.0.1:8080/api/v1/integration/switch/events",
    "event_retention_days": 14,
    "max_concurrent_calls": 100
  }'
```

新建响应 `data.secret` **只出现一次**，写入 open-call `integration.secret`。

已存在租户再次 register 返回 `200` 且**不含** secret，可更新 callback URL 与配额。

## 3. 配置 open-call 并启动

见 [open-call/deploy/config.example.yml](../open-call/deploy/config.example.yml)。`events_callback_url` 须与登记一致。

## 4. 事件通道

Switch 在 `os_call_events` 落库后 **POST** 到 `events_callback_url`（`Authorization: Bearer <secret>`）。open-call 投影话单/录音并推送 WebSocket。**不**再默认轮询 `/events`；`GET /events` 仅用于对账。

## 5. 轮换密钥（可选）

```bash
curl -sS -X POST "http://127.0.0.1:8082/switch/v2/integrations/cc-prod/rotate-secret" \
  -H "X-Register-Token: <register_token>"
```

更新 open-call YAML 中的 `integration.secret` 后重启。
