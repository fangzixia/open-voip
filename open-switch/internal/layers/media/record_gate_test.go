package media

import (
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

func TestGateInboundUntilPrompt(t *testing.T) {
	started := time.Now()
	rec := &recorder{
		started:                started,
		pcm:                    newPCMMix(8000, started),
		gateInboundUntilPrompt: true,
	}
	in := &rtp.Packet{Header: rtp.Header{PayloadType: 0}, Payload: mulawToneFrame()}
	rec.writeRTP("customer", webrtc.RTPCodecTypeAudio, "audio/PCMU", in)
	if len(rec.pcm.samples) > 0 {
		t.Fatal("inbound should be gated before prompt")
	}
	recordPromptToMix(rec, mulawToneFrame())
	if len(rec.pcm.samples) < 160 {
		t.Fatal("prompt should be mixed")
	}
	rec.writeRTP("customer", webrtc.RTPCodecTypeAudio, "audio/PCMU", in)
	if len(rec.pcm.samples) > 200 {
		t.Fatal("inbound should stay gated after prompt while IVR")
	}
}
