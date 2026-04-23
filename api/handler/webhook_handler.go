package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/internal/webhook"
)

type WebhookHandler struct {
	db         *sql.DB
	dispatcher *webhook.Dispatcher
}

func NewWebhookHandler(db *sql.DB, dispatcher *webhook.Dispatcher) *WebhookHandler {
	return &WebhookHandler{db: db, dispatcher: dispatcher}
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	hooks, err := webhook.ListWebhooks(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list webhooks")
		return
	}
	if hooks == nil {
		hooks = []*webhook.Webhook{}
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Secret string   `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" || len(req.Events) == 0 {
		writeError(w, http.StatusBadRequest, "url and events required")
		return
	}

	wh, err := webhook.CreateWebhook(r.Context(), h.db, req.URL, req.Events, req.Secret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}
	h.dispatcher.Reload(r.Context())
	writeJSON(w, http.StatusCreated, wh)
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request, id string) {
	if err := webhook.DeleteWebhook(r.Context(), h.db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		return
	}
	h.dispatcher.Reload(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *WebhookHandler) Toggle(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := webhook.ToggleWebhook(r.Context(), h.db, id, req.IsActive); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle webhook")
		return
	}
	h.dispatcher.Reload(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
