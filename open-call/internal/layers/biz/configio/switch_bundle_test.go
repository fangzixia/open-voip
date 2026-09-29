package configio

import (
	"testing"

	"open-call/internal/store/models"
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
