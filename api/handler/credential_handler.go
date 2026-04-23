package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/api/dto"
	"github.com/bigduu/bodhi-server/internal/auth"
	"github.com/bigduu/bodhi-server/internal/cache"
	"github.com/bigduu/bodhi-server/internal/config"
	"github.com/bigduu/bodhi-server/internal/crypto"
	"github.com/bigduu/bodhi-server/internal/models"
)

type CredentialHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func NewCredentialHandler(db *sql.DB, cfg *config.Config) *CredentialHandler {
	return &CredentialHandler{db: db, cfg: cfg}
}

func (h *CredentialHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req dto.CreateCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	encrypted, err := crypto.Encrypt(req.APIKey, h.cfg.Auth.EncryptionKey)
	if err != nil {
		writeInternalError(w, err, "failed to encrypt API key")
		return
	}

	_, err = models.CreateCredential(r.Context(), h.db, userID, req.Provider, encrypted, req.BaseURL)
	if err != nil {
		writeInternalError(w, err, "failed to store credential")
		return
	}

	cache.InvalidateCredential(userID, req.Provider)
	writeJSON(w, http.StatusCreated, dto.CredentialResponse{
		Provider: req.Provider,
		BaseURL:  req.BaseURL,
		IsActive: true,
	})
}

func (h *CredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	creds, err := models.ListCredentials(r.Context(), h.db, userID)
	if err != nil {
		writeInternalError(w, err, "failed to list credentials")
		return
	}

	resp := make([]dto.CredentialResponse, 0, len(creds))
	for _, c := range creds {
		resp = append(resp, dto.CredentialResponse{
			Provider:  c.Provider,
			BaseURL:   c.BaseURL.String,
			IsActive:  c.IsActive,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *CredentialHandler) Update(w http.ResponseWriter, r *http.Request, provider string) {
	userID := auth.UserIDFromContext(r.Context())

	var req dto.CreateCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	encrypted, err := crypto.Encrypt(req.APIKey, h.cfg.Auth.EncryptionKey)
	if err != nil {
		writeInternalError(w, err, "failed to encrypt API key")
		return
	}

	_, err = models.CreateCredential(r.Context(), h.db, userID, provider, encrypted, req.BaseURL)
	if err != nil {
		writeInternalError(w, err, "failed to update credential")
		return
	}

	cache.InvalidateCredential(userID, provider)
	writeJSON(w, http.StatusOK, dto.CredentialResponse{
		Provider: provider,
		BaseURL:  req.BaseURL,
		IsActive: true,
	})
}

func (h *CredentialHandler) Delete(w http.ResponseWriter, r *http.Request, provider string) {
	userID := auth.UserIDFromContext(r.Context())

	if err := models.DeleteCredential(r.Context(), h.db, userID, provider); err != nil {
		if err == models.ErrCredentialNotFound {
			writeError(w, http.StatusNotFound, "credential not found")
			return
		}
		writeInternalError(w, err, "failed to delete credential")
		return
	}

	cache.InvalidateCredential(userID, provider)
	w.WriteHeader(http.StatusNoContent)
}
