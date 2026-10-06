package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"open-call/internal/errs"
	"open-call/internal/store/models"
)

func (d RouterDeps) handleAibotIVRTemplate(w http.ResponseWriter, r *http.Request) {
	queueID := r.URL.Query().Get("queue_id")
	if queueID == "" {
		writeErr(w, errs.InvalidRequest("queue_id 必填"))
		return
	}
	doc := map[string]any{
		"start": "welcome",
		"nodes": map[string]any{
			"welcome": map[string]any{
				"type": "play", "file": "", "prompt": "欢迎语", "next": "to_ai", "timeout_sec": 30,
			},
			"to_ai": map[string]any{
				"type": "route_queue", "queue_id": queueID, "session_type": "audio",
			},
		},
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": doc, "hint": "请为 welcome 节点选择 WAV 素材并发布 IVR，再将 DID 或队列绑定到该流程"})
}

func (d RouterDeps) handleAibotQueueProfileGet(w http.ResponseWriter, r *http.Request) {
	qid := chi.URLParam(r, "queueId")
	var row models.AibotQueueProfile
	err := d.DB.WithContext(r.Context()).Where("queue_id = ?", qid).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		writeJSON(w, http.StatusOK, map[string]any{"queue_id": qid, "system_prompt": ""})
		return
	}
	if err != nil {
		writeErr(w, errs.Internal("读取失败"))
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (d RouterDeps) handleAibotQueueProfilePut(w http.ResponseWriter, r *http.Request) {
	qid := chi.URLParam(r, "queueId")
	var body struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, errs.InvalidRequest("请求体无效"))
		return
	}
	row := models.AibotQueueProfile{
		ID:           uuid.New().String(),
		QueueID:      qid,
		SystemPrompt: body.SystemPrompt,
	}
	var existing models.AibotQueueProfile
	tx := d.DB.WithContext(r.Context())
	if err := tx.Where("queue_id = ?", qid).First(&existing).Error; err == gorm.ErrRecordNotFound {
		row.ID = uuid.New().String()
		if err := tx.Create(&row).Error; err != nil {
			writeErr(w, errs.Internal("保存失败"))
			return
		}
		writeJSON(w, http.StatusOK, row)
		return
	} else if err != nil {
		writeErr(w, errs.Internal("保存失败"))
		return
	}
	existing.SystemPrompt = body.SystemPrompt
	if err := tx.Save(&existing).Error; err != nil {
		writeErr(w, errs.Internal("保存失败"))
		return
	}
	writeJSON(w, http.StatusOK, existing)
}
