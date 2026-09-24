package queue

import (
	"testing"

	"open-call/internal/store/models"
)

func TestComposeRecordingPolicy(t *testing.T) {
	d := PolicyDefaults{Mode: "audio", NotifyMessage: "告知", RetainDays: 90}

	noQueue := composeRecordingPolicy(nil, d)
	if noQueue.Mode != "audio" || noQueue.NotifyGuest || noQueue.RetainDays != 90 || noQueue.NotifyMessage != "告知" {
		t.Fatalf("no-queue policy=%+v", noQueue)
	}

	queued := composeRecordingPolicy(&models.Queue{RecordingPolicy: "video_composite", AnnounceRecording: true}, d)
	if queued.Mode != "video_composite" || !queued.NotifyGuest || queued.RetainDays != 90 {
		t.Fatalf("queue policy=%+v", queued)
	}

	off := composeRecordingPolicy(&models.Queue{RecordingPolicy: "off"}, d)
	if off.Mode != "off" || off.NotifyGuest {
		t.Fatalf("off policy=%+v", off)
	}
}
