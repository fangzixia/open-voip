/**
 * WebRTC 信令与 PeerConnection 封装：Offer/Answer/ICE 走 REST（Phase 1），
 * 本模块仅提供统一配置与占位，避免三端各自创建 RTCPeerConnection。
 */

import { getRuntimeConfig } from "./runtime-config.js";

/** @returns {RTCConfiguration} */
export function buildRtcConfiguration(iceServersFromApi) {
  const { publicUrl } = getRuntimeConfig();
  const servers = iceServersFromApi ?? [{ urls: "stun:stun.l.google.com:19302" }];
  return {
    iceServers: servers,
    bundlePolicy: "max-bundle",
    /* 内网部署时 publicUrl 用于日志关联，不参与 ICE */
    ...(publicUrl ? {} : {}),
  };
}

/**
 * 创建本地 PeerConnection（Phase 1 与 media 信令 API 配合）。
 * @param {RTCConfiguration} configuration
 */
export function createPeerConnection(configuration) {
  return new RTCPeerConnection(configuration);
}

/** Phase 0：检测浏览器是否具备 WebRTC */
export function isWebRTCSupported() {
  return typeof RTCPeerConnection !== "undefined";
}
