import test from "node:test";
import assert from "node:assert/strict";
import { acquireAgentMedia, addRemoteTrack, microphoneConstraints } from "./webrtc.js";

function stream(audio = [], video = []) {
  return {
    audio,
    video,
    getAudioTracks() { return this.audio; },
    getVideoTracks() { return this.video; },
    addTrack(track) { this.video.push(track); },
  };
}

test("agent keeps microphone when camera is missing", async () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "navigator");
  const microphone = stream([{ kind: "audio" }]);
  const requests = [];
  Object.defineProperty(globalThis, "navigator", {
    configurable: true,
    value: { mediaDevices: { async getUserMedia(constraints) {
      requests.push(constraints);
      if (constraints.video) throw Object.assign(new Error("Requested device not found"), { name: "NotFoundError" });
      return microphone;
    } } },
  });
  try {
    const result = await acquireAgentMedia({ audioDeviceId: "", videoDeviceId: "" });
    assert.equal(result.stream, microphone);
    assert.equal(result.cameraUnavailable, true);
    assert.equal(result.stream.getAudioTracks().length, 1);
    assert.deepEqual(requests.map((r) => [r.audio, r.video]), [[microphoneConstraints(), false], [false, true]]);
  } finally {
    if (previous) Object.defineProperty(globalThis, "navigator", previous);
    else delete globalThis.navigator;
  }
});

test("microphone constraints keep audio processing when a device is selected", () => {
  assert.deepEqual(microphoneConstraints("selected-mic"), {
    deviceId: { exact: "selected-mic" },
    echoCancellation: true,
    noiseSuppression: true,
    autoGainControl: true,
  });
});

test("agent receives camera track when one is available", async () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "navigator");
  const microphone = stream([{ kind: "audio" }]);
  const camera = stream([], [{ kind: "video" }]);
  Object.defineProperty(globalThis, "navigator", {
    configurable: true,
    value: { mediaDevices: { async getUserMedia(constraints) {
      return constraints.video ? camera : microphone;
    } } },
  });
  try {
    const result = await acquireAgentMedia({ audioDeviceId: "", videoDeviceId: "" });
    assert.equal(result.cameraUnavailable, false);
    assert.equal(result.stream.getAudioTracks().length, 1);
    assert.equal(result.stream.getVideoTracks().length, 1);
  } finally {
    if (previous) Object.defineProperty(globalThis, "navigator", previous);
    else delete globalThis.navigator;
  }
});

test("remote conversation excludes the waiting tone track", () => {
  const conversation = { tracks: [], getTracks() { return this.tracks; }, addTrack(track) { this.tracks.push(track); } };
  const waiting = { tracks: [], getTracks() { return this.tracks; }, addTrack(track) { this.tracks.push(track); } };
  const audioOnly = { tracks: [], getTracks() { return this.tracks; }, addTrack(track) { this.tracks.push(track); } };
  const audio = { id: "caller-audio", kind: "audio" };
  const video = { id: "caller-video", kind: "video" };
  const moh = { id: "moh", kind: "audio" };
  addRemoteTrack(conversation, waiting, audio, [{ id: "sfu-audio-leg" }], audioOnly);
  addRemoteTrack(conversation, waiting, video, [{ id: "sfu-video-leg" }], audioOnly);
  addRemoteTrack(conversation, waiting, moh, [{ id: "sfu-moh-leg" }], audioOnly);
  addRemoteTrack(conversation, waiting, audio, [{ id: "sfu-audio-leg" }], audioOnly);
  assert.deepEqual(conversation.getTracks(), [audio, video]);
  assert.deepEqual(waiting.getTracks(), [moh]);
  assert.deepEqual(audioOnly.getTracks(), [audio]);
});
