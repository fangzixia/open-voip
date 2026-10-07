package media

import "open-switch/internal/ports/dto"

const tapRecordingRate = 8000

// tapEnqueue 将 tap 写盘放到 pcmMixAsync，避免阻塞 RTP/混音热路径。
func (rec *recorder) tapEnqueue(fn func()) {
	if rec == nil || fn == nil {
		return
	}
	if rec.pcmAsync != nil {
		rec.pcmAsync.enqueue(func() {
			rec.mu.Lock()
			defer rec.mu.Unlock()
			fn()
		})
		return
	}
	rec.mu.Lock()
	fn()
	rec.mu.Unlock()
}

func (rec *recorder) tapMain() *pcmMix {
	if rec == nil {
		return nil
	}
	return rec.pcm
}

func (rec *recorder) tapUplinkAllowed(role dto.LegRole) bool {
	if rec == nil {
		return false
	}
	if !rec.gateInboundUntilPrompt {
		return true
	}
	_ = role
	return false
}

func pcmToTap8k(pcm []int16, fromRate int) []int16 {
	if len(pcm) == 0 {
		return pcm
	}
	if fromRate <= 0 || fromRate == tapRecordingRate {
		return pcm
	}
	return resamplePCM(pcm, fromRate, tapRecordingRate)
}

// TapUplink 分轨：该 leg 解码后的上行 PCM（顺序写入）。
func (rec *recorder) TapUplink(legID string, role dto.LegRole, pcm []int16, fromRate int) {
	if legID == "" || legID == "prompt" || len(pcm) == 0 {
		return
	}
	if !rec.tapUplinkAllowed(role) {
		return
	}
	pcm8 := pcmToTap8k(pcm, fromRate)
	rec.tapEnqueue(func() {
		m := rec.ensureLegPCM(legID, tapRecordingRate)
		if m != nil {
			m.appendSequential(pcm8)
		}
		rec.bytes = rec.mainByteSize()
	})
}

// TapMainMixed 主录：客户 leg 听到的混音（与 dispatchMixTick 下发 mixed 一致）。
func (rec *recorder) TapMainMixed(role dto.LegRole, pcm []int16, fromRate int) {
	if role != dto.LegRoleCustomer || len(pcm) == 0 {
		return
	}
	pcm8 := pcmToTap8k(pcm, fromRate)
	rec.tapEnqueue(func() {
		if m := rec.tapMain(); m != nil {
			m.appendSequential(pcm8)
		}
		rec.bytes = rec.mainByteSize()
	})
}

// TapPromptPCM IVR/等待音写入主录（gate 阶段仅保留出站提示音）。
func (rec *recorder) TapPromptPCM(pcm []int16) {
	if len(pcm) == 0 {
		return
	}
	pcm8 := pcmToTap8k(pcm, tapRecordingRate)
	rec.tapEnqueue(func() {
		if m := rec.tapMain(); m != nil {
			m.appendSequential(pcm8)
			rec.pcmAnchored = true
		}
		rec.bytes = rec.mainByteSize()
	})
}

func (rec *recorder) mainByteSize() int64 {
	if rec.pcm != nil {
		return rec.pcm.byteSize()
	}
	return rec.bytes
}

// tapRecording 标记 recorder 使用 tap 引擎（与 recording.opened 日志一致）。
func (rec *recorder) tapRecording() bool {
	return rec != nil && rec.recordEngine == "tap"
}
