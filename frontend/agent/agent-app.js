import { LitElement, html } from "lit";
import {
  answerCall,
  checkIn,
  checkOut,
  conferenceInvite,
  createGuestSession,
  downgradeVideo,
  fetchAgentMe,
  fetchLiveReport,
  getCall,
  hangupCall,
  holdCall,
  listAgents,
  listQueues,
  listenCall,
  login,
  outboundCall,
  requestVideo,
  screenShare,
  sendDtmf,
  setAgentState,
  transferCall,
  completeTransfer,
  wrapUp,
} from "../shared/api.js";
import { clearAccessToken, getAccessToken, setAccessToken } from "../shared/auth-store.js";
import { shellStyles } from "../shared/shell-styles.js";
import {
  applyAudioOutput,
  isWebRTCSupported,
  listMediaDevices,
  replaceInputDevice,
  setLocalMuted,
  startMediaSession,
  startScreenShare,
  stopMedia,
} from "../shared/webrtc.js";
import { BusinessWebSocket } from "../shared/ws.js";

const STATE_LABEL = {
  idle: "空闲",
  busy: "示忙",
  ringing: "振铃",
  on_call: "通话中",
  acw: "事后处理",
  offline: "未签入",
};

const NAV = [
  { id: "desk", label: "工作台" },
  { id: "inbound", label: "呼入" },
  { id: "outbound", label: "外呼" },
  { id: "queue", label: "队列签入" },
  { id: "device", label: "设备" },
];

function fmtMMSS(sec) {
  const s = Math.max(0, Number(sec) || 0);
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
}

function stateLabel(state) {
  return STATE_LABEL[state] || state || "未签入";
}

function stateTag(state) {
  return { idle: "ok", busy: "warn", ringing: "info", on_call: "info", acw: "warn", offline: "muted" }[state] || "muted";
}

export class AgentApp extends LitElement {
  static properties = {
    error: { type: String },
    username: { type: String },
    password: { type: String },
    me: { type: Object },
    queues: { type: Array },
    selectedQueues: { type: Array },
    incoming: { type: Object },
    call: { type: Object },
    elapsed: { type: Number },
    audioMuted: { type: Boolean },
    videoMuted: { type: Boolean },
    dest: { type: String },
    held: { type: Boolean },
    wrapNotes: { type: String },
    guestLink: { type: String },
    notice: { type: String },
    videoAsk: { type: Object },
    busyReason: { type: String },
    agents: { type: Array },
    devices: { type: Object },
    audioDeviceId: { type: String },
    videoDeviceId: { type: String },
    speakerDeviceId: { type: String },
    xferMode: { type: String },
    sharing: { type: Boolean },
    pendingWrapId: { type: String },
    consulting: { type: Boolean },
    nav: { type: String },
    showPad: { type: Boolean },
    recentCalls: { type: Array },
    clock: { type: String },
    live: { type: Object },
  };

  static styles = shellStyles;

  #ws = new BusinessWebSocket();
  #pc = null;
  #local = null;
  #remote = null;
  #timer = null;
  #startedAt = 0;
  #clockTimer = null;

  constructor() {
    super();
    this.error = "";
    this.username = "agent1";
    this.password = "changeme";
    this.me = null;
    this.queues = [];
    this.selectedQueues = [];
    this.incoming = null;
    this.call = null;
    this.elapsed = 0;
    this.audioMuted = false;
    this.videoMuted = false;
    this.dest = "";
    this.held = false;
    this.wrapNotes = "";
    this.guestLink = "";
    this.notice = "";
    this.videoAsk = null;
    this.busyReason = "break";
    this.agents = [];
    this.devices = { audioInputs: [], videoInputs: [], audioOutputs: [] };
    this.audioDeviceId = "";
    this.videoDeviceId = "";
    this.speakerDeviceId = "";
    this.xferMode = "blind";
    this.sharing = false;
    this.pendingWrapId = "";
    this.consulting = false;
    this.nav = "desk";
    this.showPad = false;
    this.recentCalls = [];
    this.clock = "";
    this.live = null;
  }

  connectedCallback() {
    super.connectedCallback();
    this.#tickClock();
    this.#clockTimer = setInterval(() => this.#tickClock(), 1000);
    if (getAccessToken()) this.#restore();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.#ws.disconnect();
    this.#stopTimer();
    stopMedia(this.#pc, this.#local);
    clearInterval(this.#clockTimer);
  }

  #tickClock() {
    this.clock = new Date().toLocaleTimeString("zh-CN", { hour12: false });
  }

  async #restore() {
    try {
      this.me = await fetchAgentMe();
      this.queues = (await listQueues()).items || [];
      this.devices = await listMediaDevices();
      this.agents = (await listAgents()).items || [];
      try {
        this.live = await fetchLiveReport();
      } catch {
        this.live = null;
      }
      this.#connectWs();
      this.error = "";
    } catch (e) {
      clearAccessToken();
      this.me = null;
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #login(ev) {
    ev.preventDefault();
    try {
      const tokens = await login(this.username, this.password);
      setAccessToken(tokens.access_token, { persist: true });
      await this.#restore();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  #connectWs() {
    this.#ws.disconnect();
    this.#ws.connect();
    this.#ws.subscribe((msg) => this.#onWs(msg));
  }

  #onWs(msg) {
    if (msg.type === "call.ringing") {
      this.incoming = msg.payload;
    } else if (msg.type === "call.ended") {
      if (this.call?.id === msg.payload?.call_id || this.incoming?.call_id === msg.payload?.call_id) {
        this.#endLocal(msg.payload?.call_id);
      }
    } else if (msg.type === "agent.state_changed" && this.me) {
      this.me = { ...this.me, session: { ...this.me.session, state: msg.payload.state, busy_reason: msg.payload.busy_reason } };
    } else if (msg.type === "recording.notice") {
      this.notice = msg.payload?.message || "";
    } else if (msg.type === "video.requested") {
      this.videoAsk = msg.payload;
    } else if (msg.type === "video.accepted") {
      this.notice = "对端已同意开启视频";
      this.#rejoinMedia(true);
    } else if (msg.type === "video.downgraded") {
      this.notice = "已降为语音";
      this.#rejoinMedia(false);
    } else if (msg.type === "call.consulting") {
      this.consulting = true;
      this.notice = "咨询转已接通，可完成转接或继续三方";
    } else if (msg.type === "call.transferred") {
      this.consulting = false;
      if (msg.payload?.from_agent_id === this.me?.id) {
        this.notice = "咨询转已完成，请填写小结";
        this.#endLocal(this.call?.id);
      } else {
        this.notice = "咨询转已完成，客户已接回";
      }
    } else if (msg.type === "call.answered" && !this.call && msg.payload?.call_id) {
      getCall(msg.payload.call_id).then((c) => {
        this.call = c;
      });
    }
  }

  async #doCheckIn() {
    try {
      const sess = await checkIn(this.me.id, this.selectedQueues);
      this.me = { ...this.me, session: sess };
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #doCheckOut() {
    try {
      await checkOut(this.me.id);
      this.me = { ...this.me, session: { ...this.me.session, state: "offline", queue_ids: [] } };
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #toggleBusy() {
    const next = this.me.session?.state === "busy" ? "idle" : "busy";
    try {
      const sess = await setAgentState(this.me.id, next, next === "busy" ? this.busyReason || "break" : "");
      this.me = { ...this.me, session: sess };
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #setIdle() {
    try {
      const sess = await setAgentState(this.me.id, "idle", "");
      this.me = { ...this.me, session: sess };
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #answer() {
    try {
      const call = await answerCall(this.incoming.call_id);
      this.call = call;
      this.incoming = null;
      this.nav = "desk";
      const video = call.session_type === "video" || call.session_type === "mixed";
      const leg = (call.legs || []).find((l) => l.role === "agent") || call.legs?.[1];
      const session = await startMediaSession({
        callId: call.id,
        legId: leg.id,
        video,
        audioDeviceId: this.audioDeviceId,
        videoDeviceId: this.videoDeviceId,
      });
      this.#pc = session.pc;
      this.#local = session.localStream;
      this.#remote = session.remoteStream;
      this.#bindVideos();
      this.#startTimer();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  #decline() {
    this.#ws.send("call.decline", { call_id: this.incoming?.call_id, reason: "busy" });
    this.incoming = null;
  }

  async #hangup() {
    const id = this.call?.id;
    if (id) {
      try {
        await hangupCall(id);
      } catch {
        /* 对端可能已挂 */
      }
    }
    this.#endLocal(id);
  }

  #endLocal(endedId) {
    const wrapId = endedId || this.call?.id;
    if (this.call || this.incoming) {
      this.recentCalls = [
        {
          time: new Date().toLocaleTimeString("zh-CN", { hour12: false, hour: "2-digit", minute: "2-digit" }),
          caller: this.call?.caller || this.incoming?.caller || this.dest || "—",
          queue: this.call?.queue_name || this.incoming?.queue_name || "—",
          result: this.call ? "已接通" : "未接",
          duration: this.elapsed,
        },
        ...this.recentCalls,
      ].slice(0, 20);
    }
    this.#stopTimer();
    stopMedia(this.#pc, this.#local);
    this.#pc = null;
    this.#local = null;
    this.#remote = null;
    this.call = null;
    this.incoming = null;
    this.elapsed = 0;
    this.sharing = false;
    this.consulting = false;
    this.showPad = false;
    if (wrapId) this.pendingWrapId = wrapId;
  }

  async #rejoinMedia(video) {
    if (!this.call) return;
    const leg = (this.call.legs || []).find((l) => l.role === "agent") || this.call.legs?.[1];
    if (!leg) return;
    stopMedia(this.#pc, this.#local);
    try {
      const session = await startMediaSession({
        callId: this.call.id,
        legId: leg.id,
        video,
        audioDeviceId: this.audioDeviceId,
        videoDeviceId: this.videoDeviceId,
      });
      this.#pc = session.pc;
      this.#local = session.localStream;
      this.#remote = session.remoteStream;
      this.call = { ...this.call, session_type: video ? "mixed" : "audio" };
      this.#bindVideos();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #toggleMute(kind) {
    if (kind === "audio") this.audioMuted = !this.audioMuted;
    else this.videoMuted = !this.videoMuted;
    const leg = (this.call?.legs || []).find((l) => l.role === "agent");
    await setLocalMuted(this.#local, this.call?.id, leg?.id, {
      audio: this.audioMuted,
      video: this.videoMuted,
    });
  }

  async #dial() {
    try {
      const call = await outboundCall(this.dest);
      this.call = call;
      this.nav = "desk";
      const leg = (call.legs || []).find((l) => l.role === "agent");
      if (call.state === "active" && leg) {
        const session = await startMediaSession({
          callId: call.id,
          legId: leg.id,
          video: false,
          audioDeviceId: this.audioDeviceId,
        });
        this.#pc = session.pc;
        this.#local = session.localStream;
        this.#remote = session.remoteStream;
        this.#bindVideos();
        this.#startTimer();
      }
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #hold() {
    this.held = !this.held;
    try {
      await holdCall(this.call.id, this.held);
    } catch (e) {
      this.held = !this.held;
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #xfer() {
    try {
      const target = this.agents.find((a) => a.extension === this.dest);
      const mode = this.xferMode || "blind";
      await transferCall(
        this.call.id,
        target ? { mode, target_agent_id: target.id } : { mode: "blind", target_queue_id: this.dest },
      );
      this.consulting = mode === "consult";
      this.notice = mode === "consult" ? "咨询转振铃中，客户已保持" : "已盲转";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #completeXfer() {
    try {
      await completeTransfer(this.call.id);
      this.consulting = false;
      this.notice = "咨询转已完成";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #share() {
    try {
      if (!this.#pc?.getSenders().some((s) => s.track?.kind === "video")) {
        await this.#rejoinMedia(true);
      }
      const stream = await startScreenShare(this.#pc);
      const leg = (this.call?.legs || []).find((l) => l.role === "agent");
      await screenShare(this.call.id, true, leg?.id);
      this.sharing = true;
      stream.getVideoTracks()[0].onended = () => {
        this.sharing = false;
        screenShare(this.call.id, false, leg?.id);
      };
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #askVideo() {
    try {
      await requestVideo(this.call.id);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #downgrade() {
    try {
      await downgradeVideo(this.call.id);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #conf() {
    try {
      const target = this.agents.find((a) => a.extension === this.dest);
      if (!target) throw new Error("请先填写对方分机");
      await conferenceInvite(this.call.id, target.id);
      this.notice = "已邀请第三人，对方振铃中";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #listen() {
    try {
      const id = this.incoming?.call_id || this.dest;
      const out = await listenCall(id);
      this.call = await getCall(out.call_id);
      this.nav = "desk";
      const session = await startMediaSession({
        callId: out.call_id,
        legId: out.leg_id,
        video: false,
        recvOnly: true,
      });
      this.#pc = session.pc;
      this.#remote = session.remoteStream;
      this.#bindVideos();
      this.#startTimer();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #preview() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: false,
        video: this.videoDeviceId ? { deviceId: { exact: this.videoDeviceId } } : true,
      });
      this.#local = stream;
      this.#bindVideos();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #dtmf(d) {
    const leg = (this.call?.legs || []).find((l) => l.role === "agent");
    await sendDtmf(this.call.id, leg?.id, d);
  }

  async #submitWrap() {
    const id = this.call?.id || this.pendingWrapId;
    try {
      await wrapUp(id, this.wrapNotes);
      try {
        const sess = await setAgentState(this.me.id, "idle", "wrap-up");
        this.me = { ...this.me, session: sess };
      } catch {
        /* 通话中提交小结时状态仍为 on_call */
      }
      this.notice = "小结已提交，已示闲";
      this.pendingWrapId = "";
      this.wrapNotes = "";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #makeLink() {
    try {
      const qid = this.selectedQueues[0] || this.queues[0]?.id;
      const s = await createGuestSession(qid, 3600, "audio");
      this.guestLink = s.guest_url;
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  #bindVideos() {
    this.updateComplete.then(() => {
      const localV = this.renderRoot.querySelector("#local");
      const remoteV = this.renderRoot.querySelector("#remote");
      if (localV) localV.srcObject = this.#local;
      if (remoteV) {
        remoteV.srcObject = this.#remote;
        applyAudioOutput(remoteV, this.speakerDeviceId).catch(() => {});
      }
    });
  }

  async #onMicChange(id) {
    this.audioDeviceId = id;
    if (!this.#pc || !this.call) return;
    try {
      await replaceInputDevice(this.#pc, this.#local, "audio", id);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #onCamChange(id) {
    this.videoDeviceId = id;
    if (!this.#pc || !this.call) return;
    try {
      await replaceInputDevice(this.#pc, this.#local, "video", id);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #onSpeakerChange(id) {
    this.speakerDeviceId = id;
    const remoteV = this.renderRoot.querySelector("#remote");
    try {
      await applyAudioOutput(remoteV, id);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  #startTimer() {
    this.#startedAt = Date.now();
    this.#timer = setInterval(() => {
      this.elapsed = Math.floor((Date.now() - this.#startedAt) / 1000);
    }, 500);
  }

  #stopTimer() {
    clearInterval(this.#timer);
    this.#timer = null;
  }

  #logout() {
    clearAccessToken();
    this.#ws.disconnect();
    this.#endLocal();
    this.pendingWrapId = "";
    this.nav = "desk";
    this.me = null;
  }

  #crumb() {
    return NAV.find((n) => n.id === this.nav)?.label || "工作台";
  }

  #waitingCount() {
    const queues = this.live?.queues || [];
    if (!queues.length) return 0;
    return queues.reduce((n, q) => n + (q.waiting || 0), 0);
  }

  #stageCaller() {
    return this.call?.caller || this.incoming?.caller || this.dest || "等待来电";
  }

  #stageQueue() {
    return this.call?.queue_name || this.incoming?.queue_name || "—";
  }

  render() {
    if (!this.me) return this.#loginView();
    const state = this.me.session?.state || "offline";
    return html`
      <div class="layout">
        <header class="topbar">
          <div class="brand">
            <span class="brand-mark">OV</span>
            <span>Open VoIP</span>
            <span class="brand-sub">坐席工作台</span>
          </div>
          <span class="spacer"></span>
          <span class="topbar-meta">${this.clock}</span>
          <span class="topbar-meta">${this.me.display_name || this.me.extension} · ${this.me.extension || ""}</span>
          <span class="tag ${stateTag(state)}">${stateLabel(state)}</span>
          <button @click=${() => this.#logout()}>退出</button>
        </header>
        <div class="layout-body">
          <aside class="sidebar">
            ${NAV.map(
              (n) => html`<button class="nav-item ${this.nav === n.id ? "active" : ""}" @click=${() => (this.nav = n.id)}>
                <span>${n.label}</span>
                ${n.id === "inbound" && this.incoming ? html`<span class="nav-badge">1</span>` : ""}
              </button>`,
            )}
          </aside>
          <div class="content">
            <div class="breadcrumb">坐席工作台 / <strong>${this.#crumb()}</strong></div>
            ${this.error ? html`<p class="error">${this.error}</p>` : ""}
            ${this.notice ? html`<p class="notice">${this.notice}</p>` : ""}
            ${this.nav === "desk" ? this.#deskView(state) : ""}
            ${this.nav === "inbound" ? this.#inboundView() : ""}
            ${this.nav === "outbound" ? this.#outboundView() : ""}
            ${this.nav === "queue" ? this.#queueView(state) : ""}
            ${this.nav === "device" ? this.#deviceView() : ""}
          </div>
        </div>
      </div>
    `;
  }

  #loginView() {
    return html`
      <div class="login-page">
        <header class="topbar">
          <div class="brand"><span class="brand-mark">OV</span><span>Open VoIP</span><span class="brand-sub">坐席工作台</span></div>
        </header>
        <div class="login-wrap">
          <form class="login-card" @submit=${(e) => this.#login(e)}>
            <h2>坐席登录</h2>
            <p class="hint">演示账号 agent1 / changeme。WebRTC：${isWebRTCSupported() ? "支持" : "不支持"}</p>
            <div class="field">
              <label>用户名</label>
              <input .value=${this.username} @input=${(e) => (this.username = e.target.value)} />
            </div>
            <div class="field">
              <label>密码</label>
              <input type="password" .value=${this.password} @input=${(e) => (this.password = e.target.value)} />
            </div>
            <button type="submit">登录</button>
            ${this.error ? html`<p class="error" style="margin-top:12px">${this.error}</p>` : ""}
          </form>
        </div>
      </div>
    `;
  }

  #deskView(state) {
    const inCall = !!this.call;
    return html`
      <div class="panel">
        <div class="toolbar tight">
          <button @click=${() => this.#doCheckIn()}>签入</button>
          <button class="secondary" @click=${() => this.#doCheckOut()}>签出</button>
          <button class="${state === "idle" ? "" : "secondary"}" @click=${() => this.#setIdle()}>示闲</button>
          <button class="${state === "busy" ? "" : "secondary"}" @click=${() => this.#toggleBusy()}>示忙</button>
        </div>
        ${this.incoming && !inCall ? this.#incomingBar() : ""}
        ${!inCall && this.pendingWrapId ? this.#acwPanel() : ""}
        <div class="workbench">
          ${this.#stagePanel(inCall)}
          ${this.#sideCard()}
        </div>
        <h4>近期通话</h4>
        <table>
          <tr>
            <th>时间</th>
            <th>主叫</th>
            <th>队列</th>
            <th>结果</th>
            <th>时长</th>
          </tr>
          ${this.recentCalls.length
            ? this.recentCalls.map(
                (c) => html`<tr>
                  <td>${c.time}</td>
                  <td>${c.caller}</td>
                  <td>${c.queue}</td>
                  <td>${c.result}</td>
                  <td>${fmtMMSS(c.duration)}</td>
                </tr>`,
              )
            : html`<tr><td colspan="5" class="muted">暂无本会话通话记录</td></tr>`}
        </table>
        <div class="pager">共 ${this.recentCalls.length} 条</div>
      </div>
    `;
  }

  #incomingBar() {
    return html`
      <div class="incoming-bar">
        <strong>来电</strong>
        <span>${this.incoming.caller || "未知主叫"}</span>
        <span class="muted">${this.incoming.queue_name || "—"} · ${this.incoming.session_type || "audio"}</span>
        <span class="spacer"></span>
        <button @click=${() => this.#answer()}>接听</button>
        <button class="danger" @click=${() => this.#decline()}>拒接</button>
      </div>
    `;
  }

  #acwPanel() {
    return html`
      <div class="incoming-bar">
        <strong>事后处理</strong>
        <span class="muted">通话 ${this.pendingWrapId}</span>
        <textarea style="flex:1;min-width:220px;min-height:48px" .value=${this.wrapNotes} @input=${(e) => (this.wrapNotes = e.target.value)} placeholder="填写通话小结"></textarea>
        <button @click=${() => this.#submitWrap()}>提交小结并示闲</button>
      </div>
    `;
  }

  #stagePanel(inCall) {
    return html`
      <div class="stage ${this.sharing ? "share" : ""}">
        <div class="stage-timer">${fmtMMSS(this.elapsed)}</div>
        <div class="stage-meta">
          ${this.#stageCaller()}
          ${this.#stageQueue() !== "—" ? html` · ${this.#stageQueue()}` : ""}
          ${this.call?.session_type ? html` · ${this.call.session_type}` : ""}
          ${this.held ? html` · 保持` : ""}
          ${!inCall ? html` · 空闲` : ""}
        </div>
        ${this.videoAsk && inCall
          ? html`<p class="notice">对端请求升视频
              <button @click=${() => this.#ws.send("video.respond", { call_id: this.call.id, accept: true })}>同意</button>
              <button class="secondary" @click=${() => this.#ws.send("video.respond", { call_id: this.call.id, accept: false })}>拒绝</button>
            </p>`
          : ""}
        ${inCall || this.#local
          ? html`<div class="stage-videos">
              ${inCall ? html`<video id="remote" autoplay playsinline></video>` : ""}
              <video id="local" autoplay muted playsinline></video>
            </div>`
          : ""}
        <div class="stage-controls">
          <button class="ctl ${this.audioMuted ? "on" : ""}" ?disabled=${!inCall} @click=${() => this.#toggleMute("audio")}>${this.audioMuted ? "取消静音" : "静音"}</button>
          <button class="ctl ${this.held ? "on" : ""}" ?disabled=${!inCall} @click=${() => this.#hold()}>${this.held ? "恢复" : "保持"}</button>
          <button class="ctl ${this.showPad ? "on" : ""}" ?disabled=${!inCall} @click=${() => (this.showPad = !this.showPad)}>键盘</button>
          <button class="ctl" ?disabled=${!inCall} @click=${() => this.#xfer()}>转接</button>
          <button class="hangup" ?disabled=${!inCall} @click=${() => this.#hangup()}>挂断</button>
        </div>
        ${this.showPad && inCall
          ? html`<div class="dialpad">${["1", "2", "3", "4", "5", "6", "7", "8", "9", "*", "0", "#"].map(
              (d) => html`<button @click=${() => this.#dtmf(d)}>${d}</button>`,
            )}</div>`
          : ""}
        ${inCall
          ? html`<div class="stage-extra">
              <button @click=${() => this.#toggleMute("video")}>${this.videoMuted ? "开摄像头" : "关摄像头"}</button>
              <button @click=${() => this.#share()}>屏幕共享</button>
              <button @click=${() => this.#askVideo()}>升视频</button>
              <button @click=${() => this.#downgrade()}>降为语音</button>
              <select .value=${this.xferMode} @change=${(e) => (this.xferMode = e.target.value)}>
                <option value="blind">盲转</option>
                <option value="consult">咨询转</option>
              </select>
              ${this.consulting ? html`<button @click=${() => this.#completeXfer()}>完成转接</button>` : ""}
              <button @click=${() => this.#conf()}>邀请三方</button>
              <button @click=${() => this.#submitWrap()}>提交小结</button>
            </div>
            <textarea style="margin-top:12px;max-width:480px" .value=${this.wrapNotes} @input=${(e) => (this.wrapNotes = e.target.value)} placeholder="通话小结"></textarea>`
          : ""}
      </div>
    `;
  }

  #sideCard() {
    return html`
      <div>
        <div class="panel" style="margin-bottom:16px">
          <h3>外呼</h3>
          <div class="field">
            <label>目标号码</label>
            <input .value=${this.dest} @input=${(e) => (this.dest = e.target.value)} placeholder="bob / 分机 / 号码" />
          </div>
          <button @click=${() => this.#dial()}>拨出</button>
        </div>
        <div class="panel">
          <h3>排队</h3>
          <p style="margin:0;font-size:24px;color:#1890ff;font-weight:600">${this.#waitingCount()} <span class="muted" style="font-size:13px;font-weight:400">人</span></p>
        </div>
      </div>
    `;
  }

  #inboundView() {
    return html`
      <div class="panel">
        <h3>呼入</h3>
        ${this.incoming
          ? this.#incomingBar()
          : html`<p class="muted">当前没有振铃来电。签入队列后，来电会显示在此处，也可在工作台接听。</p>`}
      </div>
    `;
  }

  #outboundView() {
    return html`
      <div class="panel">
        <h3>外呼</h3>
        <div class="form-inline">
          <div class="field">
            <label>目标号码</label>
            <input .value=${this.dest} @input=${(e) => (this.dest = e.target.value)} placeholder="bob / 分机 / 号码" />
          </div>
          <button @click=${() => this.#dial()}>拨出</button>
          <button class="secondary" @click=${() => this.#makeLink()}>入会链接</button>
          ${this.me.role === "supervisor" || this.me.role === "admin"
            ? html`<button class="secondary" @click=${() => this.#listen()}>监听（填 call_id）</button>`
            : ""}
        </div>
        ${this.guestLink ? html`<p>访客链接：${this.guestLink}</p>` : ""}
        <p class="hint">分机互拨填写对方分机；SIP 软电话填写 bob；PSTN 填 8 位以上号码。</p>
      </div>
    `;
  }

  #queueView(state) {
    return html`
      <div class="panel">
        <h3>队列签入</h3>
        ${this.queues.map(
          (q) => html`<label class="check"
            ><input
              type="checkbox"
              .checked=${this.selectedQueues.includes(q.id)}
              @change=${(e) => {
                if (e.target.checked) this.selectedQueues = [...this.selectedQueues, q.id];
                else this.selectedQueues = this.selectedQueues.filter((id) => id !== q.id);
              }}
            />
            ${q.name} ${q.video_enabled ? "(视频)" : "(语音)"}</label
          >`,
        )}
        <div class="toolbar">
          <button @click=${() => this.#doCheckIn()}>签入</button>
          <button class="secondary" @click=${() => this.#doCheckOut()}>签出</button>
          <button class="secondary" @click=${() => this.#toggleBusy()}>${state === "busy" ? "示闲" : "示忙"}</button>
        </div>
        <div class="field" style="max-width:240px">
          <label>示忙原因</label>
          <select .value=${this.busyReason} @change=${(e) => (this.busyReason = e.target.value)}>
            <option value="break">小休</option>
            <option value="training">培训</option>
            <option value="meeting">会议</option>
          </select>
        </div>
      </div>
    `;
  }

  #deviceView() {
    return html`
      <div class="panel">
        <h3>设备</h3>
        <div class="form-inline">
          <div class="field">
            <label>麦克风</label>
            <select @change=${(e) => this.#onMicChange(e.target.value)}>
              ${this.devices.audioInputs.map((d) => html`<option value=${d.deviceId} ?selected=${d.deviceId === this.audioDeviceId}>${d.label || d.deviceId}</option>`)}
            </select>
          </div>
          <div class="field">
            <label>摄像头</label>
            <select @change=${(e) => this.#onCamChange(e.target.value)}>
              ${this.devices.videoInputs.map((d) => html`<option value=${d.deviceId} ?selected=${d.deviceId === this.videoDeviceId}>${d.label || d.deviceId}</option>`)}
            </select>
          </div>
          <div class="field">
            <label>扬声器</label>
            <select @change=${(e) => this.#onSpeakerChange(e.target.value)}>
              ${this.devices.audioOutputs.map((d) => html`<option value=${d.deviceId} ?selected=${d.deviceId === this.speakerDeviceId}>${d.label || d.deviceId || "默认"}</option>`)}
            </select>
          </div>
          <button class="secondary" @click=${() => this.#preview()}>预览摄像头</button>
        </div>
        ${this.#local && !this.call ? html`<video id="local" autoplay muted playsinline></video>` : ""}
      </div>
    `;
  }
}

customElements.define("open-voip-agent-app", AgentApp);
