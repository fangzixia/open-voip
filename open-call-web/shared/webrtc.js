/**
 * WebRTC 信令与 PeerConnection 封装：Offer/Answer/ICE 走 REST。
 */

import { getRuntimeConfig } from "./runtime-config.js";
import { fetchTurn, postAnswer, postIce, postMute, postOffer } from "./api.js";
import { setCallContext } from "./call-context.js";
import { reportEvent } from "./observability.js";
import { startWebRTCStats, stopWebRTCStats } from "./webrtc-stats.js";

/** @returns {RTCConfiguration} */
export function buildRtcConfiguration(iceServersFromApi) {
  const { publicUrl } = getRuntimeConfig();
  const servers = iceServersFromApi ?? [];
  return {
    iceServers: servers,
    bundlePolicy: "max-bundle",
    ...(publicUrl ? {} : {}),
  };
}

/** 按后端下发的 ICE 配置创建浏览器媒体连接。 */
export function createPeerConnection(configuration) {
  return new RTCPeerConnection(configuration);
}

export function isWebRTCSupported() {
  return typeof RTCPeerConnection !== "undefined";
}

/** 将可用设备按输入音频、输入视频和输出音频分组。 */
export async function listMediaDevices() {
  const devices = await navigator.mediaDevices.enumerateDevices();
  return {
    audioInputs: devices.filter((d) => d.kind === "audioinput"),
    videoInputs: devices.filter((d) => d.kind === "videoinput"),
    audioOutputs: devices.filter((d) => d.kind === "audiooutput"),
  };
}

function isMissingDevice(error) {
  return error?.name === "NotFoundError" || error?.name === "OverconstrainedError";
}

/** 通话麦克风统一请求浏览器的回声消除、降噪和自动增益。 */
export function microphoneConstraints(deviceId = "") {
  return {
    ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
    echoCancellation: true,
    noiseSuppression: true,
    autoGainControl: true,
  };
}

/** 坐席摄像头不可用时保留麦克风；远端视频仍由服务端 Offer 接收。 */
export async function acquireAgentMedia({ audioDeviceId, videoDeviceId }) {
  let audioStream;
  try {
    audioStream = await navigator.mediaDevices.getUserMedia({
      audio: microphoneConstraints(audioDeviceId),
      video: false,
    });
  } catch (error) {
    if (!audioDeviceId || !isMissingDevice(error)) throw error;
    audioStream = await navigator.mediaDevices.getUserMedia({ audio: microphoneConstraints(), video: false });
  }

  try {
    let videoStream;
    try {
      videoStream = await navigator.mediaDevices.getUserMedia({
        audio: false,
        video: videoDeviceId ? { deviceId: { exact: videoDeviceId } } : true,
      });
    } catch (error) {
      if (!videoDeviceId || !isMissingDevice(error)) throw error;
      videoStream = await navigator.mediaDevices.getUserMedia({ audio: false, video: true });
    }
    for (const track of videoStream.getVideoTracks()) audioStream.addTrack(track);
    return { stream: audioStream, cameraUnavailable: videoStream.getVideoTracks().length === 0 };
  } catch (error) {
    return { stream: audioStream, cameraUnavailable: true, cameraError: error?.name || "Error" };
  }
}

/**
 * 建立与 SFU 的 WebRTC 会话（服务端 Offer）。
 * @returns {Promise<{pc: RTCPeerConnection, localStream: MediaStream, remoteStream: MediaStream, remoteAudioStream: MediaStream, waitingStream: MediaStream}>}
 */
export async function startMediaSession({ callId, legId, video, audioDeviceId, videoDeviceId, recvOnly, allowAudioOnlyForVideo = false }) {
  setCallContext({ call_id: callId, leg_id: legId });
  let localStream;
  let cameraUnavailable = false;
  try {
    reportEvent("webrtc.permission", { phase: "request", kind: video ? "audio_video" : "audio" });
    if (recvOnly) {
      localStream = new MediaStream();
    } else if (video && allowAudioOnlyForVideo) {
      const acquired = await acquireAgentMedia({ audioDeviceId, videoDeviceId });
      localStream = acquired.stream;
      cameraUnavailable = acquired.cameraUnavailable;
      if (cameraUnavailable) reportEvent("webrtc.permission", { phase: "camera_unavailable", error_name: acquired.cameraError || "NotFoundError" });
    } else {
      localStream = await navigator.mediaDevices.getUserMedia({
          audio: microphoneConstraints(audioDeviceId),
          video: video ? (videoDeviceId ? { deviceId: { exact: videoDeviceId } } : true) : false,
        });
    }
    reportEvent("webrtc.permission", { phase: recvOnly ? "not_required" : "granted", kind: video ? "audio_video" : "audio" });
  } catch (error) {
    reportEvent("webrtc.permission", { phase: "denied", error_name: error?.name || "Error" });
    throw error;
  }
  // TURN 获取失败时继续尝试直连，后续协商失败由统一清理路径处理。
  let iceServers = [];
  try {
    reportEvent("webrtc.turn", { phase: "request" });
    const turn = await fetchTurn(callId);
 if (turn?.stun_urls?.length) iceServers.push({ urls: turn.stun_urls });
    if (turn?.urls?.length) {
      iceServers = iceServers.concat({
        urls: turn.urls,
        username: turn.username,
        credential: turn.credential,
      });
    }
    reportEvent("webrtc.turn", { phase: "complete", transport: turn?.urls?.length ? "turn" : "stun" });
  } catch (error) {
    reportEvent("webrtc.turn", { phase: "failed", error_name: error?.name || "Error" });
  }
  const pc = createPeerConnection(buildRtcConfiguration(iceServers));
  reportEvent("webrtc.pc", { phase: "created" });
  pc.addEventListener("connectionstatechange", () => {
    reportEvent("webrtc.pc_state", { peer_connection_state: pc.connectionState });
  });
  pc.addEventListener("iceconnectionstatechange", () => {
    reportEvent("webrtc.ice_state", { ice_connection_state: pc.iceConnectionState });
  });
  pc.addEventListener("icegatheringstatechange", () => {
    reportEvent("webrtc.ice_gathering_state", { ice_gathering_state: pc.iceGatheringState });
  });
  startWebRTCStats(pc, (details) => reportEvent(details.phase ? "webrtc.stats_failed" : "webrtc.stats", details));
  const remoteStream = new MediaStream();
  const remoteAudioStream = new MediaStream();
  const waitingStream = new MediaStream();
  pc.ontrack = (ev) => {
    addRemoteTrack(remoteStream, waitingStream, ev.track, ev.streams, remoteAudioStream);
  };
  localStream.getTracks().forEach((t) => pc.addTrack(t, localStream));
  // 浏览器逐个上报 ICE candidate，服务端负责补入同一通话腿。
  pc.onicecandidate = (ev) => {
    if (!ev.candidate) {
      reportEvent("webrtc.ice", { phase: "gathering_complete" });
      return;
    }
    reportEvent("webrtc.ice", { phase: "candidate_generated" });
    postIce(callId, legId, {
      candidate: ev.candidate.candidate,
      sdp_mid: ev.candidate.sdpMid,
      sdp_mline_index: ev.candidate.sdpMLineIndex,
    }).then(
      () => reportEvent("webrtc.ice", { phase: "posted" }),
      (error) => {
        reportEvent("webrtc.ice", { phase: "post_failed", error_name: error?.name || "Error" });
        console.error("ICE candidate upload failed", error);
        pc.dispatchEvent(new CustomEvent("iceposterror", { detail: error }));
      },
    );
  };
  try {
    reportEvent("webrtc.offer", { phase: "request" });
    const offer = await postOffer(callId, legId);
    reportEvent("webrtc.offer", { phase: "received" });
    await pc.setRemoteDescription({ type: "offer", sdp: offer.sdp });
    const answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);
    reportEvent("webrtc.answer", { phase: "created" });
    await postAnswer(callId, legId, answer.sdp);
    reportEvent("webrtc.answer", { phase: "posted" });
    return { pc, localStream, remoteStream, remoteAudioStream, waitingStream, cameraUnavailable };
  } catch (error) {
    reportEvent("webrtc.negotiation", { phase: "failed", error_name: error?.name || "Error" });
    stopMedia(pc, localStream);
    throw error;
  }
}

/** SFU 将排队提示音和双向通话音频作为独立轨道下发，分别交给对应播放器。 */
export function addRemoteTrack(remoteStream, waitingStream, track, streams = [], remoteAudioStream) {
  const waiting = track.kind === "audio" &&
    (track.id === "moh" || streams.some((stream) => stream.id?.startsWith("sfu-moh-")));
  const target = waiting ? waitingStream : remoteStream;
  if (!target.getTracks().some((existing) => existing.id === track.id)) target.addTrack(track);
  if (!waiting && track.kind === "audio" && remoteAudioStream &&
      !remoteAudioStream.getTracks().some((existing) => existing.id === track.id)) {
    remoteAudioStream.addTrack(track);
  }
}

/** 同步切换本地轨道和服务端的静音状态。 */
export async function setLocalMuted(localStream, callId, legId, { audio, video }) {
  localStream?.getAudioTracks().forEach((t) => {
    t.enabled = !audio;
  });
  localStream?.getVideoTracks().forEach((t) => {
    t.enabled = !video;
  });
  if (callId && legId) {
    await postMute(callId, legId, !!audio, !!video);
  }
}

/** 停止统计与本地轨道，并关闭 PeerConnection。 */
export function stopMedia(pc, localStream) {
  stopWebRTCStats(pc);
  localStream?.getTracks().forEach((t) => t.stop());
  pc?.getSenders().forEach((s) => {
    try {
      s.track?.stop();
    } catch {
      /* 忽略该异常，继续执行后续操作。 */
    }
  });
  pc?.close();
  if (pc) reportEvent("webrtc.pc", { phase: "closed" });
}

/** 切换通话中的扬声器（需浏览器支持 HTMLMediaElement.setSinkId）。 */
export async function applyAudioOutput(element, deviceId) {
  if (!element || !deviceId || typeof element.setSinkId !== "function") return;
  await element.setSinkId(deviceId);
}

/** 通话中切换麦克风或摄像头，不重建 PeerConnection。 */
export async function replaceInputDevice(pc, localStream, kind, deviceId) {
  const constraints =
    kind === "video"
      ? { video: deviceId ? { deviceId: { exact: deviceId } } : true, audio: false }
      : { audio: microphoneConstraints(deviceId), video: false };
  const stream = await navigator.mediaDevices.getUserMedia(constraints);
  const track = kind === "video" ? stream.getVideoTracks()[0] : stream.getAudioTracks()[0];
  const sender = pc?.getSenders().find((s) => s.track?.kind === kind);
  if (sender) await sender.replaceTrack(track);
  else if (pc) pc.addTrack(track, stream);
  localStream?.getTracks().filter((t) => t.kind === kind).forEach((t) => {
    localStream.removeTrack(t);
    t.stop();
  });
  localStream?.addTrack(track);
  return track;
}

/** 用屏幕共享替换当前视频轨。 */
export async function startScreenShare(pc) {
  const stream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false });
  const track = stream.getVideoTracks()[0];
  const sender = pc?.getSenders().find((s) => s.track?.kind === "video");
  if (sender) await sender.replaceTrack(track);
  else pc.addTrack(track, stream);
  return stream;
}
