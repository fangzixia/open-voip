# ECS PSTN 外呼配置与验收

## 架构要点

- 坐席 WebRTC 外呼：`POST /api/v1/calls/outbound` → open-switch `OriginateSIP`。
- **路线 A（中继）**：`sip.trunks[0]`，`trunk_id` 可空时选第一条中继。
- **路线 B（网关模组）**：配置 `sip.gateway_device`（如 `100163800`），模组已向 ECS `REGISTER` 时，PSTN 外呼会向模组 Contact 发 INVITE（日志 `SIP 网关外呼`）。无需运营商 SIP host；`trunks` 可清空以免错误 REGISTER。
- `sip.devices` 中的账号供 **LuatOS 等终端注册到本机**。

## SSH

```bash
ssh open-voip-ecs   # ~/.ssh/config → 8.153.98.134
```

## 网关模组外呼（路线 B）

在 `/opt/open-switch/config.yml` 的 `sip` 段增加（并确保模组在线）：

```yaml
gateway_device: "100163800"
trunks: []
```

重启后坐席外呼手机号，查看：

```bash
journalctl -u open-switch -f | grep -E '网关外呼|INVITE 出局'
```

也可在 API 显式传 `trunk_id: "@gateway"`。

## 配置运营商中继（路线 A）

1. 将运营商提供的 **SIP 服务器 host**（非 ECS 公网 IP）写入：

```bash
echo 'carrier.sip.example.com' | sudo tee /opt/open-switch/sip-trunk-host
```

2. 合并 trunk（密码默认读取 `sip.devices` 中同用户名的 `password`）：

```bash
sudo python3 /opt/open-voip/src/scripts/deploy/patch_open_switch_trunk.py \
  --port 5060 --from-user 19159001193
sudo systemctl restart open-switch
```

3. 验收注册：

```bash
journalctl -u open-switch -n 50 | grep 'SIP REGISTER'
# 期望：SIP REGISTER 成功 trunk=default
```

## 探测候选 host（可选）

```bash
python3 /opt/open-voip/src/scripts/deploy/sip_trunk_probe.py host1:5060 host2:8910
```

账号 `100163800` 在合宙演示服务器 `180.152.6.34:8910` 会返回 **403**（日志：`SIP REGISTER 被拒 trunk=default status=403`），说明该地址不是本账号的运营商平台。请将 `/opt/open-switch/sip-trunk-host` 改为工单提供的 host 后重新执行 `patch_open_switch_trunk.py`。

## 日志排查

```bash
journalctl -u open-switch --since "1 hour ago" | grep -iE 'outbound|INVITE 出局|未找到|SIP_DISABLED|REGISTER'
tail -100 /opt/open-switch/logs/open-switch/*.log
```

| 现象 | 处理 |
|------|------|
| `未找到 SIP 中继或已注册分机` | 配置 `sip.trunks` 或 `gateway_device` |
| `SIP 网关模组未注册` | 模组 REGISTER 到 ECS，且 `gateway_device` 与 `devices.username` 一致 |
| `SIP REGISTER 被拒` / `无响应` | 核对 host、端口、账号密码、防火墙 |
| 外呼 `ringing` 但坐席无声 | 确认前端已在外呼 `ringing` 时 `media.start`；open-switch 允许坐席 `JoinWebRTC` |

## 坐席与网络

- 坐席 `terminal_type=webrtc`，权限 `calls.operate`，签入队列。
- 安全组：UDP **5060**、RTP **20002–20100**、WebRTC ICE 端口段（见 `ice.udp_port_min/max`）。

## 功能验收

1. 坐席外呼 11 位手机号 → 状态 `ringing` → `active`。
2. 日志：`SIP INVITE 出局已接通`。
3. 话单 `direction=outbound`。
