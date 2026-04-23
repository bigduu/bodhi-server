package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/internal/config"
	"github.com/bigduu/bodhi-server/internal/crypto"
	"github.com/bigduu/bodhi-server/internal/models"
)

type GroupHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func NewGroupHandler(db *sql.DB, cfg *config.Config) *GroupHandler {
	return &GroupHandler{db: db, cfg: cfg}
}

func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	userID := r.Header.Get("X-User-ID")
	group, err := models.CreateGroup(r.Context(), h.db, req.Name, req.Description, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}

	// Auto-add creator as admin
	models.AddGroupMember(r.Context(), h.db, group.ID, userID, "admin")
	writeJSON(w, http.StatusCreated, group)
}

func (h *GroupHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := models.ListGroups(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	if groups == nil {
		groups = []*models.Group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *GroupHandler) GetGroup(w http.ResponseWriter, r *http.Request, id string) {
	group, err := models.GetGroup(r.Context(), h.db, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (h *GroupHandler) UpdateGroup(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := models.UpdateGroup(r.Context(), h.db, id, req.Name, req.Description); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update group")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *GroupHandler) DeleteGroup(w http.ResponseWriter, r *http.Request, id string) {
	if err := models.DeleteGroup(r.Context(), h.db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *GroupHandler) ListMembers(w http.ResponseWriter, r *http.Request, groupID string) {
	members, err := models.ListGroupMembers(r.Context(), h.db, groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	if members == nil {
		members = []*models.GroupMember{}
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *GroupHandler) AddMember(w http.ResponseWriter, r *http.Request, groupID string) {
	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if err := models.AddGroupMember(r.Context(), h.db, groupID, req.UserID, req.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

func (h *GroupHandler) RemoveMember(w http.ResponseWriter, r *http.Request, groupID, userID string) {
	if err := models.RemoveGroupMember(r.Context(), h.db, groupID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *GroupHandler) ListCredentials(w http.ResponseWriter, r *http.Request, groupID string) {
	creds, err := models.ListGroupCredentials(r.Context(), h.db, groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}
	if creds == nil {
		creds = []*models.GroupCredential{}
	}
	writeJSON(w, http.StatusOK, creds)
}

func (h *GroupHandler) SetCredential(w http.ResponseWriter, r *http.Request, groupID, provider string) {
	var req struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key required")
		return
	}

	// Get encryption key from config — we need it passed through context or stored
	// For simplicity, store encrypted. The handler needs the encryption key.
	// We'll get it from the handler's config.
	encrypted, err := crypto.Encrypt(req.APIKey, h.cfg.Auth.EncryptionKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	var baseURL sql.NullString
	if req.BaseURL != "" {
		baseURL = sql.NullString{String: req.BaseURL, Valid: true}
	}

	if err := models.SetGroupCredential(r.Context(), h.db, groupID, provider, encrypted, baseURL); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set credential")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *GroupHandler) DeleteCredential(w http.ResponseWriter, r *http.Request, groupID, provider string) {
	if err := models.DeleteGroupCredential(r.Context(), h.db, groupID, provider); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete credential")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *GroupHandler) GetQuota(w http.ResponseWriter, r *http.Request, groupID string) {
	quota, err := models.GetGroupQuota(r.Context(), h.db, groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get quota")
		return
	}
	writeJSON(w, http.StatusOK, quota)
}

func (h *GroupHandler) SetQuota(w http.ResponseWriter, r *http.Request, groupID string) {
	var q models.GroupQuota
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	q.GroupID = groupID
	if err := models.SetGroupQuota(r.Context(), h.db, &q); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set quota")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
