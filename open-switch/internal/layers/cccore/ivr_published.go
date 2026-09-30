package cccore

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
)

// ListIVRFlows 列出激活配置中的已发布 IVR。
func (s *Service) ListIVRFlows(ctx context.Context) ([]ports.IVRPublishedView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return nil, err
	}
	_ = appID
	bundle, err := s.loadActiveBundleOrEmpty(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]ports.IVRPublishedView, 0, len(bundle.IVRs))
	for _, ivr := range bundle.IVRs {
		out = append(out, ports.IVRPublishedView{
			FlowID: ivr.FlowID, Version: ivr.Version, PayloadJSON: ivr.PayloadJSON,
		})
	}
	return out, nil
}

// GetIVRFlow 读取激活配置中的已发布 IVR。
func (s *Service) GetIVRFlow(ctx context.Context, flowID string) (ports.IVRPublishedView, error) {
	appID, err := applicationID(ctx)
	if err != nil {
		return ports.IVRPublishedView{}, err
	}
	_ = appID
	bundle, err := s.loadActiveBundleOrEmpty(ctx, "")
	if err != nil {
		return ports.IVRPublishedView{}, err
	}
	for _, ivr := range bundle.IVRs {
		if ivr.FlowID == flowID {
			return ports.IVRPublishedView{
				FlowID: ivr.FlowID, Version: ivr.Version, PayloadJSON: ivr.PayloadJSON,
			}, nil
		}
	}
	return ports.IVRPublishedView{}, errs.NotFound("IVR 流程不存在")
}

// UpsertIVRFlow 将业务系统提交的有效 IVR payload 写入激活配置并生成新版本。
func (s *Service) UpsertIVRFlow(ctx context.Context, flowID, payloadJSON string) (ports.IVRPublishedView, error) {
	flowID = strings.TrimSpace(flowID)
	if !validUUID(flowID) {
		flowID = uuid.New().String()
	}
	payload := strings.TrimSpace(payloadJSON)
	if !json.Valid([]byte(payload)) {
		return ports.IVRPublishedView{}, errs.InvalidRequest("IVR payload JSON 无效")
	}
	var view ports.IVRPublishedView
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		next := 1
		for _, ivr := range b.IVRs {
			if ivr.FlowID == flowID && ivr.Version >= next {
				next = ivr.Version + 1
			}
		}
		replaced := false
		for i, ivr := range b.IVRs {
			if ivr.FlowID == flowID {
				b.IVRs[i] = ports.IVRConfig{FlowID: flowID, Version: next, PayloadJSON: payload}
				replaced = true
				break
			}
		}
		if !replaced {
			b.IVRs = append(b.IVRs, ports.IVRConfig{FlowID: flowID, Version: next, PayloadJSON: payload})
		}
		view = ports.IVRPublishedView{FlowID: flowID, Version: next, PayloadJSON: payload}
		return nil
	})
	return view, err
}

// DeleteIVRFlow 从激活配置中移除 IVR；若仍有 DID 引用则拒绝。
func (s *Service) DeleteIVRFlow(ctx context.Context, flowID string) error {
	_, err := s.ApplyConfigMutation(ctx, func(b *ports.ConfigBundle) error {
		found := false
		out := b.IVRs[:0]
		for _, ivr := range b.IVRs {
			if ivr.FlowID == flowID {
				found = true
				continue
			}
			out = append(out, ivr)
		}
		if !found {
			return errs.NotFound("IVR 流程不存在")
		}
		b.IVRs = out
		for i := range b.Queues {
			if b.Queues[i].IVRFlowID == flowID {
				b.Queues[i].IVRFlowID = ""
			}
		}
		for _, d := range b.DIDs {
			if d.TargetType == "ivr" && d.TargetID == flowID {
				return errs.InvalidRequest("仍有 DID 引用该 IVR")
			}
		}
		return nil
	})
	return err
}
