package configio

import (
	"context"
	"open-call/internal/ports"
	"open-call/internal/store/models"
	"strings"
	"testing"
)

func TestCompileSwitchBundleQueueAgents(t *testing.T) {
	bundle := Bundle{
		Users:       []UserDump{{ID: "u1", Username: "坐席", LoginName: "a1", EmployeeNo: "E1", Role: "agent"}},
		Agents:      []models.Agent{{ID: "ag1", UserID: "u1", Extension: "1001"}},
		Skills:      []models.Skill{{ID: "sk1", Name: "通用"}},
		Queues:      []models.Queue{{ID: "q1", Name: "语音", DispatchStrategy: "longest_idle", RecordingPolicy: "off", OverflowAction: "hangup"}},
		AgentSkills: []models.AgentSkill{{AgentID: "ag1", SkillID: "sk1"}},
		QueueAgents: []models.QueueAgent{{QueueID: "q1", AgentID: "ag1"}},
		QueueSkills: []models.QueueSkill{{QueueID: "q1", SkillID: "sk1"}},
	}
	out, err := compileSwitchBundle(t.Context(), nil, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Queues) != 1 || len(out.Queues[0].AgentIDs) != 1 || out.Queues[0].AgentIDs[0] != "ag1" {
		t.Fatalf("queues %+v", out.Queues)
	}
	if len(out.Agents) != 1 || len(out.Agents[0].SkillIDs) != 1 {
		t.Fatalf("agents %+v", out.Agents)
	}
}

func TestBundleRoundTripRetainsAudioProfileAndCompilesBusinessIVR(t *testing.T) {
	active := ports.SwitchActiveConfiguration{Bundle: ports.SwitchConfigBundle{Queues: []ports.SwitchQueueConfig{{ID: "q", Name: "Support", AudioProfile: "wideband", PostCallIVRFlowID: "f"}}}}
	bundle := bundleFromActive(active)
	bundle.IVRVersions = []models.IVRPublishedSnapshot{{FlowID: "f", Version: 2, PayloadJSON: `{"start":"score","nodes":{"score":{"type":"csat","next":"end","default":"end"},"end":{"type":"hangup"}}}`}}
	out, err := compileSwitchBundle(context.Background(), nil, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Queues) != 1 || out.Queues[0].AudioProfile != "wideband" || out.Queues[0].PostCallIVRFlowID != "f" {
		t.Fatalf("queue changed: %+v", out.Queues)
	}
	if len(out.IVRs) != 1 || !strings.Contains(out.IVRs[0].PayloadJSON, `"type":"collect_input"`) || strings.Contains(out.IVRs[0].PayloadJSON, `"type":"csat"`) {
		t.Fatalf("business IVR not compiled: %+v", out.IVRs)
	}
}

func TestExportKeepsBusinessDraftsAndUnpublishedFlows(t *testing.T) {
	flows := mergeIVRDrafts([]models.IVRFlow{{ID: "rating", Name: "满意度", DraftJSON: `{"nodes":{"input":{"type":"csat"}}}`}, {ID: "draft", Name: "未发布"}}, []models.IVRFlow{{ID: "rating", Name: "rating", DraftJSON: `{"nodes":{"input":{"type":"collect_input"}}}`}, {ID: "external", Name: "external"}})
	if len(flows) != 3 || flows[0].Name != "满意度" || !strings.Contains(flows[0].DraftJSON, `"type":"csat"`) || flows[1].ID != "draft" || flows[2].ID != "external" {
		t.Fatalf("business drafts overwritten: %+v", flows)
	}
}
