package queue

import (
	"open-call/internal/ports"
)

func switchToDTO(q ports.SwitchQueueConfig) DTO {
	return DTO{
		ID:                    q.ID,
		Name:                  q.Name,
		VideoEnabled:          q.VideoEnabled,
		MaxWaitSec:            q.MaxWaitSec,
		Strategy:              q.DispatchStrategy,
		OverflowPolicy:        q.OverflowAction,
		OverflowQueueID:       q.OverflowQueueID,
		RecordingPolicy:       q.RecordingPolicy,
		IVRFlowID:             q.IVRFlowID,
		WaitPrompt:            q.WaitPrompt,
		AnnounceRecording:     q.AnnounceRecording,
		PriorityEnabled:       q.PriorityEnabled,
		SkillIDs:              q.SkillIDs,
		BusinessHoursJSON:     q.BusinessHoursJSON,
		AfterHoursAction:      q.AfterHoursAction,
		ForceHangupOnCheckout: q.ForceHangupOnCheckout,
		ListenAnnounce:        q.ListenAnnounce,
	}
}

func createToSwitch(in CreateInput) ports.SwitchQueueConfig {
	force := true
	if in.ForceHangupOnCheckout != nil {
		force = *in.ForceHangupOnCheckout
	}
	listen := in.ListenAnnounce != nil && *in.ListenAnnounce
	return ports.SwitchQueueConfig{
		Name:                  in.Name,
		VideoEnabled:          in.VideoEnabled,
		MaxWaitSec:            in.MaxWaitSec,
		DispatchStrategy:      in.Strategy,
		RecordingPolicy:       in.RecordingPolicy,
		OverflowAction:        in.OverflowPolicy,
		OverflowQueueID:       in.OverflowQueueID,
		IVRFlowID:             in.IVRFlowID,
		WaitPrompt:            in.WaitPrompt,
		AnnounceRecording:     in.AnnounceRecording,
		PriorityEnabled:       in.PriorityEnabled,
		BusinessHoursJSON:     in.BusinessHoursJSON,
		AfterHoursAction:      in.AfterHoursAction,
		ForceHangupOnCheckout: force,
		ListenAnnounce:        listen,
		SkillIDs:              in.SkillIDs,
		AgentIDs:              []string{},
	}
}
