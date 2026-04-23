package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/internal/cache"
	"github.com/bigduu/bodhi-server/internal/proxy"
)

type InstanceHandler struct {
	db *sql.DB
}

func NewInstanceHandler(db *sql.DB) *InstanceHandler {
	return &InstanceHandler{db: db}
}

func (h *InstanceHandler) ListInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := proxy.ListProviderInstances(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list instances")
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

func (h *InstanceHandler) CreateInstance(w http.ResponseWriter, r *http.Request) {
	var pi proxy.ProviderInstance
	if err := json.NewDecoder(r.Body).Decode(&pi); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if pi.ModelID == "" || pi.ProviderType == "" || pi.InstanceName == "" {
		writeError(w, http.StatusBadRequest, "model_id, provider_type, instance_name required")
		return
	}
	if err := proxy.CreateProviderInstance(r.Context(), h.db, &pi); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create instance")
		return
	}
	writeJSON(w, http.StatusCreated, pi)
}

func (h *InstanceHandler) UpdateInstance(w http.ResponseWriter, r *http.Request, id string) {
	var pi proxy.ProviderInstance
	if err := json.NewDecoder(r.Body).Decode(&pi); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := proxy.UpdateProviderInstance(r.Context(), h.db, id, &pi); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update instance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *InstanceHandler) DeleteInstance(w http.ResponseWriter, r *http.Request, id string) {
	if err := proxy.DeleteProviderInstance(r.Context(), h.db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete instance")
		return
	}
cache.InvalidateRouting()
	w.WriteHeader(http.StatusNoContent)
}
