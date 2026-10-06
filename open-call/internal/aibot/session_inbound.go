package aibot

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"open-call/internal/aibot/realtime"
	"open-call/internal/config"
)

var (
	errNoAgentLeg = errors.New("缺少 agent 通话腿")
	errNoPSTNLeg  = errors.New("缺少 PSTN 通话腿")
)

// 电话侧下行略增益，补偿窄带与混音后的听感音量。
const inboundAIDownlinkGain = 1.35

// RunInboundAI 驱动一条 AI 呼入通话的媒体与会话生命周期。
//
// 数据流（简化）：
//  主叫/排队混音 → Switch WebRTC → onRemote → 重采样 → Realtime 上行；
//  Realtime TTS 下行 → 重采样 8k → pacer → PCMU → Switch → 主叫。
//
// 须在 Switch 已振铃到本 agent 后调用；通过轮询 GetCall 感知挂断，不阻塞在 Realtime 读循环里。
func RunInboundAI(ctx context.Context, sig *Signaling, cfg config.AibotConfig, callID, mediaAgentID string, usage *UsageRecorder, log *slog.Logger) error {
	// 与人工坐席相同：先应答再建 WebRTC，避免主叫长时间听静音。
	if err := sig.api.Answer(ctx, callID, mediaAgentID); err != nil {
		return err
	}
	view, err := sig.api.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	agentLeg, ok := findAgentLeg(view, mediaAgentID)
	if !ok {
		return errNoAgentLeg
	}
	peer, err := sig.Connect(ctx, callID, agentLeg, mediaAgentID, cfg.WidebandWebRTC)
	if err != nil {
		return err
	}
	defer peer.Close()
	waitCtx, waitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer waitCancel()
	if err := peer.WaitConnected(waitCtx); err != nil {
		return err
	}
	// 队列入呼走 SFU 混音，不可使用 Direct Bridge（Switch 对 queue_id 通话拒绝桥接）。

	rs, err := realtime.Connect(ctx, cfg.OpenAI, cfg.SystemPrompt)
	if err != nil {
		return err
	}
	defer func() { _ = rs.Close() }()
	rs.BindCallLog(log, callID)

	outRate := realtime.RealtimeOutputRate(cfg.OpenAI)
	inRate := realtime.RealtimeInputRate(cfg.OpenAI)
	const phoneRate = 8000

	// Realtime 按块推送 TTS，电话网需要稳定 20ms 帧；pacer 与 WritePCMU8k 解耦突发写入。
	pacer := newPCMDownlinkPacer(peer)
	pacerCtx, pacerCancel := context.WithCancel(ctx)
	defer pacerCancel()
	go pacer.Run(pacerCtx)

	var inputMs, outputMs atomic.Int64
	var downlinkResampleLogged atomic.Bool
	start := time.Now()

	rs.OnAudio(func(pcm []byte) {
		// 通义/OpenAI 下行多为 16k/24k PCM16；SIP 腿仅 8k，必须先高质量降采样再 G.711。
		samples := realtime.PCM16BytesToSamples(pcm)
		samples8k := realtime.ResamplePCM(samples, outRate, phoneRate)
		if !downlinkResampleLogged.Load() && len(samples) > 0 {
			downlinkResampleLogged.Store(true)
			if log != nil {
				log.Info("aibot.downlink.resample",
					"call_id", callID,
					"from_rate_hz", outRate,
					"to_rate_hz", phoneRate,
					"samples_in", len(samples),
					"samples_out", len(samples8k),
					"webrtc_codec", peer.DownlinkCodec(),
				)
			}
		}
		samples8k = amplifyPCM(samples8k, inboundAIDownlinkGain)
		outputMs.Add(int64(len(samples8k)) * 1000 / phoneRate)
		pacer.Push(samples8k)
	})

	peer.SetRemotePCMHandler(func(pcm []int16, rate int) {
		// 对端可能是 8k PCMU 或 48k Opus 解码结果，统一到 Realtime 约定的上行采样率。
		pcmIn := realtime.ResamplePCM(pcm, rate, inRate)
		inputMs.Add(int64(len(pcmIn)) * 1000 / int64(inRate))
		raw := samplesToLE(pcmIn)
		_ = rs.AppendInputPCM(ctx, raw)
	})

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if usage != nil {
				usage.RecordAISession(callID, mediaAgentID, view.QueueID, start, inputMs.Load(), outputMs.Load())
			}
			return ctx.Err()
		case <-ticker.C:
			v, err := sig.api.GetCall(ctx, callID)
			if err != nil {
				continue
			}
			if v.State == "ended" {
				if usage != nil {
					usage.RecordAISession(callID, mediaAgentID, v.QueueID, start, inputMs.Load(), outputMs.Load())
				}
				return nil
			}
		}
	}
}

// amplifyPCM 对窄带下行做限幅增益，避免削波。
func amplifyPCM(pcm []int16, gain float64) []int16 {
	if gain <= 0 || gain == 1 || len(pcm) == 0 {
		return pcm
	}
	out := make([]int16, len(pcm))
	for i, s := range pcm {
		v := float64(s) * gain
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		out[i] = int16(v)
	}
	return out
}

// samplesToLE 将 int16 样本转为小端 PCM16 字节流（Realtime 上行格式）。
func samplesToLE(samples []int16) []byte {
	b := make([]byte, len(samples)*2)
	for i, s := range samples {
		b[2*i] = byte(s)
		b[2*i+1] = byte(s >> 8)
	}
	return b
}
