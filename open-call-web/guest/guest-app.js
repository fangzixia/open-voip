import { LitElement, html } from "lit";
import { guestJoin, guestJoinToken, hangupCall, listGuestQueues, respondVideo, sendDtmf } from "../shared/api.js";
import { setAccessToken, clearAccessToken } from "../shared/auth-store.js";
import { shellStyles } from "../shared/shell-styles.js";
import { setLocalMuted, startMediaSession, stopMedia } from "../shared/webrtc.js";
import { BusinessWebSocket } from "../shared/ws.js";

const NAV = [
  { id: "service", label: "选择服务" },
  { id: "queue", label: "排队状态" },
  { id: "talk", label: "当前通话" },
  { id: "list", label: "可进入队列" },
];

function fmtMMSS(sec) {
  const s = Math.max(0, Number(sec) || 0);
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
}

export class GuestApp extends LitElement {
  static properties = {
    error: { type: String },
    queues: { type: Array },
    step: { type: String },
    permissionHint: { type: String },
    join: { type: Object },
    elapsed: { type: Number },
    audioMuted: { type: Boolean },
    videoMuted: { type: Boolean },
    wantVideo: { type: Boolean },
    videoAsk: { type: Object },
    notice: { type: String },
    selected: { type: Object },
    waitSec: { type: Number },
    position: { type: Number },
    nav: { type: String },
  };

  static styles = shellStyles;

  #ws = new BusinessWebSocket();
  #pc = null;
  #local = null;
  #remote = null;
  #timer = null;
  #started = 0;
  #waitTimer = null;

  constructor() {
    super();
    this.error = "";
    this.queues = [];
    this.step = "pick";
    this.permissionHint = "";
    this.join = null;
    this.elapsed = 0;
    this.audioMuted = false;
    this.videoMuted = false;
    this.wantVideo = false;
    this.videoAsk = null;
    this.notice = "";
    this.selected = null;
    this.waitSec = 0;
    this.position = 0;
    this.nav = "service";
  }

  connectedCallback() {
    super.connectedCallback();
    const token = new URLSearchParams(location.search).get("token");
    listGuestQueues()
      .then((r) => {
        this.queues = r.items || [];
        if (this.queues[0]) this.selected = { queue: this.queues[0], video: false, vip: false };
        if (token) this.#joinToken(token);
      })
      .catch((e) => {
        this.error = e instanceof Error ? e.message : String(e);
      });
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.#cleanup();
  }

  #pick(queue, video, vip = false) {
    this.selected = { queue, video, vip };
  }

  async #start(queue, video, vip = false) {
    this.wantVideo = video;
    this.permissionHint = video
      ? "请允许浏览器使用麦克风和摄像头。若拒绝，可改选语音服务。"
      : "请允许浏览器使用麦克风。";
    if (video && navigator.connection && navigator.connection.downlink && navigator.connection.downlink < 1.5) {
      this.permissionHint += " 当前网速可能较弱，建议改用语音。";
    }
    this.step = "perm";
    this.nav = "queue";
    this.waitSec = 0;
    this.#waitTimer = setInterval(() => {
      this.waitSec += 1;
    }, 1000);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video });
      stream.getTracks().forEach((t) => t.stop());
    } catch {
      this.error = video ? "摄像头/麦克风权限被拒绝，可改选语音服务。" : "麦克风权限被拒绝，请在浏览器设置中允许后重试。";
      this.step = "pick";
      this.nav = "service";
      clearInterval(this.#waitTimer);
      return;
    }
    try {
      const join = await guestJoin(queue.id, video ? "video" : "audio", vip ? 10 : 0);
      this.join = join;
      setAccessToken(join.token);
      this.step = "wait";
      this.#ws.connect(join.token);
      this.#ws.subscribe((msg) => this.#onWs(msg));
      if (join.state === "ivr") await this.#enterMedia(false);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      this.step = "pick";
      this.nav = "service";
      clearInterval(this.#waitTimer);
    }
  }

  async #onWs(msg) {
    if (msg.type === "call.answered") {
      await this.#enterMedia(true);
    } else if (msg.type === "call.ended") {
      this.#cleanup();
      this.step = "ended";
      this.nav = "talk";
    } else if (msg.type === "queue.position") {
      this.permissionHint = msg.payload?.message || "正在排队…";
      this.position = msg.payload?.position ?? this.position;
    } else if (msg.type === "call.voicemail") {
      this.permissionHint = msg.payload?.message || "请留言";
      if (!this.#pc) await this.#enterMedia(false);
    } else if (msg.type === "recording.notice") {
      this.notice = msg.payload?.message || "";
    } else if (msg.type === "video.requested") {
      this.videoAsk = msg.payload;
    } else if (msg.type === "video.accepted") {
      this.wantVideo = true;
      await this.#rejoin(true);
    } else if (msg.type === "video.downgraded") {
      this.wantVideo = false;
      await this.#rejoin(false);
    } else if (msg.type === "ivr.started" || msg.type === "ivr.prompt") {
      this.permissionHint = msg.payload?.prompt || "IVR 放音中，请按键";
      if (!this.#pc) await this.#enterMedia(false);
    }
  }

  async #joinToken(token) {
    this.step = "wait";
    this.nav = "queue";
    try {
      const join = await guestJoinToken(token, "audio");
      this.join = join;
      setAccessToken(join.token);
      this.#ws.connect(join.token);
      this.#ws.subscribe((msg) => this.#onWs(msg));
      if (join.state === "ivr") await this.#enterMedia(false);
      if (join.state === "active") await this.#enterMedia(true);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      this.step = "pick";
      this.nav = "service";
    }
  }

  async #enterMedia(talking) {
    try {
      if (!this.#pc) {
        const session = await startMediaSession({
          callId: this.join.call_id,
          legId: this.join.leg_id,
          video: this.wantVideo,
        });
        this.#pc = session.pc;
        this.#local = session.localStream;
        this.#remote = session.remoteStream;
      }
      if (!talking) return;
      this.step = "talk";
      this.nav = "talk";
      if (!this.#timer) {
        this.#started = Date.now();
        this.#timer = setInterval(() => {
          this.elapsed = Math.floor((Date.now() - this.#started) / 1000);
        }, 500);
      }
      await this.updateComplete;
      const localV = this.renderRoot.querySelector("#local");
      const remoteV = this.renderRoot.querySelector("#remote");
      if (localV) localV.srcObject = this.#local;
      if (remoteV) remoteV.srcObject = this.#remote;
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #rejoin(video) {
    this.wantVideo = video;
    stopMedia(this.#pc, this.#local);
    this.#pc = null;
    this.#local = null;
    this.#remote = null;
    this.step = "talk";
    await this.#enterMedia(true);
  }

  async #dtmf(d) {
    if (!this.join?.call_id || !this.join?.leg_id) return;
    try {
      await sendDtmf(this.join.call_id, this.join.leg_id, d);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #hangup() {
    try {
      await hangupCall(this.join.call_id);
    } catch {
      /* ignore */
    }
    this.#cleanup();
    this.step = "ended";
    this.nav = "talk";
  }

  async #toggle(kind) {
    if (kind === "audio") this.audioMuted = !this.audioMuted;
    else this.videoMuted = !this.videoMuted;
    await setLocalMuted(this.#local, this.join.call_id, this.join.leg_id, {
      audio: this.audioMuted,
      video: this.videoMuted,
    });
  }

  #cleanup() {
    clearInterval(this.#timer);
    clearInterval(this.#waitTimer);
    this.#ws.disconnect();
    stopMedia(this.#pc, this.#local);
    this.#pc = null;
    this.#local = null;
    this.#remote = null;
    clearAccessToken();
  }

  #crumb() {
    return NAV.find((n) => n.id === this.nav)?.label || "选择服务";
  }

  #go(id) {
    this.nav = id;
    if (id === "talk" && this.step === "talk") {
      this.updateComplete.then(() => {
        const localV = this.renderRoot.querySelector("#local");
        const remoteV = this.renderRoot.querySelector("#remote");
        if (localV) localV.srcObject = this.#local;
        if (remoteV) remoteV.srcObject = this.#remote;
      });
    }
  }

  render() {
    const waiting = this.step === "perm" || this.step === "wait";
    return html`
      <div class="layout">
        <header class="topbar">
          <div class="brand">
            <span class="brand-mark">OV</span>
            <span>Open VoIP</span>
            <span class="brand-sub">在线客服</span>
          </div>
        </header>
        <div class="layout-body">
          <aside class="sidebar">
            ${NAV.map(
              (n) => html`<button class="nav-item ${this.nav === n.id ? "active" : ""}" @click=${() => this.#go(n.id)}>
                <span>${n.label}</span>
                ${n.id === "queue" && waiting ? html`<span class="nav-badge">1</span>` : ""}
              </button>`,
            )}
          </aside>
          <div class="content">
            <div class="breadcrumb">在线客服 / <strong>${this.#crumb()}</strong></div>
            ${this.error ? html`<p class="error">${this.error}</p>` : ""}
            ${this.nav === "service" ? (this.step === "ended" ? this.#endedView() : this.#pickView()) : ""}
            ${this.nav === "queue" ? (waiting ? this.#waitView() : this.#queueIdleView()) : ""}
            ${this.nav === "talk" ? (this.step === "talk" ? this.#talkView() : this.step === "ended" ? this.#endedView() : html`<div class="panel"><p class="muted">当前没有通话。请先在「选择服务」发起。</p></div>`) : ""}
            ${this.nav === "list" ? this.#listView() : ""}
          </div>
        </div>
      </div>
    `;
  }

  #pickView() {
    const sel = this.selected;
    return html`
      <div class="panel">
        <h3 class="page-title">选择服务</h3>
        <div class="svc-grid">
          ${this.queues
            .filter((q) => !q.video_enabled)
            .map((q) => {
              const on = sel?.queue?.id === q.id && !sel.video;
              return html`<div class="svc-card ${on ? "selected" : ""}" @click=${() => this.#pick(q, false, false)}>
                <h3>${q.name}</h3>
                <p class="muted">麦克风通话 · 平均等待约 30 秒</p>
                <button @click=${(e) => { e.stopPropagation(); this.#start(q, false); }}>开始通话</button>
              </div>`;
            })}
          ${this.queues
            .filter((q) => q.video_enabled)
            .map((q) => {
              const on = sel?.queue?.id === q.id && sel.video;
              return html`<div class="svc-card ${on ? "selected" : ""}" @click=${() => this.#pick(q, true, false)}>
                <h3>${q.name}</h3>
                <p class="muted">需摄像头</p>
                <button class="secondary" @click=${(e) => { e.stopPropagation(); this.#start(q, true); }}>开始视频</button>
              </div>`;
            })}
        </div>
        ${this.queues.some((q) => q.priority_enabled)
          ? html`<p class="hint">部分队列支持 VIP 优先。</p>
              ${this.queues
                .filter((q) => q.priority_enabled)
                .map((q) => html`<button class="secondary" @click=${() => this.#start(q, false, true)}>${q.name} · VIP 语音</button>`)}`
          : ""}
      </div>
    `;
  }

  #queueIdleView() {
    return html`
      <div class="panel">
        <h3>排队状态</h3>
        <dl class="desc">
          <dt>前方等候</dt>
          <dd>0 位</dd>
          <dt>已等待</dt>
          <dd>00:00</dd>
          <dt>录音提示</dt>
          <dd>本通话可能会被录音</dd>
        </dl>
      </div>
    `;
  }

  #listView() {
    return html`
      <div class="panel">
        <h3>可进入队列</h3>
        <table>
          <tr><th>队列名称</th><th>类型</th><th>操作</th></tr>
          ${this.queues.map(
            (q) => html`<tr>
              <td>${q.name}</td>
              <td>${q.video_enabled ? "语音 / 视频" : "语音"}</td>
              <td><button class="ghost" @click=${() => this.#start(q, false)}>进入</button></td>
            </tr>`,
          )}
        </table>
      </div>
    `;
  }

  #waitView() {
    return html`
      <div class="panel">
        <h3>排队状态</h3>
        <p class="muted">${this.permissionHint || "正在等待坐席接听…"}</p>
        <dl class="desc">
          <dt>前方等候</dt>
          <dd>${this.position || 0} 位</dd>
          <dt>已等待</dt>
          <dd>${fmtMMSS(this.waitSec)}</dd>
          <dt>录音提示</dt>
          <dd>${this.notice || "本通话可能会被录音"}</dd>
        </dl>
        <p style="margin:16px 0 8px">IVR 请按键：</p>
        <div class="row">${["1", "2", "3", "4", "5", "6", "7", "8", "9", "*", "0", "#"].map(
          (d) => html`<button class="secondary" @click=${() => this.#dtmf(d)}>${d}</button>`,
        )}</div>
        <div class="toolbar" style="margin-top:16px">
          <button class="secondary" @click=${() => this.#hangup()}>结束排队</button>
        </div>
      </div>
    `;
  }

  #talkView() {
    return html`
      <div class="panel guest-talk">
        <h3>通话中 ${fmtMMSS(this.elapsed)}</h3>
        ${this.notice ? html`<p class="notice">${this.notice}</p>` : ""}
        ${this.videoAsk
          ? html`<p class="notice">坐席请求开启视频
              <button @click=${() => { respondVideo(this.join.call_id, true); this.videoAsk = null; }}>同意</button>
              <button class="secondary" @click=${() => { respondVideo(this.join.call_id, false); this.videoAsk = null; }}>拒绝</button>
            </p>`
          : ""}
        <div class="row">
          <video id="remote" autoplay playsinline></video>
          <video id="local" autoplay muted playsinline></video>
        </div>
        <div class="toolbar">
          <button class="secondary" @click=${() => this.#toggle("audio")}>${this.audioMuted ? "取消静音" : "静音"}</button>
          ${this.wantVideo
            ? html`<button class="secondary" @click=${() => this.#toggle("video")}>${this.videoMuted ? "开摄像头" : "关摄像头"}</button>`
            : html`<button class="secondary" @click=${() => respondVideo(this.join.call_id, true)}>同意升视频</button>`}
          <button class="danger" @click=${() => this.#hangup()}>挂断</button>
        </div>
        <div class="row">${["1", "2", "3", "4", "5", "6", "7", "8", "9", "*", "0", "#"].map(
          (d) => html`<button class="secondary" @click=${() => this.#dtmf(d)}>${d}</button>`,
        )}</div>
      </div>
    `;
  }

  #endedView() {
    return html`
      <div class="panel">
        <h3>通话已结束</h3>
        <p class="muted">设备已释放。</p>
        <button @click=${() => location.reload()}>返回</button>
      </div>
    `;
  }
}

customElements.define("open-voip-guest-app", GuestApp);
