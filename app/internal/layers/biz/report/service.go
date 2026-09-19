package report

import (
	"context"
	"time"

	"gorm.io/gorm"

	"open-voip/internal/store/models"
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
	AgentsOnline  int64       `json:"agents_online"`
	AgentsOnCall  int64       `json:"agents_on_call"`
	ActiveCalls   int64       `json:"active_calls"`
	Queues        []QueueLive `json:"queues"`
	GeneratedAt   time.Time   `json:"generated_at"`
}

// Historical 历史聚合。
type Historical struct {
	Total             int64   `json:"total"`
	Answered          int64   `json:"answered"`
	Abandoned         int64   `json:"abandoned"`
	AnswerRate        float64 `json:"answer_rate"`
	AbandonRate       float64 `json:"abandon_rate"`
	AvgWaitSec        float64 `json:"avg_wait_sec"`
	AvgTalkSec        float64 `json:"avg_talk_sec"`
	VideoAnswered     int64   `json:"video_answered"`
	VideoAnswerRate   float64 `json:"video_answer_rate"`
	VideoUpgradeOK    int64   `json:"video_upgrade_ok"`
	ScreenShareCount  int64   `json:"screen_share_count"`
	BusyReasonBreak   int64   `json:"busy_reason_break,omitempty"`
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
	db *gorm.DB
}

// NewService 创建报表服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// Live 秒级实时。
func (s *Service) Live(ctx context.Context) (Live, error) {
	out := Live{Queues: []QueueLive{}, GeneratedAt: time.Now().UTC()}
	_ = s.db.WithContext(ctx).Model(&models.AgentSession{}).Where("state NOT IN ?", []string{"offline"}).Count(&out.AgentsOnline).Error
	_ = s.db.WithContext(ctx).Model(&models.AgentSession{}).Where("state = ?", "on_call").Count(&out.AgentsOnCall).Error
	_ = s.db.WithContext(ctx).Model(&models.Call{}).Where("state IN ?", []string{"queued", "ringing", "active", "held", "ivr", "transferring"}).Count(&out.ActiveCalls).Error
	var queues []models.Queue
	if err := s.db.WithContext(ctx).Find(&queues).Error; err != nil {
		return out, err
	}
	for _, q := range queues {
		item := QueueLive{QueueID: q.ID, Name: q.Name}
		_ = s.db.WithContext(ctx).Model(&models.Call{}).Where("queue_id = ? AND state = ?", q.ID, "queued").Count(&item.Waiting).Error
		var oldest models.Call
		if err := s.db.WithContext(ctx).Where("queue_id = ? AND state = ?", q.ID, "queued").Order("created_at ASC").Limit(1).Find(&oldest).Error; err == nil && oldest.ID != "" {
			item.LongestWaitSec = int(time.Since(oldest.CreatedAt).Seconds())
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
	_ = scoped().Count(&out.Total).Error
	_ = scoped().Where("result = ?", "answered").Count(&out.Answered).Error
	_ = scoped().Where("result IN ?", []string{"abandoned", "timeout"}).Count(&out.Abandoned).Error
	if out.Total > 0 {
		out.AnswerRate = float64(out.Answered) / float64(out.Total)
		out.AbandonRate = float64(out.Abandoned) / float64(out.Total)
	}
	type avgRow struct{ Wait, Talk float64 }
	var avg avgRow
	_ = scoped().Select("COALESCE(AVG(wait_sec),0) as wait, COALESCE(AVG(duration_sec),0) as talk").Scan(&avg).Error
	out.AvgWaitSec, out.AvgTalkSec = avg.Wait, avg.Talk
	_ = scoped().Where("session_type IN ? AND result = ?", []string{"video", "mixed"}, "answered").Count(&out.VideoAnswered).Error
	var videoTotal int64
	_ = scoped().Where("session_type IN ?", []string{"video", "mixed"}).Count(&videoTotal).Error
	if videoTotal > 0 {
		out.VideoAnswerRate = float64(out.VideoAnswered) / float64(videoTotal)
	}
	_ = scoped().Where("video_upgrade_ok = ?", true).Count(&out.VideoUpgradeOK).Error
	_ = scoped().Select("COALESCE(SUM(screen_share_count),0)").Scan(&out.ScreenShareCount).Error
	_ = s.db.WithContext(ctx).Model(&models.AgentStateLog{}).Where("to_state = ? AND reason = ?", "busy", "break").Count(&out.BusyReasonBreak).Error
	return out, nil
}

// AgentUtilization 状态时长统计。
func (s *Service) AgentUtilization(ctx context.Context, from, to string) ([]AgentUtil, error) {
	var logs []models.AgentStateLog
	q := s.db.WithContext(ctx).Order("agent_id, created_at")
	if t, err := time.Parse(time.RFC3339, from); err == nil {
		q = q.Where("created_at >= ?", t)
	}
	if t, err := time.Parse(time.RFC3339, to); err == nil {
		q = q.Where("created_at <= ?", t)
	}
	if err := q.Find(&logs).Error; err != nil {
		return nil, err
	}
	by := map[string]*AgentUtil{}
	var prev *models.AgentStateLog
	for i := range logs {
		cur := logs[i]
		if prev != nil && prev.AgentID == cur.AgentID {
			sec := cur.CreatedAt.Sub(prev.CreatedAt).Seconds()
			u := by[cur.AgentID]
			if u == nil {
				u = &AgentUtil{AgentID: cur.AgentID}
				by[cur.AgentID] = u
			}
			switch prev.ToState {
			case "idle":
				u.IdleSec += sec
			case "on_call", "ringing":
				u.OnCallSec += sec
			case "busy", "acw":
				u.BusySec += sec
			}
		}
		prev = &logs[i]
	}
	out := make([]AgentUtil, 0, len(by))
	for _, u := range by {
		den := u.IdleSec + u.OnCallSec + u.BusySec
		if den > 0 {
			u.Utilization = u.OnCallSec / den
		}
		out = append(out, *u)
	}
	return out, nil
}
