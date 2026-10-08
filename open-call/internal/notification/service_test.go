package notification

import (
	"context"
	"open-call/internal/integration/switchapi"
	"open-call/internal/ports"
	"open-call/internal/ports/dto"
	"open-call/internal/store/models"
	"testing"
	"time"
)

type testAPI struct {
	state, playState              string
	created, dialed, played, hung int
	reason                        dto.HangupReason
}

func (a *testAPI) CreateApplicationCall(context.Context, string, string) error {
	a.created++
	return nil
}
func (a *testAPI) DialApplicationSIP(context.Context, string, string, string) (string, error) {
	a.dialed++
	return "pstn", nil
}
func (a *testAPI) GetCall(context.Context, string) (ports.CallView, error) {
	return ports.CallView{State: a.state}, nil
}
func (a *testAPI) PlayAsset(context.Context, string, string, string, string) (string, error) {
	a.played++
	return "play", nil
}
func (a *testAPI) Playback(context.Context, string, string, string) (switchapi.PlaybackStatus, error) {
	return switchapi.PlaybackStatus{State: a.playState}, nil
}
func (a *testAPI) Hangup(_ context.Context, _ string, r dto.HangupReason) error {
	a.hung++
	a.reason = r
	return nil
}

func TestNotificationWaitsForAnswerAndServerDrain(t *testing.T) {
	a := &testAPI{state: "ringing", playState: "playing"}
	s := &Service{api: a}
	task := models.VoiceNotification{ID: "call", AssetID: "asset", State: "queued", Deadline: time.Now().Add(time.Hour)}
	step := func() {
		t.Helper()
		if err := s.step(context.Background(), &task); err != nil {
			t.Fatal(err)
		}
	}
	step()
	step()
	if a.created != 1 || a.dialed != 1 || a.played != 0 || a.hung != 0 {
		t.Fatalf("played before answer: %+v", a)
	}
	a.state = "active"
	step()
	step()
	if a.played != 1 || a.hung != 0 {
		t.Fatalf("hung up before drain: %+v", a)
	}
	a.playState = "finished"
	step()
	if task.State != "completed" || a.hung != 1 || a.reason != dto.HangupReasonNormal {
		t.Fatalf("completion: %+v %+v", task, a)
	}
}
func TestNotificationCancelAndRestartAfterHangup(t *testing.T) {
	a := &testAPI{state: "ended"}
	s := &Service{api: a}
	task := models.VoiceNotification{State: "finishing", Deadline: time.Now().Add(time.Hour)}
	if err := s.step(context.Background(), &task); err != nil || task.State != "completed" {
		t.Fatalf("recovery after BYE: %+v %v", task, err)
	}
	task.State = "stopping"
	task.LastError = "canceled"
	if err := s.step(context.Background(), &task); err != nil || task.State != "canceled" {
		t.Fatalf("cancel: %+v %v", task, err)
	}
	if a.played != 0 {
		t.Fatal("cancellation started audio")
	}
}

func TestTerminalNotificationNeverRunsAgain(t *testing.T) {
	a := &testAPI{}
	s := &Service{api: a}
	for _, state := range []string{"completed", "failed", "canceled"} {
		task := models.VoiceNotification{State: state, Deadline: time.Now().Add(-time.Hour)}
		if err := s.step(context.Background(), &task); err != nil || task.State != state {
			t.Fatalf("terminal task changed: %+v %v", task, err)
		}
	}
	if a.created+a.dialed+a.played+a.hung != 0 {
		t.Fatal("completed task executed again")
	}
}
