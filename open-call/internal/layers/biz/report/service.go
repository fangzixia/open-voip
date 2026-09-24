package report

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/ports"
	"open-call/internal/store/models"
)

// QueueLive 队列实时。
type QueueLive struct {
	QueueID        string `json:"queue_id"`
	Name           string `json:"name"`
	Waiting        int64  `json:"waiting"`
	LongestWaitSec int    `json:"longest_wait_sec"`
}

// Live 实时报表。
type Live struct {
	AgentsOnline int64       `json:"agents_online"`
	AgentsOnCall int64       `json:"agents_on_call"`
	ActiveCalls  int64       `json:"active_calls"`
	Queues       []QueueLive `json:"queues"`
	GeneratedAt  time.Time   `json:"generated_at"`
}

// Historical 历史聚合。
type Historical struct {
	Total            int64   `json:"total"`
	Answered         int64   `json:"answered"`
	Abandoned        int64   `json:"abandoned"`
	AnswerRate       float64 `json:"answer_rate"`
	AbandonRate      float64 `json:"abandon_rate"`
	AvgWaitSec       float64 `json:"avg_wait_sec"`
	AvgTalkSec       float64 `json:"avg_talk_sec"`
	VideoAnswered    int64   `json:"video_answered"`
	VideoAnswerRate  float64 `json:"video_answer_rate"`
	VideoUpgradeOK   int64   `json:"video_upgrade_ok"`
	ScreenShareCount int64   `json:"screen_share_count"`
	BusyReasonBreak  int64   `json:"busy_reason_break,omitempty"`
}

// AgentUtil 坐席利用率。
type AgentUtil struct {
	AgentID     string  `json:"agent_id"`
	IdleSec     float64 `json:"idle_sec"`
	OnCallSec   float64 `json:"on_call_sec"`
	BusySec     float64 `json:"busy_sec"`
	Utilization float64 `json:"utilization"`
}

// Service 报表查询（只读 calls/cdr/sessions）。
type Service struct {
	db      *gorm.DB
	runtime interface {
		ListCalls(context.Context) ([]ports.CallView, error)
	}
}

// NewService 创建报表服务。
func NewService(db *gorm.DB, runtime interface {
	ListCalls(context.Context) ([]ports.CallView, error)
}) *Service {
	return &Service{db: db, runtime: runtime}
}

// Live 秒级实时。
func (s *Service) Live(ctx context.Context) (Live, error) {
	out := Live{Queues: []QueueLive{}, GeneratedAt: time.Now().UTC()}
	if s.runtime == nil {
		return out, errs.NotImplemented("交换服务未配置")
	}
	calls, err := s.runtime.ListCalls(ctx)
	if err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&models.AgentSession{}).Where("state <> ?", "offline").Count(&out.AgentsOnline).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&models.AgentSession{}).Where("state = ?", "on_call").Count(&out.AgentsOnCall).Error; err != nil {
		return out, err
	}
	out.ActiveCalls = int64(len(calls))
	var queues []models.Queue
	if err := s.db.WithContext(ctx).Find(&queues).Error; err != nil {
		return out, err
	}
	for _, q := range queues {
		item := QueueLive{QueueID: q.ID, Name: q.Name}
		for _, call := range calls {
			if call.QueueID == q.ID && call.State == "queued" {
				item.Waiting++
				wait := int(time.Since(call.CreatedAt).Seconds())
				if wait > item.LongestWaitSec {
					item.LongestWaitSec = wait
				}
			}
		}
		out.Queues = append(out.Queues, item)
	}
	return out, nil
}

// Historical 按日期与队列聚合。
func (s *Service) Historical(ctx context.Context, from, to, queueID string) (Historical, error) {
	scoped := func() *gorm.DB {
		q := s.db.WithContext(ctx).Model(&models.CDR{})
		if queueID != "" {
			q = q.Where("queue_id = ?", queueID)
		}
		if t, err := time.Parse("2006-01-02", from); err == nil {
			q = q.Where("started_at >= ?", t)
		} else if t, err := time.Parse(time.RFC3339, from); err == nil {
			q = q.Where("started_at >= ?", t)
		}
		if t, err := time.Parse("2006-01-02", to); err == nil {
			q = q.Where("started_at < ?", t.Add(24*time.Hour))
		} else if t, err := time.Parse(time.RFC3339, to); err == nil {
			q = q.Where("started_at <= ?", t)
		}
		return q
	}
	var out Historical
	if err := scoped().Count(&out.Total).Error; err != nil {
		return out, err
	}
	if err := scoped().Where("result = ?", "answered").Count(&out.Answered).Error; err != nil {
		return out, err
	}
	if err := scoped().Where("result IN ?", []string{"abandoned", "timeout"}).Count(&out.Abandoned).Error; err != nil {
		return out, err
	}
	if out.Total > 0 {
		out.AnswerRate = float64(out.Answered) / float64(out.Total)
		out.AbandonRate = float64(out.Abandoned) / float64(out.Total)
	}
	type avgRow struct{ Wait, Talk float64 }
	var avg avgRow
	if err := scoped().Select("COALESCE(AVG(wait_sec),0) as wait, COALESCE(AVG(duration_sec),0) as talk").Scan(&avg).Error; err != nil {
		return out, err
	}
	out.AvgWaitSec, out.AvgTalkSec = avg.Wait, avg.Talk
	if err := scoped().Where("session_type IN ? AND result = ?", []string{"video", "mixed"}, "answered").Count(&out.VideoAnswered).Error; err != nil {
		return out, err
	}
	var videoTotal int64
	if err := scoped().Where("session_type IN ?", []string{"video", "mixed"}).Count(&videoTotal).Error; err != nil {
		return out, err
	}
	if videoTotal > 0 {
		out.VideoAnswerRate = float64(out.VideoAnswered) / float64(videoTotal)
	}
	if err := scoped().Where("video_upgrade_ok = ?", true).Count(&out.VideoUpgradeOK).Error; err != nil {
		return out, err
	}
	if err := scoped().Select("COALESCE(SUM(screen_share_count),0)").Scan(&out.ScreenShareCount).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&models.AgentStateLog{}).Where("to_state = ? AND reason = ?", "busy", "break").Count(&out.BusyReasonBreak).Error; err != nil {
		return out, err
	}
	return out, nil
}

// AgentUtilization 状态时长统计。
func (s *Service) AgentUtilization(ctx context.Context, from, to string) ([]AgentUtil, error) {
	start, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return nil, errs.InvalidRequest("from 必须为 RFC3339 时间")
	}
	end, err := time.Parse(time.RFC3339, to)
	if err != nil || !end.After(start) {
		return nil, errs.InvalidRequest("to 必须为晚于 from 的 RFC3339 时间")
	}
	if end.Sub(start) > 366*24*time.Hour {
		return nil, errs.InvalidRequest("单次查询范围不能超过 366 天")
	}

	var initial []models.AgentStateLog
	if err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT ON (agent_id) * FROM oc_agent_state_log WHERE created_at < ? ORDER BY agent_id, created_at DESC`, start).Scan(&initial).Error; err != nil {
		return nil, err
	}
	var logs []models.AgentStateLog
	if err := s.db.WithContext(ctx).Where("created_at >= ? AND created_at <= ?", start, end).Order("agent_id, created_at, id").Find(&logs).Error; err != nil {
		return nil, err
	}
	type stateCursor struct {
		state string
		at    time.Time
	}
	states := map[string]stateCursor{}
	by := map[string]*AgentUtil{}
	for _, row := range initial {
		states[row.AgentID] = stateCursor{state: row.ToState, at: start}
		by[row.AgentID] = &AgentUtil{AgentID: row.AgentID}
	}
	add := func(u *AgentUtil, state string, sec float64) {
		if sec <= 0 {
			return
		}
		switch state {
		case "idle":
			u.IdleSec += sec
		case "on_call", "ringing":
			u.OnCallSec += sec
		case "busy", "acw":
			u.BusySec += sec
		}
	}
	for _, cur := range logs {
		u := by[cur.AgentID]
		if u == nil {
			u = &AgentUtil{AgentID: cur.AgentID}
			by[cur.AgentID] = u
		}
		st, ok := states[cur.AgentID]
		if !ok {
			st = stateCursor{state: cur.FromState, at: cur.CreatedAt}
		}
		add(u, st.state, cur.CreatedAt.Sub(st.at).Seconds())
		states[cur.AgentID] = stateCursor{state: cur.ToState, at: cur.CreatedAt}
	}
	for agentID, st := range states {
		add(by[agentID], st.state, end.Sub(st.at).Seconds())
	}
	out := make([]AgentUtil, 0, len(by))
	for _, u := range by {
		den := u.IdleSec + u.OnCallSec + u.BusySec
		if den > 0 {
			u.Utilization = u.OnCallSec / den
		}
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out, nil
}
