import test from "node:test";
import assert from "node:assert/strict";
import {
  formatFileSizeKb,
  recordingDurationLabel,
  recordingFormatLabel,
  recordingSizeKbLabel,
} from "./recording-meta.js";

test("formatFileSizeKb", () => {
  assert.equal(formatFileSizeKb(0), "0 KB");
  assert.equal(formatFileSizeKb(1024), "1.0 KB");
  assert.equal(formatFileSizeKb(1536), "1.5 KB");
  assert.equal(formatFileSizeKb(102400), "100 KB");
});

test("recordingFormatLabel", () => {
  assert.equal(recordingFormatLabel({ format: "ogg", media_type: "audio" }), "OGG");
  assert.equal(recordingFormatLabel({ format: "mp4", media_type: "video_composite" }), "MP4");
  assert.equal(recordingFormatLabel({ format: "ivf", media_type: "video_composite" }), "旧版分离");
});

test("recordingDurationLabel from samples", () => {
  assert.equal(
    recordingDurationLabel({ duration_samples: 9600, sample_rate_hz: 8000 }),
    "1.20 秒",
  );
  assert.equal(
    recordingDurationLabel({ duration_samples: 480000, sample_rate_hz: 8000 }),
    "01:00",
  );
});

test("recordingSizeKbLabel", () => {
  assert.equal(recordingSizeKbLabel({ file_size: 2048 }), "2.0 KB");
});
