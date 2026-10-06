// 坐席 WebRTC 媒体会话：与通话业务 API 解耦。
import { applyAudioOutput, replaceInputDevice, requireMediaDevices, setLocalMuted, startMediaSession, startScreenShare, stopMedia } from "../../shared/webrtc.js";
import { screenShare } from "../../shared/api.js";
import { runFeedbackAction } from "../../shared/feedback.js";

export class AgentMediaController {
  #host;
  #pc = null;
  #local = null;
  #remote = null;
  #remoteAudio = null;
  #connecting = false;

  constructor(host) {
    this.#host = host;
    host.addController(this);
  }

  get pc() { return this.#pc; }
  get local() { return this.#local; }
  get remote() { return this.#remote; }
  get remoteAudio() { return this.#remoteAudio; }
  get connecting() { return this.#connecting; }
  get hasLocal() { return !!this.#local; }

  hostDisconnected() {
    this.stop();
  }

  stop() {
    stopMedia(this.#pc, this.#local);
    this.#pc = null;
    this.#local = null;
    this.#remote = null;
    this.#remoteAudio = null;
    this.#connecting = false;
    this.#host.sharing = false;
  }

  // 与 switch 信令交换 SDP，建立 PC；重复 start 会先 tear down 旧连接，避免双 PC。
  async start(opts) {
    if (this.#connecting) return null;
    this.#connecting = true;
    stopMedia(this.#pc, this.#local);
    try {
      const session = await startMediaSession(opts);
      this.#pc = session.pc;
      this.#local = session.localStream;
      this.#remote = session.remoteStream;
      this.#remoteAudio = session.remoteAudioStream;
      this.#host.cameraUnavailable = session.cameraUnavailable;
      this.bindVideos();
      return session;
    } finally {
      this.#connecting = false;
    }
  }

  // 会话类型切换（纯音↔视频/屏幕共享）或 ICE 失败后按当前 call/leg 重新入会。
  async rejoin(video) {
    const host = this.#host;
    if (!host.call || host.me?.terminal_type === "sip" || this.#connecting) return;
    const leg = (host.call.legs || []).find((l) => l.agent_id === host.me?.id) || host.call.legs?.[1];
    if (!leg) return;
    const joinEpoch = host.getJoinEpoch?.() ?? 0;
    const callId = host.call.id;
    try {
      await this.start({
        callId,
        legId: leg.id,
        video,
        audioDeviceId: host.audioDeviceId,
        videoDeviceId: host.videoDeviceId,
        allowAudioOnlyForVideo: true,
      });
      host.feedback.clear();
      host.call = { ...host.call, session_type: video ? "mixed" : "audio" };
    } catch (err) {
      if (host.shouldSuppressJoinError?.(err, callId, joinEpoch)) return;
      host.feedback.fail(err instanceof Error ? err.message : String(err));
    }
  }

  async toggleMute(kind) {
    const host = this.#host;
    if (kind === "audio") host.audioMuted = !host.audioMuted;
    else host.videoMuted = !host.videoMuted;
    const leg = (host.call?.legs || []).find((l) => l.agent_id === host.me?.id);
    await setLocalMuted(this.#local, host.call?.id, leg?.id, {
      audio: host.audioMuted,
      video: host.videoMuted,
    });
  }

  async share() {
    const host = this.#host;
    await runFeedbackAction(host.feedback, async () => {
      if (!this.#pc?.getSenders().some((s) => s.track?.kind === "video")) {
        await this.rejoin(true);
      }
      const stream = await startScreenShare(this.#pc);
      const leg = (host.call?.legs || []).find((l) => l.agent_id === host.me?.id);
      await screenShare(host.call.id, true, leg?.id);
      host.sharing = true;
      stream.getVideoTracks()[0].onended = () => {
        host.sharing = false;
        screenShare(host.call.id, false, leg?.id);
      };
    });
  }

  async preview() {
    const host = this.#host;
    await runFeedbackAction(host.feedback, async () => {
      const stream = await requireMediaDevices().getUserMedia({
        audio: false,
        video: host.videoDeviceId ? { deviceId: { exact: host.videoDeviceId } } : true,
      });
      this.#local = stream;
      this.bindVideos();
    });
  }

  bindVideos() {
    const host = this.#host;
    host.updateComplete.then(() => {
      const localV = host.renderRoot.querySelector("#local");
      const remoteV = host.renderRoot.querySelector("#remote");
      const remoteAudio = host.renderRoot.querySelector("#remote-audio");
      if (localV) localV.srcObject = this.#local;
      if (remoteV) {
        remoteV.srcObject = this.#remote;
        void remoteV.play().catch(() => {});
      }
      if (remoteAudio) {
        remoteAudio.srcObject = this.#remoteAudio;
        applyAudioOutput(remoteAudio, host.speakerDeviceId).catch(() => {});
        void this.playRemoteAudio();
      }
    });
  }

  async playRemoteAudio() {
    const host = this.#host;
    const el = host.renderRoot.querySelector("#remote-audio");
    if (!el) return;
    try {
      await el.play();
      host.audioPlaybackBlocked = false;
    } catch {
      host.audioPlaybackBlocked = true;
    }
  }

  async onMicChange(id) {
    this.#host.audioDeviceId = id;
    if (this.#pc) await replaceInputDevice(this.#pc, this.#local, "audio", id);
  }

  async onCamChange(id) {
    this.#host.videoDeviceId = id;
    if (this.#pc) {
      if (!this.#local?.getVideoTracks().length) {
        await this.rejoin(true);
        return;
      }
      await replaceInputDevice(this.#pc, this.#local, "video", id);
      this.#host.cameraUnavailable = false;
      this.bindVideos();
    }
  }

  async onSpeakerChange(id) {
    this.#host.speakerDeviceId = id;
    const el = this.#host.renderRoot.querySelector("#remote-audio");
    if (el) await applyAudioOutput(el, id);
  }
}
