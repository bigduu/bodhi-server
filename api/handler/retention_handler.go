package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bigduu/bodhi-server/internal/retention"
)

type RetentionHandler struct {
	db     *sql.DB
	purger *retention.Purger
}

func NewRetentionHandler(db *sql.DB, purger *retention.Purger) *RetentionHandler {
	return &RetentionHandler{db: db, purger: purger}
}

func (h *RetentionHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := retention.ListPolicies(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list policies")
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

func (h *RetentionHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request, tableName string) {
	var req struct {
		RetentionDays int  `json:"retention_days"`
		Enabled       bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := retention.UpdatePolicy(r.Context(), h.db, tableName, req.RetentionDays, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update policy")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *RetentionHandler) TriggerPurge(w http.ResponseWriter, r *http.Request) {
	// Detach from the request context (which is cancelled once the handler
	// returns) so the background purge can run to completion.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		h.purger.RunOnce(ctx)
	}()
	writeJSON(w, http.StatusOK, map[string]string{"status": "purge started"})
}
