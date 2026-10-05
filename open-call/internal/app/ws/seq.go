package ws

import (
	"context"
	"encoding/json"
	"log/slog"

	"gorm.io/gorm"
)

func (h *Hub) loadSeqFromDB(ctx context.Context) {
	if h.db == nil {
		return
	}
	var seq uint64
	if err := h.db.WithContext(ctx).Raw(`SELECT last_seq FROM oc_ws_event_seq WHERE id = 1`).Scan(&seq).Error; err != nil {
		h.log.Warn("加载 WS 序号失败", "err", err)
		return
	}
	if seq > 0 {
		h.sequence.Store(seq)
	}
}

func (h *Hub) allocateSeq(ctx context.Context) uint64 {
	if h.db == nil {
		return h.sequence.Add(1)
	}
	var seq uint64
	err := h.db.WithContext(ctx).Raw(`
UPDATE oc_ws_event_seq SET last_seq = last_seq + 1, updated_at = NOW() WHERE id = 1 RETURNING last_seq`).Scan(&seq).Error
	if err != nil || seq == 0 {
		h.log.Warn("分配 WS 序号失败，使用进程内序号", "err", err)
		return h.sequence.Add(1)
	}
	h.sequence.Store(seq)
	return seq
}

func (h *Hub) persistBufferedEvent(ctx context.Context, callID, agentID string, msg envelope) {
	if h.db == nil {
		return
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if err := h.db.WithContext(ctx).Exec(
		`INSERT INTO oc_ws_event_buffer (seq, call_id, agent_id, payload) VALUES (?, ?, ?, ?::jsonb) ON CONFLICT (seq) DO NOTHING`,
		msg.Seq, callID, agentID, string(raw),
	).Error; err != nil {
		h.log.Warn("持久化 WS 事件失败", "seq", msg.Seq, "err", err)
		return
	}
	_ = h.db.WithContext(ctx).Exec(`
DELETE FROM oc_ws_event_buffer
WHERE seq <= (SELECT COALESCE(MIN(seq), 0) FROM (SELECT seq FROM oc_ws_event_buffer ORDER BY seq DESC LIMIT 1000) keep)`).Error
}

func replayFromDB(ctx context.Context, db *gorm.DB, log *slog.Logger, cl *client, since uint64) {
	if db == nil || since == 0 {
		return
	}
	agentID := cl.principal.AgentID
	callID := cl.principal.GuestCallID
	var payloads []json.RawMessage
	err := db.WithContext(ctx).Raw(`
SELECT payload FROM oc_ws_event_buffer
WHERE seq > ? AND (agent_id = ? OR (call_id <> '' AND call_id = ?))
ORDER BY seq ASC LIMIT 500`, since, agentID, callID).Scan(&payloads).Error
	if err != nil {
		if log != nil {
			log.Warn("WS 补发查询失败", "err", err)
		}
		return
	}
	for _, raw := range payloads {
		var msg envelope
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if err := cl.sendWait(ctx, msg); err != nil {
			return
		}
	}
}
