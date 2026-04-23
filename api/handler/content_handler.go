package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/internal/moderation"
)

type ContentHandler struct {
	db     *sql.DB
	filter *moderation.Filter
}

func NewContentHandler(db *sql.DB) *ContentHandler {
	return &ContentHandler{
		db:     db,
		filter: moderation.NewFilter(db),
	}
}

func (h *ContentHandler) ListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := moderation.ListRules(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list rules")
		return
	}
	if rules == nil {
		rules = []*moderation.Rule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h *ContentHandler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Pattern string `json:"pattern"`
		Action  string `json:"action"`
		Scope   string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Pattern == "" {
		writeError(w, http.StatusBadRequest, "name and pattern required")
		return
	}
	if req.Action == "" {
		req.Action = "block"
	}
	if req.Scope == "" {
		req.Scope = "both"
	}

	rule, err := moderation.CreateRule(r.Context(), h.db, req.Name, req.Pattern, req.Action, req.Scope)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.filter.Reload(r.Context())
	writeJSON(w, http.StatusCreated, rule)
}

func (h *ContentHandler) DeleteRule(w http.ResponseWriter, r *http.Request, id string) {
	if err := moderation.DeleteRule(r.Context(), h.db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	h.filter.Reload(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *ContentHandler) ToggleRule(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := moderation.ToggleRule(r.Context(), h.db, id, req.IsActive); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle rule")
		return
	}
	h.filter.Reload(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
