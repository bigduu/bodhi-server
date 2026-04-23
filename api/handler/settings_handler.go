package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bigduu/bodhi-server/internal/models"
)

type SettingsHandler struct {
	db *sql.DB
}

func NewSettingsHandler(db *sql.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

func (h *SettingsHandler) ListSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := models.ListSettings(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) UpdateSetting(w http.ResponseWriter, r *http.Request, key string) {
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := models.SetSetting(r.Context(), h.db, key, req.Value); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update setting")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *SettingsHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxUses   int     `json:"max_uses"`
		ExpiresAt *string `json:"expires_at"`
	}
	req.MaxUses = 1
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID := r.Header.Get("X-User-ID")
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err == nil {
			expiresAt = &t
		}
	}

	code, err := models.CreateInviteCode(r.Context(), h.db, userID, req.MaxUses, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create invite code")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"code": code})
}

func (h *SettingsHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
	codes, err := models.ListInviteCodes(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list invites")
		return
	}
	writeJSON(w, http.StatusOK, codes)
}

func (h *SettingsHandler) DeleteInvite(w http.ResponseWriter, r *http.Request, code string) {
	if err := models.DeleteInviteCode(r.Context(), h.db, code); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete invite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
