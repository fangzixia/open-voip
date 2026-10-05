// 本文件负责Webhook 订阅与投递接口。
package http

import (
	"net/http"
	"open-call/internal/layers/biz/webhook"

	"github.com/go-chi/chi/v5"
)

func (d RouterDeps) handleWebhookList(w http.ResponseWriter, r *http.Request) {
	out, err := d.Webhooks.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleWebhookCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
		Secret     string   `json:"secret"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, err)
		return
	}
	out, err := d.Webhooks.Create(r.Context(), body.URL, body.EventTypes, body.Secret)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleWebhookUpdate 更新、停用或轮换订阅密钥。
func (d RouterDeps) handleWebhookUpdate(w http.ResponseWriter, r *http.Request) {
	var in webhook.UpdateInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	out, secret, err := d.Webhooks.Update(r.Context(), chi.URLParam(r, "subscriptionId"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	response := map[string]any{"subscription": out}
	if secret != "" {
		response["secret"] = secret
	}
	writeJSON(w, http.StatusOK, response)
}

// handleWebhookDelete 删除订阅和所属投递历史。
func (d RouterDeps) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	if err := d.Webhooks.Delete(r.Context(), chi.URLParam(r, "subscriptionId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

// handleWebhookDeliveryList 分页查询投递任务和死信。
func (d RouterDeps) handleWebhookDeliveryList(w http.ResponseWriter, r *http.Request) {
	page, size := pageParams(r)
	query := r.URL.Query()
	out, err := d.Webhooks.ListDeliveries(r.Context(), page, size, query.Get("status"), query.Get("event_type"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (d RouterDeps) handleWebhookRetry(w http.ResponseWriter, r *http.Request) {
	if err := d.Webhooks.Retry(r.Context(), chi.URLParam(r, "deliveryId")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, nil)
}
