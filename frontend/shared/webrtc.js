/**
 * WebRTC 信令与 PeerConnection 封装：Offer/Answer/ICE 走 REST。
 */

import { getRuntimeConfig } from "./runtime-config.js";
import { fetchTurn, postAnswer, postIce, postMute, postOffer } from "./api.js";

/** @returns {RTCConfiguration} */
export function buildRtcConfiguration(iceServersFromApi) {
  const { publicUrl } = getRuntimeConfig();
  const servers = iceServersFromApi ?? [{ urls: "stun:stun.l.google.com:19302" }];
  return {
    iceServers: servers,
    bundlePolicy: "max-bundle",
    ...(publicUrl ? {} : {}),
  };
}

export function createPeerConnection(configuration) {
  return new RTCPeerConnection(configuration);
}

export function isWebRTCSupported() {
  return typeof RTCPeerConnection !== "undefined";
}

export async function listMediaDevices() {
  const devices = await navigator.mediaDevices.enumerateDevices();
  return {
    audioInputs: devices.filter((d) => d.kind === "audioinput"),
    videoInputs: devices.filter((d) => d.kind === "videoinput"),
    audioOutputs: devices.filter((d) => d.kind === "audiooutput"),
  };
}

/**
 * 建立与 SFU 的 WebRTC 会话（服务端 Offer）。
 * @returns {Promise<{pc: RTCPeerConnection, localStream: MediaStream, remoteStream: MediaStream}>}
 */
export async function startMediaSession({ callId, legId, video, audioDeviceId, videoDeviceId, recvOnly }) {
  const localStream = recvOnly
    ? new MediaStream()
    : await navigator.mediaDevices.getUserMedia({
        audio: audioDeviceId ? { deviceId: { exact: audioDeviceId } } : true,
        video: video ? (videoDeviceId ? { deviceId: { exact: videoDeviceId } } : true) : false,
      });
  let iceServers = [{ urls: "stun:stun.l.google.com:19302" }];
  try {
    const turn = await fetchTurn(callId);
    if (turn?.urls?.length) {
      iceServers = iceServers.concat({
        urls: turn.urls,
        username: turn.username,
        credential: turn.credential,
      });
    }
  } catch {
    /* TURN 未启用 */
  }
  const pc = createPeerConnection(buildRtcConfiguration(iceServers));
  const remoteStream = new MediaStream();
  pc.ontrack = (ev) => {
    ev.streams[0]?.getTracks().forEach((t) => {
      if (!remoteStream.getTracks().some((x) => x.id === t.id)) remoteStream.addTrack(t);
    });
    if (!ev.streams[0]) remoteStream.addTrack(ev.track);
  };
  localStream.getTracks().forEach((t) => pc.addTrack(t, localStream));
  pc.onicecandidate = (ev) => {
    if (!ev.candidate) return;
    postIce(callId, legId, {
      candidate: ev.candidate.candidate,
      sdp_mid: ev.candidate.sdpMid,
      sdp_mline_index: ev.candidate.sdpMLineIndex,
    }).catch(() => {});
  };
  const offer = await postOffer(callId, legId);
  await pc.setRemoteDescription({ type: "offer", sdp: offer.sdp });
  const answer = await pc.createAnswer();
  await pc.setLocalDescription(answer);
  await postAnswer(callId, legId, answer.sdp);
  return { pc, localStream, remoteStream };
}

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

export function stopMedia(pc, localStream) {
  localStream?.getTracks().forEach((t) => t.stop());
  pc?.getSenders().forEach((s) => {
    try {
      s.track?.stop();
    } catch {
      /* ignore */
    }
  });
  pc?.close();
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
      : { audio: deviceId ? { deviceId: { exact: deviceId } } : true, video: false };
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
