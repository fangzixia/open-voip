// 通话控制、桥接、媒体信令与会话内动作。
import { callMutate, setCallVersion, newIdempotencySuffix } from "../call-mutation.js";
import { apiFetch } from "./client.js";

export async function getCall(callId) {
  const view = await apiFetch(`/api/v1/calls/${callId}`);
  if (view?.version != null) setCallVersion(view.version, callId);
  return view;
}

export async function listOpenCalls() {
  const data = await apiFetch("/api/v1/calls?status=open");
  return data?.items || [];
}

export function createBridge(callId, legA, legB) {
  return callMutate(`/api/v1/calls/${callId}/bridges`, {
    action: "bridge",
    callId,
    method: "POST",
    body: { leg_ids: [legA, legB], leg_a: legA, leg_b: legB },
  });
}

export function answerCall(callId) {
  return callMutate(`/api/v1/calls/${callId}/answer`, { action: "answer", callId, method: "POST", body: {} });
}

export function hangupCall(callId, reason = "normal") {
  return callMutate(`/api/v1/calls/${callId}/hangup`, { action: "hangup", callId, method: "POST", body: { reason } });
}

export function declineCall(callId) {
  return callMutate(`/api/v1/calls/${callId}/decline`, { action: "decline", callId, method: "POST", body: {} });
}

export function holdLeg(callId, legId, on) {
  return callMutate(`/api/v1/calls/${callId}/legs/${legId}/hold`, { action: "leg.hold", callId, method: "POST", body: { on } });
}

export function rejectLeg(callId, legId, reason = "") {
  return callMutate(`/api/v1/calls/${callId}/legs/${legId}/reject`, { action: "leg.reject", callId, method: "POST", body: { reason } });
}

export function startLegPlayback(callId, legId, assetId) {
  return callMutate(`/api/v1/calls/${callId}/legs/${legId}/playbacks`, {
    action: "leg.playback",
    callId,
    method: "POST",
    body: { asset_id: assetId },
  });
}

export function stopLegPlayback(callId, legId, playbackId) {
  return callMutate(`/api/v1/calls/${callId}/legs/${legId}/playbacks/${playbackId}`, {
    action: "leg.playback.stop",
    callId,
    method: "DELETE",
    body: {},
  });
}

export function replaceBridge(callId, bridgeId, legA, legB) {
  return callMutate(`/api/v1/calls/${callId}/bridges/${bridgeId}`, {
    action: "bridge.replace",
    callId,
    method: "PUT",
    body: { leg_ids: [legA, legB], leg_a: legA, leg_b: legB },
  });
}

export function endBridge(callId, bridgeId) {
  return callMutate(`/api/v1/calls/${callId}/bridges/${bridgeId}`, {
    action: "bridge.end",
    callId,
    method: "DELETE",
    body: {},
  });
}

export function postOffer(callId, legId) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/offer`, {
    method: "POST",
    body: JSON.stringify({ type: "offer" }),
  });
}

export function postAnswer(callId, legId, sdp) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/answer`, {
    method: "POST",
    body: JSON.stringify({ type: "answer", sdp }),
  });
}

export function postIce(callId, legId, candidate) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/ice`, {
    method: "POST",
    body: JSON.stringify(candidate),
  });
}

export function postMute(callId, legId, audio, video) {
  return apiFetch(`/api/v1/calls/${callId}/legs/${legId}/mute`, {
    method: "POST",
    body: JSON.stringify({ audio, video }),
  });
}

export function outboundCall(destination) {
  return apiFetch("/api/v1/calls/outbound", {
    method: "POST",
    body: JSON.stringify({ destination }),
  });
}

export function voiceNotificationCall(destination, promptAssetId, requestKey = newIdempotencySuffix()) {
  return apiFetch("/api/v1/calls/voice-notifications", {
    method: "POST",
    headers: { "Idempotency-Key": requestKey },
    body: JSON.stringify({ destination, prompt_asset_id: promptAssetId }),
  });
}

export function getVoiceNotification(taskId) {
  return apiFetch(`/api/v1/calls/voice-notifications/${encodeURIComponent(taskId)}`);
}
export function cancelVoiceNotification(taskId) {
  return apiFetch(`/api/v1/calls/voice-notifications/${encodeURIComponent(taskId)}`, { method: "DELETE" });
}

export function holdCall(callId, on) {
  return callMutate(`/api/v1/calls/${callId}/hold`, { action: "hold", callId, method: "POST", body: { on } });
}

export function startSurvey(callId, flowId = "") {
  const body = flowId ? { flow_id: flowId } : {};
  return callMutate(`/api/v1/calls/${callId}/survey`, { action: "survey", callId, method: "POST", body });
}

export function transferCall(callId, body) {
  return callMutate(`/api/v1/calls/${callId}/transfer`, { action: "transfer", callId, method: "POST", body });
}

export function completeTransfer(callId) {
  return callMutate(`/api/v1/calls/${callId}/transfer/complete`, {
    action: "transfer.complete",
    callId,
    method: "POST",
    body: {},
  });
}

export function requestVideo(callId) {
  return apiFetch(`/api/v1/calls/${callId}/video/request`, { method: "POST" });
}

export function respondVideo(callId, accept) {
  return apiFetch(`/api/v1/calls/${callId}/video/respond`, {
    method: "POST",
    body: JSON.stringify({ accept }),
  });
}

export function downgradeVideo(callId) {
  return apiFetch(`/api/v1/calls/${callId}/video/downgrade`, { method: "POST" });
}

export function screenShare(callId, on, legId) {
  return apiFetch(`/api/v1/calls/${callId}/screen-share`, {
    method: "POST",
    body: JSON.stringify({ on, leg_id: legId }),
  });
}

export function sendDtmf(callId, legId, digit) {
  return apiFetch(`/api/v1/calls/${callId}/dtmf`, {
    method: "POST",
    body: JSON.stringify({ leg_id: legId, digit }),
  });
}

export function wrapUp(callId, notes) {
  return apiFetch(`/api/v1/calls/${callId}/wrap-up`, {
    method: "POST",
    body: JSON.stringify({ notes }),
  });
}

export function fetchTurn(callId) {
  return apiFetch(`/api/v1/calls/${callId}/turn-credentials`);
}

export function conferenceInvite(callId, agentId) {
  return apiFetch(`/api/v1/calls/${callId}/conference`, {
    method: "POST",
    body: JSON.stringify({ agent_id: agentId }),
  });
}

export function addQaMark(callId, offsetSec, label) {
  return apiFetch(`/api/v1/calls/${callId}/qa-marks`, {
    method: "POST",
    body: JSON.stringify({ offset_sec: offsetSec, label }),
  });
}
