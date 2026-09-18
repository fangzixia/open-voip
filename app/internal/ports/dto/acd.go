package dto

// DispatchRequest ACD 选人请求，由 L3 在 queued 状态发起。
type DispatchRequest struct {
	// CallID 当前通话 ID，用于日志与防重。
	CallID string
	// QueueID 目标队列。
	QueueID string
	// RequireVideo 是否必须 video_capable 坐席。
	RequireVideo bool
	// SkillIDs 所需技能超集，空表示不限制。
	SkillIDs []string
}

// DispatchResult ACD 分配结果。
type DispatchResult struct {
	// AgentID 被选中的坐席 ID，空表示暂无可用坐席。
	AgentID string
}
