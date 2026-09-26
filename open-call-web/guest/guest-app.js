import { NAV, renderApp } from "./views/shell.js";
import { bindApiFeedback } from "../shared/http-client.js";
import { beginTrace, clearCallContext, setCallContext } from "../shared/call-context.js";
import { LitElement } from "lit";
import { guestJoin, guestJoinToken, hangupCall, listGuestQueues, respondVideo, sendDtmf } from "../shared/api.js";
import { setAccessToken, clearAccessToken } from "../shared/auth-store.js";
import { parseGuestInvite } from "../shared/guest-invite.js";
import { appStyles } from "../shared/styles/index.js";
import { listMediaDevices, microphoneConstraints, replaceInputDevice, setLocalMuted, startMediaSession, stopMedia } from "../shared/webrtc.js";
import { BusinessWebSocket } from "../shared/ws.js";
import { reportEvent } from "../shared/observability.js";


export class GuestApp extends LitElement {
  static properties = {
    error: { type: String },
    queues: { type: Array },
    step: { type: String },
    permissionHint: { type: String },
    join: { type: Object },
    elapsed: { type: Number },
    audioMuted: { type: Boolean },
    audioPlaybackBlocked: { type: Boolean },
    videoMuted: { type: Boolean },
    wantVideo: { type: Boolean },
    videoAsk: { type: Object },
    notice: { type: String },
    selected: { type: Object },
    waitSec: { type: Number },
    position: { type: Number },
    nav: { type: String },
    inviteToken: { type: String },
    inviteMedia: { type: String },
    videoDeviceId: { type: String },
    userId: { type: String },
  };

  static styles = appStyles;

  #ws = new BusinessWebSocket();
  #pc = null;
  #local = null;
  #remote = null;
  #remoteAudio = null;
  #waiting = null;
  #mediaPromise = null;
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
    this.audioPlaybackBlocked = false;
    this.videoMuted = false;
    this.wantVideo = false;
    this.videoAsk = null;
    this.notice = "";
    this.selected = null;
    this.waitSec = 0;
    this.position = 0;
    this.nav = "service";
    this.inviteToken = "";
    this.inviteMedia = "audio";
    this.videoDeviceId = "";
    this.userId = "";
  }

  connectedCallback() {
    super.connectedCallback();
    beginTrace({ role: "guest", agent_id: "" });
    this._unbindApi = bindApiFeedback(this, () => { this.#cleanup(); this.join = null; this.nav = "service"; });
    const { token, media } = parseGuestInvite(location.search);
    if (token) {
      this.inviteToken = token;
      this.inviteMedia = media;
      this.wantVideo = this.inviteMedia === "video";
      this.step = "invite";
      return;
    }
    listGuestQueues()
      .then((r) => {
        this.queues = r.items || [];
        if (this.queues[0]) this.selected = { queue: this.queues[0], video: false, vip: false };
      })
      .catch((e) => {
        this.error = e instanceof Error ? e.message : String(e);
      });
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unbindApi?.();
    this.#cleanup();
  }

  #pick(queue, video, vip = false) {
    this.selected = { queue, video, vip };
  }

  /** 检查媒体权限后加入队列，并连接业务事件和等待音频。 */
  async #start(queue, video, vip = false) {
    reportEvent("call.starting", { kind: video ? "video" : "audio" });
    this.error = "";
    const userId = this.userId.trim();
    if (video && !userId) {
      this.error = "发起视频通话前，请填写您的用户标识。";
      return;
    }
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
      const stream = await navigator.mediaDevices.getUserMedia({ audio: microphoneConstraints(), video });
      stream.getTracks().forEach((t) => t.stop());
    } catch {
      this.error = video ? "摄像头/麦克风权限被拒绝，可改选语音服务。" : "麦克风权限被拒绝，请在浏览器设置中允许后重试。";
      this.step = "pick";
      this.nav = "service";
      clearInterval(this.#waitTimer);
      return;
    }
    try {
      const join = await guestJoin(queue.id, video ? "video" : "audio", vip ? 10 : 0, userId);
      this.join = join;
      setCallContext({ call_id: join.call_id, leg_id: join.leg_id, queue_id: queue.id });
      reportEvent("call.joined", { state: join.state });
      setAccessToken(join.token);
      this.step = "wait";
      this.#ws.connect(join.token);
      this.#ws.subscribe((msg) => this.#onWs(msg));
      if (["ivr", "queued", "ringing"].includes(join.state)) await this.#enterMedia(false);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      this.step = "pick";
      this.nav = "service";
      clearInterval(this.#waitTimer);
    }
  }

  /** 根据接通、排队、IVR 与挂断事件更新访客页面状态。 */
  async #onWs(msg) {
    if (msg.type === "call.answered") {
      reportEvent("call.answered");
      await this.#enterMedia(true);
    } else if (msg.type === "call.ended") {
      reportEvent("call.ended", { reason: msg.payload?.reason || "" });
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

  async #startToken() {
    this.error = "";
    this.permissionHint = this.wantVideo ? "正在申请摄像头和麦克风权限…" : "正在申请麦克风权限…";
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: microphoneConstraints(), video: this.wantVideo });
      stream.getTracks().forEach((track) => track.stop());
      await this.#joinToken(this.inviteToken, this.inviteMedia);
    } catch (e) {
      this.error = e?.name === "NotAllowedError"
        ? "摄像头或麦克风权限被拒绝，请在浏览器设置中允许后重试。"
        : e instanceof Error ? e.message : String(e);
      this.step = "invite";
    }
  }

  /** 使用邀请令牌入会，随后接收当前通话的事件和媒体。 */
  async #joinToken(token, media = "") {
    this.step = "wait";
    this.nav = "queue";
    try {
      const join = await guestJoinToken(token, media);
      this.join = join;
      setCallContext({ call_id: join.call_id, leg_id: join.leg_id, queue_id: join.queue_id || "" });
      reportEvent("call.joined", { state: join.state });
      setAccessToken(join.token);
      history.replaceState(null, "", location.pathname);
      this.#ws.connect(join.token);
      this.#ws.subscribe((msg) => this.#onWs(msg));
      if (["ivr", "queued", "ringing"].includes(join.state)) await this.#enterMedia(false);
      if (join.state === "active") await this.#enterMedia(true);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      this.step = "pick";
      this.nav = "service";
    }
  }

  /** 等候阶段播放远端音频，接通后绑定通话媒体和计时器。 */
  async #enterMedia(talking) {
    try {
      if (!this.#pc) {
        if (!this.#mediaPromise) {
          this.#mediaPromise = startMediaSession({
            callId: this.join.call_id,
            legId: this.join.leg_id,
            video: this.wantVideo,
            videoDeviceId: this.videoDeviceId,
          });
        }
        const session = await this.#mediaPromise;
        this.#pc = session.pc;
        this.#local = session.localStream;
        this.#remote = session.remoteStream;
        this.#remoteAudio = session.remoteAudioStream;
        this.#waiting = session.waitingStream;
      }
      await this.updateComplete;
      if (!talking) {
        if (this.step === "talk") return;
        const queueAudio = this.renderRoot.querySelector("#queue-audio");
        if (queueAudio) {
          queueAudio.srcObject = this.#waiting;
          void this.#playRemoteAudio();
        }
        return;
      }
      const queueAudio = this.renderRoot.querySelector("#queue-audio");
      if (queueAudio) {
        queueAudio.pause();
        queueAudio.srcObject = null;
      }
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
      const remoteAudio = this.renderRoot.querySelector("#remote-audio");
      if (localV) localV.srcObject = this.#local;
      if (remoteV) {
        remoteV.srcObject = this.#remote;
        void remoteV.play().catch(() => {});
      }
      if (remoteAudio) {
        remoteAudio.srcObject = this.#remoteAudio;
        void this.#playRemoteAudio();
      }
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    } finally {
      this.#mediaPromise = null;
    }
  }

  async #playRemoteAudio() {
    const audio = this.renderRoot.querySelector(this.step === "talk" ? "#remote-audio" : "#queue-audio");
    if (!audio) return;
    try {
      await audio.play();
      this.audioPlaybackBlocked = false;
    } catch {
      this.audioPlaybackBlocked = true;
    }
  }

  async #rejoin(video) {
    this.wantVideo = video;
    stopMedia(this.#pc, this.#local);
    this.#pc = null;
    this.#local = null;
    this.#remote = null;
    this.#remoteAudio = null;
    this.#waiting = null;
    this.#mediaPromise = null;
    this.audioPlaybackBlocked = false;
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
      /* 忽略该异常，继续执行后续操作。 */
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

  async #switchCamera() {
    if (!this.#pc || !this.wantVideo) return;
    try {
      const { videoInputs } = await listMediaDevices();
      if (videoInputs.length < 2) {
        this.notice = "当前仅检测到一个摄像头";
        return;
      }
      const index = videoInputs.findIndex((item) => item.deviceId === this.videoDeviceId);
      const next = videoInputs[(index + 1) % videoInputs.length];
      await replaceInputDevice(this.#pc, this.#local, "video", next.deviceId);
      this.videoDeviceId = next.deviceId;
      this.notice = "摄像头已切换";
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    }
  }

  /** 清理访客连接、媒体和计时器，供挂断与异常路径共用。 */
  #cleanup() {
    clearInterval(this.#timer);
    clearInterval(this.#waitTimer);
    this.#ws.disconnect();
    stopMedia(this.#pc, this.#local);
    this.#pc = null;
    this.#local = null;
    this.#remote = null;
    this.#remoteAudio = null;
    this.#waiting = null;
    this.#mediaPromise = null;
    this.audioPlaybackBlocked = false;
    clearAccessToken();
    clearCallContext();
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
        const remoteAudio = this.renderRoot.querySelector("#remote-audio");
        if (localV) localV.srcObject = this.#local;
        if (remoteV) remoteV.srcObject = this.#remote;
        if (remoteAudio) {
          remoteAudio.srcObject = this.#remoteAudio;
          void this.#playRemoteAudio();
        }
      });
    }
  }

  async #respondVideo(accept) {
    if (!this.join?.call_id) return;
    try {
      await respondVideo(this.join.call_id, accept);
      this.videoAsk = null;
    } catch (error) {
      this.error = error instanceof Error ? error.message : String(error);
    }
  }

  render() {
    return renderApp(this, {
      crumb: (...args) => this.#crumb(...args),
      dtmf: (...args) => this.#dtmf(...args),
      go: (...args) => this.#go(...args),
      hangup: (...args) => this.#hangup(...args),
      pick: (...args) => this.#pick(...args),
      playRemoteAudio: (...args) => this.#playRemoteAudio(...args),
      respondVideo: (...args) => this.#respondVideo(...args),
      restart: () => location.reload(),
      setUserId: (value) => { this.userId = value; },
      start: (...args) => this.#start(...args),
      startToken: (...args) => this.#startToken(...args),
      switchCamera: (...args) => this.#switchCamera(...args),
      toggle: (...args) => this.#toggle(...args)
    });
  }
}

customElements.define("open-voip-guest-app", GuestApp);
