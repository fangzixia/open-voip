import { bindApiFeedback } from "../shared/http-client.js";
import { beginTrace, clearCallContext, setCallContext } from "../shared/call-context.js";
import { formatDateTime } from "../shared/datetime.js";
import { LitElement, html } from "lit";
import {
  answerCall,
 fetchMyCalls,
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
  logout,
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
import { sessionTypeLabel } from "../shared/call-enums.js";
import { renderAppShell, renderDialpad, renderEmptyTable, renderFeedback, renderLoginLayout } from "../shared/components/ui.js";
import { agentStateLabel, agentStateTone, formatDuration } from "../shared/display.js";
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
import { reportEvent } from "../shared/observability.js";

const NAV = [
  { id: "desk", label: "工作台" },
  { id: "inbound", label: "呼入" },
  { id: "outbound", label: "外呼" },
  { id: "queue", label: "队列签入" },
  { id: "device", label: "设备" },
];

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
    cameraUnavailable: { type: Boolean },
    audioPlaybackBlocked: { type: Boolean },
    dest: { type: String },
    held: { type: Boolean },
    wrapNotes: { type: String },
    guestLink: { type: String },
    guestMedia: { type: String },
    guestExpiresAt: { type: String },
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
  #remoteAudio = null;
  #timer = null;
  #startedAt = 0;
  #clockTimer = null;
  #mediaConnecting = false;

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
    this.cameraUnavailable = false;
    this.audioPlaybackBlocked = false;
    this.dest = "";
    this.held = false;
    this.wrapNotes = "";
    this.guestLink = "";
    this.guestMedia = "video";
    this.guestExpiresAt = "";
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
    beginTrace({ role: "agent" });
    this._unbindApi = bindApiFeedback(this, () => { this.#ws.disconnect(); this.#endLocal(); this.me = null; });
    this.#tickClock();
    this.#clockTimer = setInterval(() => this.#tickClock(), 1000);
    if (getAccessToken()) this.#restore();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unbindApi?.();
    this.#ws.disconnect();
    this.#stopTimer();
    stopMedia(this.#pc, this.#local);
    clearInterval(this.#clockTimer);
  }

  #tickClock() {
    this.clock = formatDateTime();
  }

  async #restore() {
    try {
      this.me = await fetchAgentMe();
      setCallContext({ agent_id: this.me.id, role: this.me.role || "agent" });
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
    void this.#syncCalls();
  }

  /** 连接恢复后从服务端同步振铃和当前通话，必要时重新加入媒体。 */
  async #syncCalls() {
    try {
      this.me = await fetchAgentMe();
      const { items = [] } = await fetchMyCalls();
      const ringing = items.find(c => c.state === "ringing" && c.agent_id === this.me.id && !c.legs?.some(l => l.agent_id === this.me.id));
      this.incoming = ringing ? { ...ringing, call_id: ringing.id } : null;
      const active = items.find(c => c !== ringing);
      if (!active && this.call) this.#endLocal(this.call.id);
      if (active) {
        this.call = active;
        const activeLeg = active.legs?.find((item) => item.agent_id === this.me.id);
        setCallContext({ call_id: active.id, leg_id: activeLeg?.id || "", queue_id: active.queue_id || "" });
        if (["active", "held"].includes(active.state) && !this.#pc && this.me.terminal_type !== "sip") await this.#rejoinMedia(active.session_type !== "audio");
      }
    } catch (e) { this.error = e.message; }
  }

  /** 将业务事件映射为来电提示、通话状态和视频协商提示。 */
  #onWs(msg) {
    if (msg.type === "system.connected") { void this.#syncCalls(); return; }
    if (msg.type === "call.ringing") {
      this.incoming = msg.payload;
      setCallContext({ call_id: msg.payload?.call_id || "", queue_id: msg.payload?.queue_id || "" });
      reportEvent("call.ringing");
    } else if (msg.type === "call.ended") {
      reportEvent("call.ended", { reason: msg.payload?.reason || "" });
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
    } else if (msg.type === "call.answered" && msg.payload?.call_id) {
      void this.#syncCalls();
    }
  }

  async #doCheckIn() {
    try {
      await this.#ensureCheckedIn(true);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #ensureCheckedIn(force = false) {
    const state = this.me?.session?.state;
    if (!force && state && state !== "offline") return this.me.session;
    const session = await checkIn(this.me.id, this.selectedQueues);
    this.me = { ...this.me, session };
    if (session.queue_ids?.length) this.selectedQueues = [...session.queue_ids];
    this.error = "";
    return session;
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

  /** 接听当前来电并为坐席通话腿建立 WebRTC 会话。 */
  async #answer() {
    if (this.#mediaConnecting) return;
    this.#mediaConnecting = true;
    try {
      if (this.me?.terminal_type === "sip") { this.notice = "请在 SIP 话机接听"; return; }
 const call = await answerCall(this.incoming.call_id);
      this.call = call;
      this.incoming = null;
      this.nav = "desk";
      const video = call.session_type === "video" || call.session_type === "mixed";
      const leg = (call.legs || []).find((l) => l.agent_id === this.me?.id) || call.legs?.[1];
      setCallContext({ call_id: call.id, leg_id: leg?.id || "", queue_id: call.queue_id || "" });
      reportEvent("call.answered");
      const session = await startMediaSession({
        callId: call.id,
        legId: leg.id,
        video,
        audioDeviceId: this.audioDeviceId,
        videoDeviceId: this.videoDeviceId,
        allowAudioOnlyForVideo: true,
      });
      this.#pc = session.pc;
      this.#local = session.localStream;
      this.#remote = session.remoteStream;
      this.#remoteAudio = session.remoteAudioStream;
      this.cameraUnavailable = session.cameraUnavailable;
      this.error = "";
      this.#bindVideos();
      this.#startTimer();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    } finally {
      this.#mediaConnecting = false;
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

  /** 记录最近通话并释放本地媒体与界面状态。 */
  #endLocal(endedId) {
    const wrapId = endedId || this.call?.id;
    if (this.call || this.incoming) {
      this.recentCalls = [
        {
          time: formatDateTime(),
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
    this.#remoteAudio = null;
    this.cameraUnavailable = false;
    this.audioPlaybackBlocked = false;
    this.call = null;
    this.incoming = null;
    this.elapsed = 0;
    this.sharing = false;
    this.consulting = false;
    this.showPad = false;
    if (wrapId) {
      this.pendingWrapId = wrapId;
      setCallContext({ call_id: wrapId, leg_id: "" });
    } else {
      clearCallContext();
    }
  }

  /** 媒体类型变化或断线恢复时重新协商当前通话。 */
  async #rejoinMedia(video) {
    if (!this.call || this.me?.terminal_type === "sip" || this.#mediaConnecting) return;
    const leg = (this.call.legs || []).find((l) => l.agent_id === this.me?.id) || this.call.legs?.[1];
    if (!leg) return;
    this.#mediaConnecting = true;
    stopMedia(this.#pc, this.#local);
    try {
      const session = await startMediaSession({
        callId: this.call.id,
        legId: leg.id,
        video,
        audioDeviceId: this.audioDeviceId,
        videoDeviceId: this.videoDeviceId,
        allowAudioOnlyForVideo: true,
      });
      this.#pc = session.pc;
      this.#local = session.localStream;
      this.#remote = session.remoteStream;
      this.#remoteAudio = session.remoteAudioStream;
      this.cameraUnavailable = session.cameraUnavailable;
      this.error = "";
      this.call = { ...this.call, session_type: video ? "mixed" : "audio" };
      this.#bindVideos();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    } finally {
      this.#mediaConnecting = false;
    }
  }

  async #toggleMute(kind) {
    if (kind === "audio") this.audioMuted = !this.audioMuted;
    else this.videoMuted = !this.videoMuted;
    const leg = (this.call?.legs || []).find((l) => l.agent_id === this.me?.id);
    await setLocalMuted(this.#local, this.call?.id, leg?.id, {
      audio: this.audioMuted,
      video: this.videoMuted,
    });
  }

  async #dial() {
    try {
      if (!this.dest.trim()) {
        this.error = "请输入目标号码";
        return;
      }
      await this.#ensureCheckedIn();
      if (this.me?.terminal_type === "sip") { this.notice = "已自动签入，请从已注册的 SIP 话机拨号"; return; }
      const call = await outboundCall(this.dest.trim());
      this.call = call;
      this.nav = "desk";
      this.error = "";
      const leg = (call.legs || []).find((l) => l.agent_id === this.me?.id);
      setCallContext({ call_id: call.id, leg_id: leg?.id || "", queue_id: call.queue_id || "" });
      reportEvent("call.outbound_created", { state: call.state });
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
        this.#remoteAudio = session.remoteAudioStream;
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

  /** 根据页面选择的目标和模式发起转接。 */
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
      const leg = (this.call?.legs || []).find((l) => l.agent_id === this.me?.id);
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
      setCallContext({ call_id: out.call_id, leg_id: out.leg_id });
      reportEvent("call.listen_started");
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
      this.#remoteAudio = session.remoteAudioStream;
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
    const leg = (this.call?.legs || []).find((l) => l.agent_id === this.me?.id);
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
      clearCallContext();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #makeLink() {
    try {
      const qid = this.selectedQueues[0] || this.queues[0]?.id;
      const s = await createGuestSession(qid, 3600, this.guestMedia);
      this.guestLink = new URL(s.guest_url, location.origin).href;
      this.guestExpiresAt = s.expires_at || "";
      this.error = "";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #copyGuestLink() {
    if (!this.guestLink) return;
    try {
      await navigator.clipboard.writeText(this.guestLink);
      this.notice = "访客链接已复制";
    } catch {
      this.error = "复制失败，请手动复制链接";
    }
  }

  #bindVideos() {
    this.updateComplete.then(() => {
      const localV = this.renderRoot.querySelector("#local");
      const remoteV = this.renderRoot.querySelector("#remote");
      const remoteAudio = this.renderRoot.querySelector("#remote-audio");
      if (localV) localV.srcObject = this.#local;
      if (remoteV) {
        remoteV.srcObject = this.#remote;
        void remoteV.play().catch(() => {});
      }
      if (remoteAudio) {
        remoteAudio.srcObject = this.#remoteAudio;
        applyAudioOutput(remoteAudio, this.speakerDeviceId).catch(() => {});
        void this.#playRemoteAudio();
      }
    });
  }

  async #playRemoteAudio() {
    const audio = this.renderRoot.querySelector("#remote-audio");
    if (!audio) return;
    try {
      await audio.play();
      this.audioPlaybackBlocked = false;
    } catch {
      this.audioPlaybackBlocked = true;
    }
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
      if (!this.#local?.getVideoTracks().length) {
        await this.#rejoinMedia(true);
        return;
      }
      await replaceInputDevice(this.#pc, this.#local, "video", id);
      this.cameraUnavailable = false;
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  async #onSpeakerChange(id) {
    this.speakerDeviceId = id;
    const remoteV = this.renderRoot.querySelector("#remote-audio");
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

  async #logout() {
    try { await logout(); } catch (e) { this.error = e.message; }
    clearAccessToken();
    this.#ws.disconnect();
    this.#endLocal();
    this.pendingWrapId = "";
    clearCallContext();
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
    return renderAppShell({
      subtitle: "坐席工作台",
      navItems: NAV,
      activeNav: this.nav,
      onNavigate: (id) => {
        this.nav = id;
        if (id === "desk" && this.#pc) this.#bindVideos();
      },
      breadcrumb: this.#crumb(),
      badges: { inbound: this.incoming ? 1 : 0 },
      topbar: html`
          <span class="topbar-meta">${this.clock}</span>
          <span class="topbar-meta">${this.me.display_name || this.me.extension} · ${this.me.extension || ""}</span>
          <span class="tag ${agentStateTone(state)}">${agentStateLabel(state)}</span>
          <button @click=${() => this.#logout()}>退出</button>
      `,
      content: html`
        ${renderFeedback({ error: this.error, notice: this.notice })}
        ${this.nav === "desk" ? this.#deskView(state) : ""}
        ${this.nav === "inbound" ? this.#inboundView() : ""}
        ${this.nav === "outbound" ? this.#outboundView() : ""}
        ${this.nav === "queue" ? this.#queueView(state) : ""}
        ${this.nav === "device" ? this.#deviceView() : ""}
      `,
    });
  }

  #loginView() {
    return renderLoginLayout({
      subtitle: "坐席工作台",
      title: "坐席登录",
      hint: `WebRTC：${isWebRTCSupported() ? "支持" : "不支持"}`,
      onSubmit: (event) => this.#login(event),
      error: this.error,
      fields: html`
        <div class="field">
          <label for="agent-username">用户名</label>
          <input id="agent-username" autocomplete="username" .value=${this.username} @input=${(e) => (this.username = e.target.value)} />
        </div>
        <div class="field">
          <label for="agent-password">密码</label>
          <input id="agent-password" type="password" autocomplete="current-password" .value=${this.password} @input=${(e) => (this.password = e.target.value)} />
        </div>
      `,
    });
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
                  <td>${formatDuration(c.duration)}</td>
                </tr>`,
              )
            : renderEmptyTable(5, "暂无本会话通话记录")}
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
        <span class="muted">${this.incoming.queue_name || "—"} · ${sessionTypeLabel(this.incoming.session_type)}</span>
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
        <textarea class="acw-notes" .value=${this.wrapNotes} @input=${(e) => (this.wrapNotes = e.target.value)} placeholder="填写通话小结"></textarea>
        <button @click=${() => this.#submitWrap()}>提交小结并示闲</button>
      </div>
    `;
  }

  #stagePanel(inCall) {
    return html`
      <div class="stage ${this.sharing ? "share" : ""}">
        <div class="stage-timer">${formatDuration(this.elapsed)}</div>
        <div class="stage-meta">
          ${this.#stageCaller()}
          ${this.#stageQueue() !== "—" ? html` · ${this.#stageQueue()}` : ""}
          ${this.call?.session_type ? html` · ${sessionTypeLabel(this.call.session_type)}` : ""}
          ${this.held ? html` · 保持` : ""}
          ${!inCall ? html` · 空闲` : ""}
        </div>
        ${this.videoAsk && inCall
          ? html`<p class="notice">对端请求升视频
              <button @click=${() => this.#ws.send("video.respond", { call_id: this.call.id, accept: true })}>同意</button>
              <button class="secondary" @click=${() => this.#ws.send("video.respond", { call_id: this.call.id, accept: false })}>拒绝</button>
            </p>`
          : ""}
        ${inCall && this.cameraUnavailable
          ? html`<p class="notice">本机没有可用摄像头，已用麦克风接通；仍可观看访客视频。</p>`
          : ""}
        ${inCall && this.audioPlaybackBlocked
          ? html`<p class="notice">浏览器已暂停通话声音 <button @click=${() => this.#playRemoteAudio()}>播放声音</button></p>`
          : ""}
        ${inCall || this.#local
          ? html`<div class="stage-videos">
              ${inCall ? html`<video id="remote" autoplay muted playsinline></video><audio id="remote-audio" autoplay playsinline></audio>` : ""}
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
        ${this.showPad && inCall ? renderDialpad((digit) => this.#dtmf(digit)) : ""}
        ${inCall
          ? html`<div class="stage-extra">
              <button ?disabled=${this.cameraUnavailable} @click=${() => this.#toggleMute("video")}>${this.cameraUnavailable ? "无摄像头" : this.videoMuted ? "开摄像头" : "关摄像头"}</button>
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
            <textarea class="wrap-notes" .value=${this.wrapNotes} @input=${(e) => (this.wrapNotes = e.target.value)} placeholder="通话小结"></textarea>`
          : ""}
      </div>
    `;
  }

  #sideCard() {
    return html`
      <div>
        <div class="panel side-panel">
          <h3>外呼</h3>
          <div class="field">
            <label>目标号码</label>
            <input .value=${this.dest} @input=${(e) => (this.dest = e.target.value)} placeholder="bob / 分机 / 号码" />
          </div>
          <button @click=${() => this.#dial()}>拨出</button>
        </div>
        <div class="panel">
          <h3>排队</h3>
          <p class="queue-count">${this.#waitingCount()} <span class="queue-count-unit">人</span></p>
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
          <select aria-label="访客邀请媒体" .value=${this.guestMedia} @change=${(e) => (this.guestMedia = e.target.value)}>
            <option value="video">视频邀请</option>
            <option value="audio">语音邀请</option>
          </select>
          <button class="secondary" @click=${() => this.#makeLink()}>入会链接</button>
          ${this.me.role === "supervisor" || this.me.role === "admin"
            ? html`<button class="secondary" @click=${() => this.#listen()}>监听（填 call_id）</button>`
            : ""}
        </div>
        ${this.guestLink ? html`
          <div class="invite-link">
            <label for="guest-link">H5 访客链接${this.guestExpiresAt ? `（有效至 ${this.guestExpiresAt}）` : ""}</label>
            <input id="guest-link" readonly .value=${this.guestLink} />
            <button class="secondary" @click=${() => this.#copyGuestLink()}>复制链接</button>
          </div>` : ""}
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
        <div class="field field-narrow">
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
          <button class="secondary" ?disabled=${!this.devices.videoInputs.length} @click=${() => this.#preview()}>预览摄像头</button>
        </div>
        ${this.#local && !this.call ? html`<video id="local" autoplay muted playsinline></video>` : ""}
      </div>
    `;
  }
}

customElements.define("open-voip-agent-app", AgentApp);
