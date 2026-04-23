package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bigduu/bodhi-server/internal/audit"
)

type AuditHandler struct {
	db *sql.DB
}

func NewAuditHandler(db *sql.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

func (h *AuditHandler) ListLogs(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	items, total, err := audit.QueryLogs(r.Context(), h.db, action, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query audit logs")
		return
	}
	if items == nil {
		items = []*audit.AuditEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
