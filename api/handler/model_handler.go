package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/internal/cache"
	"github.com/bigduu/bodhi-server/internal/models"
)

type ModelHandler struct {
	db *sql.DB
}

func NewModelHandler(db *sql.DB) *ModelHandler {
	return &ModelHandler{db: db}
}

func (h *ModelHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models_list, err := models.ListModels(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}
	writeJSON(w, http.StatusOK, models_list)
}

func (h *ModelHandler) ListActiveModels(w http.ResponseWriter, r *http.Request) {
	models_list, err := models.ListActiveModels(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}
	writeJSON(w, http.StatusOK, models_list)
}

func (h *ModelHandler) CreateModel(w http.ResponseWriter, r *http.Request) {
	var m models.Model
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if m.Name == "" || m.Provider == "" {
		writeError(w, http.StatusBadRequest, "name and provider required")
		return
	}
	if err := models.CreateModel(r.Context(), h.db, &m); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create model")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *ModelHandler) UpdateModel(w http.ResponseWriter, r *http.Request, id string) {
	var m models.Model
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := models.UpdateModel(r.Context(), h.db, id, &m); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update model")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *ModelHandler) DeleteModel(w http.ResponseWriter, r *http.Request, id string) {
	if err := models.DeleteModel(r.Context(), h.db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete model")
		return
	}
cache.InvalidateRouting()
	w.WriteHeader(http.StatusNoContent)
}
