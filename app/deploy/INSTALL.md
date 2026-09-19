# 内网安装（Phase 1～3）

> 对齐 DEPLOY-01～04、PLAT-01。交付：**本机 PostgreSQL + open-voip 二进制 + 独立前端静态站（Nginx）**。不提供 Docker / Compose。

## 1. 目录约定

**API**

```
/opt/open-voip/
  open-voip
  config.yml
  data/recordings/
```

**UI**（与 API 分离）

```
/var/www/open-voip-ui/     # frontend/dist 内容
```

数据：PostgreSQL 数据目录与 `recordings.dir` 落在本机磁盘，按操作系统常规方式备份。

本机需先安装 PostgreSQL 16+，建库账号与 `config.yml` 的 `database.dsn` 一致。

## 2. 防火墙

- **TCP 443**：UI Nginx 与 API（或只对 Nginx 开放 443，API 走内网）
- **UDP 5060**（或 `sip.listen`）：仅对运营商 SBC / 本机软电话；勿对公网全开
- **UDP `sip.rtp_port_min`–`rtp_port_max`**：SIP RTP（与 WebRTC `ice.udp_port_*` 分开）
- **UDP 10000–20000**：open-voip 进程的 WebRTC ICE/RTP（`ice.udp_port_*`）

## 3. systemd（仅 API）

示例：[open-voip.service](./open-voip.service)

```bash
sudo useradd -r -s /usr/sbin/nologin openvoip
sudo mkdir -p /opt/open-voip/data/recordings
sudo cp open-voip config.yml /opt/open-voip/
sudo chown -R openvoip:openvoip /opt/open-voip
sudo systemctl enable --now open-voip
```

`ExecStart` 仅接受 `-config`。API 对任意浏览器 Origin 开放 CORS。UI / API 根地址由 Nginx 与前端 `VITE_API_BASE` 决定，服务端不配置。

## 4. 前端

```bash
cd frontend
cp .env.example .env   # VITE_API_BASE=https://api.cc.internal
npm ci && npm run build
sudo rsync -a dist/ /var/www/open-voip-ui/
```

Nginx：[nginx-frontend.example.conf](./nginx-frontend.example.conf)（UI）、[nginx-tls.example.conf](./nginx-tls.example.conf)（API 反代）。

## 5. HTTPS 与浏览器权限（PLAT-01）

getUserMedia 需要 `https://` 或 `http://localhost`。

### 5.1 自签证书

```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 825 \
  -keyout server.key -out server.crt \
  -subj "/CN=ui.cc.internal" \
  -addext "subjectAltName=DNS:ui.cc.internal,DNS:api.cc.internal,IP:192.168.1.10"
```

将 `server.crt` 导入坐席浏览器信任库（否则麦克风/摄像头权限会被拦）。也可由企业 CA 签发 UI 与 API 两张证。

**Windows + Chrome / Edge（企业 CA 或自签）**

1. 双击 `server.crt`（或企业根 CA 证书）→「安装证书」。
2. 选择「本地计算机」→「将所有的证书都放入下列存储」→「受信任的根证书颁发机构」。
3. 重启浏览器，用 `https://ui.cc.internal` 打开坐席页；地址栏应显示锁，无证书警告。
4. 首次 getUserMedia 时允许麦克风/摄像头。

**Firefox**：设置 → 隐私与安全 → 证书 → 查看证书 → 证书机构 → 导入同一 `server.crt`，勾选「信任此 CA 标识网站」。

内网 IP 访问时，证书 SAN 必须包含该 IP；仅 CN 不够。localhost 可用 `http://127.0.0.1` 免证书。

### 5.2 API 进程内 TLS

`tls.enabled: true` 时 `server.listen` 可直接 443；更常见是 Nginx 终结 TLS 后反代到 `127.0.0.1:8080`。

## 6. 空库种子

| 用户 | 角色 |
|------|------|
| admin / changeme | 管理员 |
| agent1 / changeme | 坐席 1001（视频能力） |
| agent2 / changeme | 坐席 1002（视频能力，便于转接/分机互拨） |
| supervisor / changeme | 班长 1099（可监听、强制签出） |

队列：「语音服务」「视频服务」。生产请立即改密。

## 7. 备份（DEPLOY-06）

- `pg_dump` 数据库
- rsync `recordings.dir`
- 管理端 `GET /api/v1/config/export` 导出用户/队列/IVR/DID（不含密码哈希还原）

## 8. 硬件与容量（DEPLOY-05 / NFR-02）

建议最低：4 核 CPU、8 GB 内存、100 GB 磁盘（含录音）。

录音估算：GB/天 ≈ 并发路数 × 码率(Mbps) × 86400 / 8 / 1024。10 路语音 Opus ~32 kbps 约 3.4 GB/天；5 路 720p 另加大约 50～100 GB/天（若录像）。

5 路 720p + 10 路语音在千兆内网通常可行；视频 CPU 主要消耗在浏览器编码。服务端录制写 Ogg/IVF，本机若安装 ffmpeg 会在通结束时尝试封装 WebM。

IVR/排队提示音：将 **PCM WAV**（建议 8 kHz 或 16 kHz 16-bit 单声道）放到 `recordings.dir/prompts/`，IVR 节点 `file` 填文件名。无文件时播放短提示音。不内置 TTS。

## 9. TURN（MEDIA-04b / VIDEO-12）

同网段可关闭。跨 VLAN / 对称 NAT 时同机部署 coturn，`turn.enabled: true`，`urls` 填 `turn:cc.internal:3478`，`auth_secret` 与 coturn `static-auth-secret` 一致。服务端签发短期 HMAC 凭证。

## 10. SIP / PSTN（MEDIA-07/08，可选）

进程内 **sipgo** UA（默认 UDP）。`sip.enabled: true` 时须配置 `sip.external_ip`、`sip.rtp_port_min/max` 与至少一条 `sip.trunks`。防火墙仅对运营商 SBC 开放 UDP `sip.listen` 与 RTP 段；家宽请关闭 SIP ALG。

### 10.1 本机 MicroSIP 演示

1. `local_registrar: true`，`external_ip: "127.0.0.1"`，`allowed_cidrs` 含 `127.0.0.1/32`。
2. 种子 DID **8001** 路由到「语音服务」。
3. MicroSIP：服务器为本机 `sip.listen`（示例常为 `127.0.0.1:50600`），UDP，PCMU/PCMA；账号与密码用 `registrar_password`（默认 `changeme`），realm 为本机 `sip.local_domain` 或 `external_ip`。REGISTER 走 Digest（RFC 3261 §22）。
4. 坐席签入语音队列后拨 `8001`；呼入走队列 IVR（若队列已绑定）。
5. 公网切中继前必须把 `local_registrar` 设为 `false`，否则 5060 会接受软电话 REGISTER。

### 10.2 直连运营商 SIP 中继

1. 向运营商/云通信申请 SIP Trunk（固定公网 IP + Digest 账号，或 IP 互信）。
2. `external_ip` 填该公网地址；`from_user` 填已报备外显 DID。
3. `register: true` 时填写 `username`/`password`，进程会 REGISTER 并按 `expire_sec` 刷新；`401/407` 自动 Digest。
4. `allowed_cidrs` 填对端 SBC IP；入站 INVITE 不在名单则 `403`。
5. 管理端把真实号码写入 DID 路由（精确匹配失败时会尝试去掉 `+`/`00`/`86`/区号前缀）。
6. 出局 `From` 使用 `from_user`；`P-Asserted-Identity` 仅发往受信任中继（RFC 3325），本机分机不带 PAI。号码按 `strip_prefix`/`prefix` 规范化。
7. SDP **offer** 可列 PCMU+PCMA；**answer** 按 RFC 3264 只回一个 G.711 PT。浏览器侧仍为 PCMU，PCMA 在 SIP RTP 边查表转码。
8. 出局跟随 **3xx** Contact（最多 2 跳）。坐席未接听回 **480**（不是 408）。未实现方法回 **405** 且带 `Allow`。
9. `session_expires_sec ≥ 90` 时发送 `Session-Expires;refresher=uac`，协商后由 refresher 发 UPDATE（失败则 re-INVITE）。对端 `Require: 100rel` 时发送 **PRACK**（RFC 3262）。`transport: tls` 需证书。

未启用则外呼 PSTN 返回 `SIP_DISABLED`；分机互拨不依赖 SIP。

## 11. 录音清理（REC-05）

无内置 cron。由外部定时调用 `POST /api/v1/admin/recordings/purge-expired`（按 `recordings.retain_days`）。

## 12. 离线安装（DEPLOY-07）

在可联网机构建：

```bash
cd app && go build -o open-voip ./cmd/open-voip
cd ../frontend && npm ci && npm run build
```

将 `open-voip`、`deploy/config.yml`、`frontend/dist/` 与 PostgreSQL 安装包拷贝到内网机，按本文第 1～4 节安装，无需访问公网。

## 13. 浏览器（NFR-06）

Chrome / Edge 最新版。Safari 视频建议 H.264（服务端已注册）；若协商失败可降级语音。

