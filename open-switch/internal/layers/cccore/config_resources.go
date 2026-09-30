package cccore

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
)

// ListQueueConfigs 返回激活配置中的全部队列。
func (s *Service) ListQueueConfigs(ctx context.Context) ([]ports.QueueConfig, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return nil, err
	}
	_ = appID
	bundle, err := s.loadActiveBundleOrEmpty(ctx, "")
	if err != nil {
		return nil, err
	}
	return append([]ports.QueueConfig(nil), bundle.Queues...), nil
}

// CreateQueueConfig 创建队列并激活新版本。
func (s *Service) CreateQueueConfig(ctx context.Context, in ports.QueueConfig) (ports.QueueConfig, error) {
	if !validUUID(in.ID) {
		in.ID = uuid.New().String()
	}
	normalizeBundle(&ports.ConfigBundle{Queues: []ports.QueueConfig{in}})
	var created ports.QueueConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		if findQueueIndex(b, in.ID) >= 0 {
			return errs.Conflict("队列 ID 已存在", "")
		}
		b.Queues = append(b.Queues, in)
		created = in
		return nil
	})
	return created, err
}

// GetQueueConfig 按 ID 读取队列配置。
func (s *Service) GetQueueConfig(ctx context.Context, id string) (ports.QueueConfig, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.QueueConfig{}, err
	}
	_ = appID
	bundle, err := s.loadActiveBundleOrEmpty(ctx, "")
	if err != nil {
		return ports.QueueConfig{}, err
	}
	i := findQueueIndex(&bundle, id)
	if i < 0 {
		return ports.QueueConfig{}, errs.NotFound("队列不存在")
	}
	return bundle.Queues[i], nil
}

// UpdateQueueConfig 替换队列配置（调用方负责合并 PATCH）。
func (s *Service) UpdateQueueConfig(ctx context.Context, id string, patch ports.QueueConfig) (ports.QueueConfig, error) {
	patch.ID = id
	var out ports.QueueConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		i := findQueueIndex(b, id)
		if i < 0 {
			return errs.NotFound("队列不存在")
		}
		b.Queues[i] = patch
		out = patch
		return nil
	})
	return out, err
}

// DeleteQueueConfig 删除队列。
func (s *Service) DeleteQueueConfig(ctx context.Context, id string) error {
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		if findQueueIndex(b, id) < 0 {
			return errs.NotFound("队列不存在")
		}
		removeQueue(b, id)
		return nil
	})
	return err
}

// SetQueueAgents 设置队列绑定的坐席。
func (s *Service) SetQueueAgents(ctx context.Context, queueID string, agentIDs []string) (ports.QueueConfig, error) {
	var out ports.QueueConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		i := findQueueIndex(b, queueID)
		if i < 0 {
			return errs.NotFound("队列不存在")
		}
		b.Queues[i].AgentIDs = append([]string(nil), agentIDs...)
		out = b.Queues[i]
		return nil
	})
	return out, err
}

// SetQueueSkills 设置队列绑定的技能。
func (s *Service) SetQueueSkills(ctx context.Context, queueID string, skillIDs []string) (ports.QueueConfig, error) {
	var out ports.QueueConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		i := findQueueIndex(b, queueID)
		if i < 0 {
			return errs.NotFound("队列不存在")
		}
		b.Queues[i].SkillIDs = append([]string(nil), skillIDs...)
		out = b.Queues[i]
		return nil
	})
	return out, err
}

// ListSkillConfigs 列出技能。
func (s *Service) ListSkillConfigs(ctx context.Context) ([]ports.SkillConfig, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return nil, err
	}
	_ = appID
	bundle, err := s.loadActiveBundleOrEmpty(ctx, "")
	if err != nil {
		return nil, err
	}
	return append([]ports.SkillConfig(nil), bundle.Skills...), nil
}

// CreateSkillConfig 创建技能。
func (s *Service) CreateSkillConfig(ctx context.Context, in ports.SkillConfig) (ports.SkillConfig, error) {
	if !validUUID(in.ID) {
		in.ID = uuid.New().String()
	}
	in.Name = strings.TrimSpace(in.Name)
	var created ports.SkillConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		if findSkillIndex(b, in.ID) >= 0 {
			return errs.Conflict("技能 ID 已存在", "")
		}
		for _, sk := range b.Skills {
			if strings.EqualFold(sk.Name, in.Name) {
				return errs.Conflict("技能名称已存在", "")
			}
		}
		b.Skills = append(b.Skills, in)
		created = in
		return nil
	})
	return created, err
}

// UpdateSkillConfig 更新技能名称。
func (s *Service) UpdateSkillConfig(ctx context.Context, id, name string) (ports.SkillConfig, error) {
	name = strings.TrimSpace(name)
	var out ports.SkillConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		i := findSkillIndex(b, id)
		if i < 0 {
			return errs.NotFound("技能不存在")
		}
		for j, sk := range b.Skills {
			if j != i && strings.EqualFold(sk.Name, name) {
				return errs.Conflict("技能名称已存在", "")
			}
		}
		b.Skills[i].Name = name
		out = b.Skills[i]
		return nil
	})
	return out, err
}

// DeleteSkillConfig 删除技能。
func (s *Service) DeleteSkillConfig(ctx context.Context, id string) error {
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		if findSkillIndex(b, id) < 0 {
			return errs.NotFound("技能不存在")
		}
		removeSkill(b, id)
		return nil
	})
	return err
}

// ListAgentConfigs 列出坐席路由配置。
func (s *Service) ListAgentConfigs(ctx context.Context) ([]ports.AgentConfig, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return nil, err
	}
	_ = appID
	bundle, err := s.loadActiveBundleOrEmpty(ctx, "")
	if err != nil {
		return nil, err
	}
	return append([]ports.AgentConfig(nil), bundle.Agents...), nil
}

// UpsertAgentConfig 创建或更新坐席路由资料。
func (s *Service) UpsertAgentConfig(ctx context.Context, in ports.AgentConfig) (ports.AgentConfig, error) {
	if !validUUID(in.ID) {
		in.ID = uuid.New().String()
	}
	normalizeBundle(&ports.ConfigBundle{Agents: []ports.AgentConfig{in}})
	var out ports.AgentConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		i := findAgentIndex(b, in.ID)
		if i >= 0 {
			b.Agents[i] = in
		} else {
			b.Agents = append(b.Agents, in)
		}
		out = in
		return nil
	})
	return out, err
}

// DeleteAgentConfig 删除坐席路由配置。
func (s *Service) DeleteAgentConfig(ctx context.Context, id string) error {
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		if findAgentIndex(b, id) < 0 {
			return errs.NotFound("坐席不存在")
		}
		removeAgent(b, id)
		return nil
	})
	return err
}

// SetAgentSkills 设置坐席技能。
func (s *Service) SetAgentSkills(ctx context.Context, agentID string, skillIDs []string) (ports.AgentConfig, error) {
	var out ports.AgentConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		i := findAgentIndex(b, agentID)
		if i < 0 {
			return errs.NotFound("坐席不存在")
		}
		b.Agents[i].SkillIDs = append([]string(nil), skillIDs...)
		out = b.Agents[i]
		return nil
	})
	return out, err
}

// ListDIDConfigs 列出 DID 路由。
func (s *Service) ListDIDConfigs(ctx context.Context) ([]ports.DIDConfig, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return nil, err
	}
	_ = appID
	bundle, err := s.loadActiveBundleOrEmpty(ctx, "")
	if err != nil {
		return nil, err
	}
	return append([]ports.DIDConfig(nil), bundle.DIDs...), nil
}

// UpsertDIDConfig 创建或更新 DID（按 id 或新建）。
func (s *Service) UpsertDIDConfig(ctx context.Context, in ports.DIDConfig) (ports.DIDConfig, error) {
	if !validUUID(in.ID) {
		in.ID = uuid.New().String()
	}
	normalizeBundle(&ports.ConfigBundle{DIDs: []ports.DIDConfig{in}})
	var out ports.DIDConfig
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		i := findDIDIndex(b, in.ID)
		if i >= 0 {
			b.DIDs[i] = in
		} else {
			b.DIDs = append(b.DIDs, in)
		}
		out = in
		return nil
	})
	return out, err
}

// DeleteDIDConfig 删除 DID。
func (s *Service) DeleteDIDConfig(ctx context.Context, id string) error {
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		if findDIDIndex(b, id) < 0 {
			return errs.NotFound("DID 不存在")
		}
		removeDID(b, id)
		return nil
	})
	return err
}
