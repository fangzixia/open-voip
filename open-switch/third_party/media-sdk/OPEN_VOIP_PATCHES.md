# Controlled media-sdk source

Upstream: https://github.com/livekit/media-sdk
Base: v0.1.2-0.20260925164200-bc3eace31b2e (bc3eace31b2e).
Go module normalized-content checksum (not the SHA256 of the ZIP bytes):
`h1:vWJUafmeTyUQto3aWsL7PO1nilnEzD2UcZBeOAjy/PU=`.
go.mod checksum: `h1:TuYRjSepaakL6ATsM9V2VMuksewW1PlhA32BG7Pxty0=`.
The Apache-2.0 LICENSE and upstream tests are retained.

Local extensions must be listed here and tested against this exact base.
The root package requires libsoxr under CGO; the opus package is not imported
by the G.711 pipeline, and libopus is not a narrowband build dependency.

The frozen resample writer only exposes fixed ratios and SOXR_LQ quality.
It does not expose cancellation/reset or variable-rate adjustment. The project
therefore provides a thin libsoxr binding for those native capabilities, with
streaming continuity, explicit drain/reset, and error propagation.

## Controlled extensions to this exact base

* `jitter.WithStartupDelay`: opt-in initial reorder reserve. Audio uses 20 ms;
  default upstream behavior remains available to upstream video tests.
* `jitter.WithBatchDelivery`: opt-in contiguous packet delivery in one callback,
  so a room tick cannot consume between commits from the same release batch.
* Duplicate sequence/SSRC detection and explicit duplicate/expired counters.
  The adapter reads final ordering counters separately from PLC sample counts.
* Closing the buffer serializes fuse cancellation and final delivery with Push;
  packets after close are ignored. Restart/switch statistics reset per-stream
  native state in the project adapter.
* `mixer.WithFrameHandler`: before-tick input preparation and owned, zero-padded
  same-frame stems. The existing mixer ticker, native input ring and mixing
  body are retained; project routing derives receiver-specific mix-minus.
* `mixer.WithInputBufferMin`: audio sets zero additional startup buffering and
  retains ten frames as the hard input bound. It avoids stacking the reserve.
* `mixer.Input.Reset`: clears the retained ring after a stream epoch change.
* `mixer.StopAndWait`: joins the ticker before native stream state is released.
* `mixer.WithTimingHandler`: reports actual tick lateness against scheduled
  time, without logging or callbacks under the mixer mutex.
* Native soxr errors have static library ownership and must not be freed.

Upstream root/jitter/mixer/ring tests and local extension tests are retained.
Run `go test -race . ./jitter ./mixer ./ring -count=1` in this directory with
libsoxr installed. There is no floating branch dependency or alternate engine.
The project's native wrapper uses HQ conversion and variable-rate control;
it is not a replacement resampling algorithm. SpanDSP owns G.711 PLC.
