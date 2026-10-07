// 坐席工作台：签入、通话控制、WebRTC 与业务动作回调。
import { NAV, renderApp } from "./views/shell.js";
import { AgentMediaController } from "./controllers/media.js";
import { bindApiFeedback } from "../shared/http-client.js";
import { FeedbackController, runFeedbackAction } from "../shared/feedback.js";
import { beginTrace, clearCallContext, setCallContext } from "../shared/call-context.js";
import { formatDateTime } from "../shared/datetime.js";
import { LitElement } from "lit";
import { answerCall, authMe, authOptions, declineCall, fetchMyCalls, checkIn, checkOut, conferenceInvite, createGuestSession, downgradeVideo, exchangeSSOTicket, fetchAgentMe, fetchLiveReport, getCall, hangupCall, holdCall, listAgents, listIvrAssets, listQueues, listenCall, login, logout, outboundCall, popSSOTicket, requestVideo, sendDtmf, setAgentState, startSSO, setCallVersion, startSurvey, transferCall, completeTransfer, voiceNotificationCall, wrapUp } from "../shared/api.js";
import { clearAccessToken, getAccessToken, setAuthTokens } from "../shared/auth-store.js";
import { appStyles } from "../shared/styles/index.js";
import { listMediaDevices } from "../shared/webrtc.js";
import { BusinessWebSocket } from "../shared/ws.js";
import { reportEvent } from "../shared/observability.js";
import {
  applyCallEnded,
  isStaleCallMediaError,
  notifyCallFailure,
  outboundProgressLabel,
  recentCallResultLabel,
  shouldPromptWrapUp,
} from "../shared/call-outcome.js";
import { isAgentNav } from "../shared/staff-nav.js";


export class AgentApp extends LitElement {
  static properties = {
    error: { type: String },
    authOptions: { type: Object },
    permissions: { type: Array },
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
    embedded: { type: Boolean },
    workspaceActive: { type: Boolean },
    contentOnly: { type: Boolean, attribute: "content-only" },
    staffNav: { type: String, attribute: "staff-nav" },
    outboundNotice: { type: String },
    promptAssetId: { type: String },
    ivrAssets: { type: Array },
    voiceNotifyBusy: { type: Boolean },
  };

  static styles = appStyles;

  #ws = new BusinessWebSocket();
  /** @type {(() => void)|null} */
  #wsUnsub = null;
  #timer = null;
  #startedAt = 0;
  #clockTimer = null;
  /** 媒体 join 世代：挂断/失败/新拨号时递增，作废进行中的 WebRTC。 */
  #joinEpoch = 0;
  /** 已通过 WS 提示过失败文案的 call_id，避免与 call.ended 重复 toast。 */
  #failureNotifiedCallId = "";
  feedback = new FeedbackController(this);
  media = new AgentMediaController(this);

  get hasLocal() { return this.media.hasLocal; }

  constructor() {
    super();
    this.error = "";
    this.authOptions = null;
    this.permissions = [];
    this.username = "";
    this.password = "";
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
    this.guestMedia = "audio";
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
    this.embedded = false;
    this.workspaceActive = false;
    this.contentOnly = false;
    this.staffNav = "";
    this.outboundNotice = "";
    this.promptAssetId = "";
    this.ivrAssets = [];
    this.voiceNotifyBusy = false;
  }

  #emitNavBadges() {
    this.dispatchEvent(new CustomEvent("staff-nav-badges", {
      bubbles: true,
      composed: true,
      detail: { inbound: this.incoming ? 1 : 0 },
    }));
  }

  #run(fn) {
    return runFeedbackAction(this.feedback, fn);
  }

  getJoinEpoch() {
    return this.#joinEpoch;
  }

  shouldSuppressJoinError(err, callId, joinEpoch) {
    return isStaleCallMediaError(err, {
      callId,
      failedCallId: this.#failureNotifiedCallId,
      joinEpoch,
      currentJoinEpoch: this.#joinEpoch,
    });
  }

  #invalidateJoin(callIdForNotice = "") {
    this.#joinEpoch += 1;
    this.media.stop();
    if (callIdForNotice) this.#failureNotifiedCallId = callIdForNotice;
  }

  connectedCallback() {
    super.connectedCallback();
    beginTrace({ role: "agent" });
    this._unbindApi = bindApiFeedback(this, () => { this.#ws.disconnect(); this.#endLocal(); this.me = null; });
    this.#tickClock();
    this.#clockTimer = setInterval(() => this.#tickClock(), 1000);
    if (this.embedded) {
      if (getAccessToken()) this.#restore();
      return;
    }
    authOptions().then((v) => { this.authOptions = v || {}; }).catch((e) => { this.authOptions = { unavailable: true }; this.feedback.liveError(e instanceof Error ? e.message : "无法读取登录方式"); });
    const ticket = popSSOTicket();
    if (ticket) this.#completeSSO(ticket);
    else if (getAccessToken()) this.#restore();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unbindApi?.();
    this.#ws.disconnect();
    this.#stopTimer();
    this.media.stop();
    clearInterval(this.#clockTimer);
  }

  updated(changed) {
    if (changed.has("workspaceActive") && this.workspaceActive) {
      this.feedback.activate();
      if (this.embedded && !this.me && getAccessToken()) void this.#restore();
    }
    if (this.contentOnly && changed.has("staffNav") && isAgentNav(this.staffNav) && this.staffNav !== this.nav) {
      this.feedback.begin();
      this.nav = this.staffNav;
      if (this.nav === "outbound") void this.#loadIvrAssets();
      if (this.nav === "desk" && this.media.pc) this.media.bindVideos();
    }
    if (changed.has("incoming")) this.#emitNavBadges();
  }

  #tickClock() {
    this.clock = formatDateTime();
  }

  async #restore() {
    await this.#run(async () => {
      try {
        const identity = await authMe();
        this.permissions = identity.permissions || [];
        const can = (code) => this.permissions.includes(code);
        this.me = await fetchAgentMe();
        setCallContext({ agent_id: this.me.id, role: this.me.role || "agent" });
        this.queues = can("queues.read") ? (await listQueues()).items || [] : [];
        this.devices = await listMediaDevices();
        this.agents = can("agents.read") ? (await listAgents()).items || [] : [];
        if (!can("calls.operate") && ["inbound", "outbound"].includes(this.nav)) this.nav = "queue";
        try {
          this.live = await fetchLiveReport();
        } catch {
          this.live = null;
        }
        this.#connectWs();
        this.feedback.clear();
      } catch (e) {
        clearAccessToken();
        this.me = null;
        if (this.embedded) {
          this.dispatchEvent(new CustomEvent("session-ended", { bubbles: true, composed: true }));
        }
        throw e;
      }
    });
  }

  async #login(ev) {
    ev.preventDefault();
    await this.#run(async () => {
      const tokens = await login(this.username, this.password);
      setAuthTokens(tokens, { persist: true });
      await this.#restore();
    });
  }

  async #completeSSO(ticket) {
    await this.#run(async () => {
      const tokens = await exchangeSSOTicket(ticket);
      setAuthTokens(tokens, { persist: true });
      await this.#restore();
    });
  }

  #connectWs() {
    this.#wsUnsub?.();
    this.#ws.disconnect();
    this.#ws.connect();
    this.#wsUnsub = this.#ws.subscribe((msg) => this.#onWs(msg));
    void this.#syncCalls();
  }

  /** 连接恢复后从服务端同步振铃和当前通话，必要时重新加入媒体。 */
  async #syncCalls() {
    await this.#run(async () => {
      this.me = await fetchAgentMe();
      const { items = [] } = await fetchMyCalls();
      const ringing = items.find(c => c.state === "ringing" && c.agent_id === this.me.id && !c.legs?.some(l => l.agent_id === this.me.id));
      this.incoming = ringing ? { ...ringing, call_id: ringing.id } : null;
      const active = items.find(c => c !== ringing);
      if (!active && this.call) {
        let endMeta = { result: "failed" };
        try {
          const ended = await getCall(this.call.id);
          if (ended?.result) endMeta = { result: ended.result };
        } catch {
          /* 已结束或不可读 */
        }
        this.#endLocal(this.call.id, endMeta);
      }
      if (active) {
        if (active.outbound_mode === "prompt_outbound") {
          this.outboundNotice = outboundProgressLabel(active.state === "active" ? "playing" : "dialing");
          return;
        }
        this.call = active;
        if (active?.version != null) setCallVersion(active.version, active.id);
        const activeLeg = active.legs?.find((item) => item.agent_id === this.me.id);
        setCallContext({ call_id: active.id, leg_id: activeLeg?.id || "", queue_id: active.queue_id || "" });
        if (["active", "held"].includes(active.state) && !this.media.pc && this.me.terminal_type !== "sip") {
          const joinEpoch = this.#joinEpoch;
          try {
            await this.media.rejoin(active.session_type !== "audio");
          } catch (err) {
            if (!this.shouldSuppressJoinError(err, active.id, joinEpoch)) throw err;
          }
        }
      }
    });
  }

  /** 将业务事件映射为来电提示、通话状态和视频协商提示。 */
  #onWs(msg) {
    if (msg.type === "system.connected") { void this.#syncCalls(); return; }
    if (msg.type === "call.ringing") {
      this.incoming = msg.payload;
      this.#emitNavBadges();
      setCallContext({ call_id: msg.payload?.call_id || "", queue_id: msg.payload?.queue_id || "" });
      reportEvent("call.ringing");
    } else if (msg.type === "call.ended") {
      const p = msg.payload || {};
      reportEvent("call.ended", { reason: p.reason || "", result: p.result || "", error_code: p.error_code || "" });
      if (this.call?.id === p.call_id || this.incoming?.call_id === p.call_id) {
        const suppressToast = this.#failureNotifiedCallId === p.call_id;
        this.#invalidateJoin();
        applyCallEnded(this, p, (meta) => {
          this.outboundNotice = "";
          this.#endLocal(p.call_id, meta);
        }, { suppressToast });
        if (suppressToast) this.#failureNotifiedCallId = "";
      }
    } else if (msg.type === "call.outbound_progress") {
      const p = msg.payload || {};
      if (this.call?.id !== p.call_id) return;
      this.outboundNotice = p.message || outboundProgressLabel(p.phase);
      if (p.phase === "failed") {
        this.#invalidateJoin(p.call_id);
        reportEvent("call.outbound_failed", { result: "failed", error_code: p.error_code || "", message: p.message || "" });
        notifyCallFailure(this, p);
      }
      if (p.phase === "connected" || p.phase === "playing") {
        if (p.phase === "connected" && this.call?.id === p.call_id) this.#startTimer();
      }
    } else if (msg.type === "command.failed" || msg.type === "recording.failed") {
      notifyCallFailure(this, msg.payload || {});
    } else if (msg.type === "leg.failed") {
      const p = msg.payload || {};
      if (this.call?.id === p.call_id) this.#invalidateJoin(p.call_id);
      notifyCallFailure(this, p);
    } else if (msg.type === "agent.routing_state_changed" && this.me) {
      this.me = { ...this.me, session: { ...this.me.session, state: msg.payload.state, busy_reason: msg.payload.busy_reason } };
    } else if (msg.type === "recording.notice") {
      this.feedback.liveNotice(msg.payload?.message || "");
    } else if (msg.type === "video.requested") {
      this.videoAsk = msg.payload;
    } else if (msg.type === "video.accepted") {
      this.feedback.liveNotice("对端已同意开启视频");
      this.media.rejoin(true);
    } else if (msg.type === "video.downgraded") {
      this.feedback.liveNotice("已降为语音");
      this.media.rejoin(false);
    } else if (msg.type === "call.consulting") {
      this.consulting = true;
      this.feedback.liveNotice("咨询转已接通，可完成转接或继续三方");
    } else if (msg.type === "call.transferred") {
      this.consulting = false;
      if (msg.payload?.from_agent_id === this.me?.id) {
        this.feedback.liveNotice("咨询转已完成，请填写小结");
        this.#endLocal(this.call?.id, { result: "answered" });
      } else {
        this.feedback.liveNotice("咨询转已完成，客户已接回");
      }
    } else if (msg.type === "call.answered" && msg.payload?.call_id) {
      void this.#syncCalls().then(async () => {
        if (this.call?.outbound_mode === "prompt_outbound") return;
        if (this.call?.state !== "active") return;
        this.outboundNotice = "";
        if (!this.media.pc && this.me?.terminal_type !== "sip") {
          const joinEpoch = this.#joinEpoch;
          try {
            await this.media.rejoin(this.call.session_type !== "audio");
          } catch (err) {
            if (!this.shouldSuppressJoinError(err, this.call?.id, joinEpoch)) {
              this.feedback.fail(err instanceof Error ? err.message : String(err));
            }
          }
        }
        this.#startTimer();
      });
    }
  }

  async #doCheckIn() {
    await this.#run(async () => {
      await this.#ensureCheckedIn(true);
    });
  }

  async #ensureCheckedIn(force = false) {
    const state = this.me?.session?.state;
    if (!force && state && state !== "offline") return this.me.session;
    const session = await checkIn(this.me.id, this.selectedQueues);
    this.me = { ...this.me, session };
    if (session.queue_ids?.length) this.selectedQueues = [...session.queue_ids];
    this.feedback.clear();
    return session;
  }

  async #doCheckOut() {
    await this.#run(async () => {
      await checkOut(this.me.id);
      this.me = { ...this.me, session: { ...this.me.session, state: "offline", queue_ids: [] } };
    });
  }

  async #toggleBusy() {
    const next = this.me.session?.state === "busy" ? "idle" : "busy";
    await this.#run(async () => {
      const sess = await setAgentState(this.me.id, next, next === "busy" ? this.busyReason || "break" : "");
      this.me = { ...this.me, session: sess };
    });
  }

  async #setIdle() {
    await this.#run(async () => {
      const sess = await setAgentState(this.me.id, "idle", "");
      this.me = { ...this.me, session: sess };
    });
  }

  /** 接听当前来电并为坐席通话腿建立 WebRTC 会话。 */
  async #answer() {
    if (this.media.connecting) return;
    await this.#run(async (epoch) => {
      if (this.me?.terminal_type === "sip") { this.feedback.ok("请在 SIP 话机接听", epoch); return; }
      const joinEpoch = ++this.#joinEpoch;
      const incomingId = this.incoming.call_id;
      await answerCall(incomingId);
      const call = await getCall(incomingId);
      if (joinEpoch !== this.#joinEpoch) return;
      this.call = call;
      this.incoming = null;
      this.nav = "desk";
      const video = call.session_type === "video" || call.session_type === "mixed";
      const leg = (call.legs || []).find((l) => l.agent_id === this.me?.id) || call.legs?.[1];
      if (!leg?.id) throw new Error("未找到坐席通话腿");
      setCallContext({ call_id: call.id, leg_id: leg.id, queue_id: call.queue_id || "" });
      reportEvent("call.answered");
      try {
        await this.media.start({
          callId: call.id,
          legId: leg.id,
          video,
          audioDeviceId: this.audioDeviceId,
          videoDeviceId: this.videoDeviceId,
          allowAudioOnlyForVideo: true,
        });
      } catch (err) {
        if (this.shouldSuppressJoinError(err, call.id, joinEpoch)) return;
        throw err;
      }
      if (joinEpoch !== this.#joinEpoch) return;
      this.feedback.clear();
      this.media.bindVideos();
      this.#startTimer();
    });
  }

  async #decline() {
    const id = this.incoming?.call_id;
    if (id) {
      try {
        await declineCall(id);
      } catch {
        this.#ws.send("call.decline", { call_id: id, reason: "busy" });
      }
    }
    this.incoming = null;
    this.#emitNavBadges();
  }

  async #hangup() {
    const id = this.call?.id;
    if (id) {
      try {
        await hangupCall(id);
      } catch (e) {
        this.feedback.fail(e instanceof Error ? e.message : String(e));
      }
    }
    this.#endLocal(id);
  }

  async #survey() {
    const id = this.call?.id;
    if (!id) return;
    await this.#run(async (epoch) => {
      const view = await startSurvey(id);
      if (view?.version != null) setCallVersion(view.version);
      this.#endLocal(id);
      this.feedback.ok("已转满意度调查", epoch);
    });
  }

  /** 记录最近通话并释放本地媒体与界面状态。 */
  #endLocal(endedId, endMeta = {}) {
    const wrapId = endedId || this.call?.id;
    const hadCall = !!this.call;
    const hadIncoming = !!this.incoming;
    const elapsed = this.elapsed;
    const resultKey = endMeta.result ?? this.call?.result;
    if (hadCall || hadIncoming) {
      this.recentCalls = [
        {
          time: formatDateTime(),
          caller: this.call?.caller || this.incoming?.caller || this.dest || "—",
          queue: this.call?.queue_name || this.incoming?.queue_name || "—",
          result: recentCallResultLabel({
            result: resultKey,
            elapsed,
            hadActiveCall: hadCall,
            hadIncoming,
          }),
          duration: elapsed,
        },
        ...this.recentCalls,
      ].slice(0, 20);
    }
    this.#stopTimer();
    this.media.stop();
    this.cameraUnavailable = false;
    this.audioPlaybackBlocked = false;
    this.held = false;
    this.audioMuted = false;
    this.videoMuted = false;
    this.sharing = false;
    this.videoAsk = null;
    this.call = null;
    this.incoming = null;
    if (endedId && this.#failureNotifiedCallId === endedId) this.#failureNotifiedCallId = "";
    this.elapsed = 0;
    this.consulting = false;
    this.showPad = false;
    if (wrapId && shouldPromptWrapUp({ result: resultKey, elapsed })) {
      this.pendingWrapId = wrapId;
      setCallContext({ call_id: wrapId, leg_id: "" });
    } else {
      clearCallContext();
    }
  }

  async #loadIvrAssets() {
    try {
      const data = await listIvrAssets();
      this.ivrAssets = data?.items || [];
    } catch (err) {
      this.ivrAssets = [];
      const msg = err instanceof Error ? err.message : String(err);
      if (msg && !msg.includes("CANCELED")) this.feedback.fail(`无法加载语音素材：${msg}`);
    }
  }

  async #voiceNotify() {
    await this.#run(async (epoch) => {
      const dest = this.dest.trim();
      const asset = (this.promptAssetId || "").trim();
      if (!dest) {
        this.feedback.fail("请输入目标号码", epoch);
        return;
      }
      if (!asset) {
        this.feedback.fail("请选择语音素材", epoch);
        return;
      }
      if (this.voiceNotifyBusy) return;
      this.voiceNotifyBusy = true;
      try {
        await this.#ensureCheckedIn();
        if (this.me?.session?.state === "busy") {
          const sess = await setAgentState(this.me.id, "idle", "");
          this.me = { ...this.me, session: sess };
        }
        if (this.me?.terminal_type === "sip") {
          this.feedback.ok("请从已注册的 SIP 话机操作", epoch);
          return;
        }
        const view = await voiceNotificationCall(dest, asset);
        this.outboundNotice = "已提交语音通知，关闭页面不影响播放";
        reportEvent("call.voice_notification_created", { call_id: view?.id || "", state: view?.state });
        this.feedback.ok("语音通知已提交", epoch);
      } finally {
        this.voiceNotifyBusy = false;
      }
    });
  }

  async #dial() {
    await this.#run(async (epoch) => {
      if (!this.dest.trim()) {
        this.feedback.fail("请输入目标号码", epoch);
        return;
      }
      await this.#ensureCheckedIn();
      if (this.me?.session?.state === "busy") {
        const sess = await setAgentState(this.me.id, "idle", "");
        this.me = { ...this.me, session: sess };
      }
      if (this.me?.terminal_type === "sip") { this.feedback.ok("已自动签入，请从已注册的 SIP 话机拨号", epoch); return; }
      const joinEpoch = ++this.#joinEpoch;
      this.#failureNotifiedCallId = "";
      let call;
      try {
        call = await outboundCall(this.dest.trim());
      } catch (err) {
        this.#joinEpoch += 1;
        throw err;
      }
      if (joinEpoch !== this.#joinEpoch) return;
      this.call = call;
      this.nav = "desk";
      this.feedback.clear();
      const leg = (call.legs || []).find((l) => l.agent_id === this.me?.id);
      setCallContext({ call_id: call.id, leg_id: leg?.id || "", queue_id: call.queue_id || "" });
      reportEvent("call.outbound_created", { state: call.state });
      this.outboundNotice = call.pstn_dial_state === "dialing" ? "正在出局拨号…" : "";
      if (leg && (call.state === "active" || call.state === "ringing")) {
        try {
          await this.#joinOutboundMedia(call, leg, joinEpoch);
        } catch (err) {
          if (this.shouldSuppressJoinError(err, call.id, joinEpoch)) return;
          throw err;
        }
        if (joinEpoch !== this.#joinEpoch) return;
        if (call.state === "active") {
          this.media.bindVideos();
          this.#startTimer();
        }
      }
    });
  }

  async #joinOutboundMedia(call, leg, joinEpoch) {
    if (joinEpoch !== this.#joinEpoch) return;
    await this.media.start({
      callId: call.id,
      legId: leg.id,
      video: false,
      audioDeviceId: this.audioDeviceId,
    });
  }

  async #hold() {
    this.held = !this.held;
    await this.#run(async () => {
      try {
        await holdCall(this.call.id, this.held);
      } catch (e) {
        this.held = !this.held;
        throw e;
      }
    });
  }

  /** 根据页面选择的目标和模式发起转接。 */
  async #xfer() {
    await this.#run(async (epoch) => {
      const target = this.agents.find((a) => a.extension === this.dest);
      const mode = this.xferMode || "blind";
      await transferCall(
        this.call.id,
        target ? { mode, target_agent_id: target.id } : { mode: "blind", target_queue_id: this.dest },
      );
      this.consulting = mode === "consult";
      this.feedback.ok(mode === "consult" ? "咨询转振铃中，客户已保持" : "已盲转", epoch);
    });
  }

  async #completeXfer() {
    await this.#run(async (epoch) => {
      await completeTransfer(this.call.id);
      this.consulting = false;
      this.feedback.ok("咨询转已完成", epoch);
    });
  }

  async #askVideo() {
    await this.#run(async () => {
      await requestVideo(this.call.id);
    });
  }

  async #downgrade() {
    await this.#run(async () => {
      await downgradeVideo(this.call.id);
    });
  }

  async #conf() {
    await this.#run(async (epoch) => {
      const target = this.agents.find((a) => a.extension === this.dest);
      if (!target) throw new Error("请先填写对方分机");
      await conferenceInvite(this.call.id, target.id);
      this.feedback.ok("已邀请第三人，对方振铃中", epoch);
    });
  }

  async #listen() {
    await this.#run(async () => {
      const joinEpoch = ++this.#joinEpoch;
      const id = this.incoming?.call_id || this.dest;
      const out = await listenCall(id);
      if (joinEpoch !== this.#joinEpoch) return;
      setCallContext({ call_id: out.call_id, leg_id: out.leg_id });
      reportEvent("call.listen_started");
      this.call = await getCall(out.call_id);
      this.nav = "desk";
      try {
        await this.media.start({
          callId: out.call_id,
          legId: out.leg_id,
          video: false,
          recvOnly: true,
        });
      } catch (err) {
        if (this.shouldSuppressJoinError(err, out.call_id, joinEpoch)) return;
        throw err;
      }
      if (joinEpoch !== this.#joinEpoch) return;
      this.media.bindVideos();
      this.#startTimer();
    });
  }

  async #dtmf(d) {
    const leg = (this.call?.legs || []).find((l) => l.agent_id === this.me?.id);
    await sendDtmf(this.call.id, leg?.id, d);
  }

  async #submitWrap() {
    await this.#run(async (epoch) => {
      const id = this.call?.id || this.pendingWrapId;
      await wrapUp(id, this.wrapNotes);
      try {
        const sess = await setAgentState(this.me.id, "idle", "wrap-up");
        this.me = { ...this.me, session: sess };
      } catch {
        /* 通话中提交小结时状态仍为 on_call */
      }
      this.feedback.ok("小结已提交，已示闲", epoch);
      this.pendingWrapId = "";
      this.wrapNotes = "";
      clearCallContext();
    });
  }

  async #makeLink() {
    await this.#run(async () => {
      const qid = this.selectedQueues[0] || this.queues[0]?.id;
      const s = await createGuestSession(qid, 3600, "audio");
      this.guestLink = new URL(s.guest_url, location.origin).href;
      this.guestExpiresAt = s.expires_at || "";
      this.feedback.clear();
    });
  }

  async #copyGuestLink() {
    if (!this.guestLink) return;
    await this.#run(async (epoch) => {
      try {
        await navigator.clipboard.writeText(this.guestLink);
        this.feedback.ok("访客链接已复制", epoch);
      } catch {
        this.feedback.fail("复制失败，请手动复制链接", epoch);
      }
    });
  }

  #startTimer() {
    this.#stopTimer();
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
    await this.#run(async () => {
      try { await logout(); } finally {
        clearAccessToken();
        this.#wsUnsub?.();
        this.#wsUnsub = null;
        this.#ws.disconnect();
        this.#endLocal();
        this.pendingWrapId = "";
        clearCallContext();
        this.nav = "desk";
        this.me = null;
        this.dispatchEvent(new CustomEvent("session-ended", { bubbles: true, composed: true }));
      }
    });
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

  #navigate(id) {
    if (this.contentOnly) {
      this.dispatchEvent(new CustomEvent("staff-navigate", { detail: { id }, bubbles: true, composed: true }));
      return;
    }
    this.feedback.begin();
    this.nav = id;
    if (id === "outbound") void this.#loadIvrAssets();
    if (id === "desk" && this.media.pc) this.media.bindVideos();
  }

  #respondVideo(accept) {
    if (this.call?.id) this.#ws.send("video.respond", { call_id: this.call.id, accept });
  }

  render() {
    return renderApp(this, {
      answer: (...args) => this.#answer(...args),
      askVideo: (...args) => this.#askVideo(...args),
      bindVideos: (...args) => this.media.bindVideos(...args),
      completeXfer: (...args) => this.#completeXfer(...args),
      conf: (...args) => this.#conf(...args),
      copyGuestLink: (...args) => this.#copyGuestLink(...args),
      crumb: (...args) => this.#crumb(...args),
      decline: (...args) => this.#decline(...args),
      dial: (...args) => this.#dial(...args),
      doCheckIn: (...args) => this.#doCheckIn(...args),
      doCheckOut: (...args) => this.#doCheckOut(...args),
      downgrade: (...args) => this.#downgrade(...args),
      dtmf: (...args) => this.#dtmf(...args),
      hangup: (...args) => this.#hangup(...args),
      survey: (...args) => this.#survey(...args),
      hold: (...args) => this.#hold(...args),
      listen: (...args) => this.#listen(...args),
      login: (...args) => this.#login(...args),
      startSSO: () => startSSO(),
      logout: (...args) => this.#logout(...args),
      makeLink: (...args) => this.#makeLink(...args),
      navigate: (...args) => this.#navigate(...args),
      onCamChange: (...args) => this.media.onCamChange(...args),
      onMicChange: (...args) => this.media.onMicChange(...args),
      onSpeakerChange: (...args) => this.media.onSpeakerChange(...args),
      playRemoteAudio: (...args) => this.media.playRemoteAudio(...args),
      preview: (...args) => this.media.preview(...args),
      respondVideo: (...args) => this.#respondVideo(...args),
      setBusyReason: (value) => { this.busyReason = value; },
      setDest: (value) => { this.dest = value; },
      setPromptAssetId: (value) => { this.promptAssetId = value; },
      voiceNotify: (...args) => this.#voiceNotify(...args),
      setUsername: (value) => { this.username = value; },
      setPassword: (value) => { this.password = value; },
      setGuestMedia: (value) => { this.guestMedia = value; },
      setIdle: (...args) => this.#setIdle(...args),
      setSelectedQueues: (value) => { this.selectedQueues = value; },
      setShowPad: (value) => { this.showPad = value; },
      setWrapNotes: (value) => { this.wrapNotes = value; },
      setXferMode: (value) => { this.xferMode = value; },
      share: (...args) => this.media.share(...args),
      stageCaller: (...args) => this.#stageCaller(...args),
      stageQueue: (...args) => this.#stageQueue(...args),
      submitWrap: (...args) => this.#submitWrap(...args),
      toggleBusy: (...args) => this.#toggleBusy(...args),
      toggleMute: (...args) => this.media.toggleMute(...args),
      waitingCount: (...args) => this.#waitingCount(...args),
      xfer: (...args) => this.#xfer(...args)
    });
  }
}

customElements.define("open-voip-agent-app", AgentApp);
