package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/api/dto"
	"github.com/bigduu/bodhi-server/internal/auth"
	"github.com/bigduu/bodhi-server/internal/models"
)

type KeyHandler struct {
	db *sql.DB
}

func NewKeyHandler(db *sql.DB) *KeyHandler {
	return &KeyHandler{db: db}
}

func (h *KeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req dto.CreateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	key, fullKey, err := models.CreateAPIKey(r.Context(), h.db, userID, req.Name, req.ExpiresAt, req.AllowedModels, req.AllowedProviders, req.IPWhitelist)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	resp := dto.APIKeyResponse{
		ID:        key.ID,
		Name:      key.Name,
		Key:       fullKey,
		KeyPrefix: key.KeyPrefix,
		KeySuffix: key.KeySuffix,
		IsActive:  key.IsActive,
		CreatedAt: key.CreatedAt,
	}
	if key.ExpiresAt.Valid {
		resp.ExpiresAt = &key.ExpiresAt.Time
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *KeyHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	keys, err := models.ListAPIKeys(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}

	resp := make([]dto.APIKeyResponse, 0, len(keys))
	for _, k := range keys {
		item := dto.APIKeyResponse{
			ID:        k.ID,
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			KeySuffix: k.KeySuffix,
			IsActive:  k.IsActive,
			CreatedAt: k.CreatedAt,
		}
		if k.ExpiresAt.Valid {
			item.ExpiresAt = &k.ExpiresAt.Time
		}
		if k.LastUsedAt.Valid {
			item.LastUsedAt = &k.LastUsedAt.Time
		}
		resp = append(resp, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *KeyHandler) Delete(w http.ResponseWriter, r *http.Request, keyID string) {
	userID := auth.UserIDFromContext(r.Context())

	if err := models.DeleteAPIKey(r.Context(), h.db, userID, keyID); err != nil {
		if err == models.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "API key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete API key")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *KeyHandler) Rotate(w http.ResponseWriter, r *http.Request, keyID string) {
	userID := auth.UserIDFromContext(r.Context())

	var req dto.RotateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	key, fullKey, err := models.RotateAPIKey(r.Context(), h.db, userID, keyID, req.Name, req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rotate API key")
		return
	}

	resp := dto.APIKeyResponse{
		ID:        key.ID,
		Name:      key.Name,
		Key:       fullKey,
		KeyPrefix: key.KeyPrefix,
		KeySuffix: key.KeySuffix,
		IsActive:  key.IsActive,
		CreatedAt: key.CreatedAt,
	}
	if key.ExpiresAt.Valid {
		resp.ExpiresAt = &key.ExpiresAt.Time
	}

	writeJSON(w, http.StatusOK, resp)
}
