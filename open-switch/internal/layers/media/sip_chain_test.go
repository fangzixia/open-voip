package media

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"

	"open-switch/internal/config"
)

// TestSIPChainAnswerGateThenInDialog 验收：应答门控（200 OK 前不跑媒体侧逻辑）与 dialog 建立后 BYE 必须 tag 匹配。
func TestSIPChainAnswerGateThenInDialog(t *testing.T) {
	media := &Service{}
	media.sip = newSIPUA(config.SIPConfig{}, media)
	u := media.sip
	callID := "chain-call"
	sipCID := "sip-cid-chain"
	u.putDialog(&sipSession{callID: callID, sipCallID: sipCID})

	media.markSIPAnswerPending(callID)
	var mediaStarted atomic.Bool
	if !media.DeferUntilAnswered(callID, func() { mediaStarted.Store(true) }) {
		t.Fatal("IVR/媒体逻辑应在应答前挂起")
	}
	byeEarly := inDialogRequest(sip.BYE, sipCID, "remote", "")
	if u.inDialog(byeEarly) == nil {
		t.Fatal("未应答前应仍可按 Call-ID 匹配会话")
	}

	media.finishSIPAnswer(callID, true)
	deadline := time.Now().Add(2 * time.Second)
	for !mediaStarted.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !mediaStarted.Load() {
		t.Fatal("200 OK 后应释放挂起逻辑")
	}

	good := inDialogRequest(sip.BYE, sipCID, "remote", "local-tag")
	dlgID, err := sip.DialogIDFromRequestUAS(good)
	if err != nil {
		t.Fatal(err)
	}
	u.putDialog(&sipSession{callID: callID, sipCallID: sipCID, dlgID: dlgID})
	if u.inDialog(good) == nil {
		t.Fatal("应答后 BYE 须匹配 dialog tag")
	}
	if u.inDialog(inDialogRequest(sip.BYE, sipCID, "remote", "wrong")) != nil {
		t.Fatal("错误 To tag 不得挂断")
	}
}
