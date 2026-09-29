package skill

import (
	"context"
	"strings"
	"time"

	"uuid"

	"open-call/internal/errs"
	"open-call/internal/ports"
)

// DTO 技能组。
type DTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Service 技能 CRUD（Switch 权威）。
type Service struct {
	sw ports.SwitchAdminPort
}

// NewService 创建技能服务。
func NewService(sw ports.SwitchAdminPort) *Service { return &Service{sw: sw} }

// List 列出技能。
func (s *Service) List(ctx context.Context) ([]DTO, error) {
	rows, err := s.sw.ListSkillConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, DTO{ID: r.ID, Name: r.Name})
	}
	return out, nil
}

// Create 创建技能。
func (s *Service) Create(ctx context.Context, name string) (DTO, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return DTO{}, errs.InvalidRequest("技能名称必填")
	}
	created, err := s.sw.CreateSkillConfig(ctx, ports.SwitchSkillConfig{ID: uuid.New().String(), Name: name})
	if err != nil {
		return DTO{}, err
	}
	return DTO{ID: created.ID, Name: created.Name}, nil
}

func (s *Service) Update(ctx context.Context, id, name string) (DTO, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return DTO{}, errs.InvalidRequest("技能名称必填")
	}
	updated, err := s.sw.UpdateSkillConfig(ctx, id, name)
	if err != nil {
		return DTO{}, err
	}
	return DTO{ID: updated.ID, Name: updated.Name}, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.sw.DeleteSkillConfig(ctx, id)
}

func (s *Service) BindAgents(ctx context.Context, skillID string, agentIDs []string) error {
	agents, err := s.sw.ListAgentConfigs(ctx)
	if err != nil {
		return err
	}
	for _, ag := range agents {
		has := false
		for _, sid := range ag.SkillIDs {
			if sid == skillID {
				has = true
				break
			}
		}
		want := false
		for _, id := range agentIDs {
			if id == ag.ID {
				want = true
				break
			}
		}
		skills := append([]string(nil), ag.SkillIDs...)
		if want && !has {
			skills = append(skills, skillID)
		}
		if !want && has {
			next := skills[:0]
			for _, sid := range skills {
				if sid != skillID {
					next = append(next, sid)
				}
			}
			skills = next
		}
		if want != has {
			if _, err := s.sw.SetAgentSkills(ctx, ag.ID, skills); err != nil {
				return err
			}
		}
	}
	return nil
}

// BindAgent 覆盖坐席技能绑定（Switch 权威）。
func (s *Service) BindAgent(ctx context.Context, agentID string, skillIDs []string) error {
	_, err := s.sw.SetAgentSkills(ctx, agentID, skillIDs)
	return err
}

// AgentSkills 读取坐席技能。
func (s *Service) AgentSkills(ctx context.Context, agentID string) ([]string, error) {
	agents, err := s.sw.ListAgentConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for _, ag := range agents {
		if ag.ID == agentID {
			if ag.SkillIDs == nil {
				return []string{}, nil
			}
			return ag.SkillIDs, nil
		}
	}
	return nil, errs.NotFound("坐席不存在")
}
