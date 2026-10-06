package control

import (
	"context"
	"testing"
	"time"

	"open-switch/internal/ports/dto"
)

func TestVoiceNotificationAllowsJoinWebRTC(t *testing.T) {
	svc, media, _, _ := newTestService("ag1")
	media.sipOK = true
	ctx := context.Background()
	id, err := svc.VoiceNotification(ctx, dto.VoiceNotificationRequest{
		AgentID: "ag1", Destination: "+8613800138000", PromptAssetID: "00000000-0000-0000-0000-000000000001.wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var agentLeg string
	for time.Now().Before(deadline) {
		view, err := svc.GetCall(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if view.OutboundMode != "prompt_outbound" {
			t.Fatalf("expected prompt_outbound mode, got %q", view.OutboundMode)
		}
		if view.PromptAssetID == "" {
			t.Fatal("expected prompt_asset_id on call view")
		}
		for _, leg := range view.Legs {
			if leg.AgentID == "ag1" {
				agentLeg = leg.ID
			}
		}
		if view.State == stateActive && agentLeg != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if agentLeg == "" {
		t.Fatal("missing agent leg")
	}
	if _, err := svc.JoinWebRTC(ctx, id, agentLeg); err != nil {
		t.Fatalf("JoinWebRTC: %v", err)
	}
}

func TestVoiceNotificationDoesNotInjectOnAnswer(t *testing.T) {
	svc, media, _, _ := newTestService("ag1")
	media.sipOK = true
	ctx := context.Background()
	id, err := svc.VoiceNotification(ctx, dto.VoiceNotificationRequest{
		AgentID: "ag1", Destination: "+8613800138000", PromptAssetID: "00000000-0000-0000-0000-000000000001.wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		view, err := svc.GetCall(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if view.State == stateActive {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if media.injects != 0 {
		t.Fatalf("server must not InjectAudio for voice notification, injects=%d", media.injects)
	}
}
