package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/bigduu/bodhi-server/internal/models"
)

type BillingHandler struct {
	db *sql.DB
}

func NewBillingHandler(db *sql.DB) *BillingHandler {
	return &BillingHandler{db: db}
}

func (h *BillingHandler) CurrentUsage(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	report, err := models.GetCurrentUsage(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get current usage")
		return
	}

	byModel, _ := models.GetCurrentUsageByModel(r.Context(), h.db, userID)
	balance, _ := models.GetBalance(r.Context(), h.db, userID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current":  report,
		"by_model": byModel,
		"balance_cents": balance,
	})
}

func (h *BillingHandler) MonthlyReport(w http.ResponseWriter, r *http.Request, userID string) {
	year, _ := strconv.Atoi(r.PathValue("year"))
	month, _ := strconv.Atoi(r.PathValue("month"))
	if year == 0 || month == 0 {
		writeError(w, http.StatusBadRequest, "year and month required")
		return
	}

	report, err := models.GenerateMonthlyReport(r.Context(), h.db, userID, year, time.Month(month))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate report")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *BillingHandler) ExportCSV(w http.ResponseWriter, r *http.Request, userID string) {
	year, _ := strconv.Atoi(r.PathValue("year"))
	month, _ := strconv.Atoi(r.PathValue("month"))
	if year == 0 || month == 0 {
		writeError(w, http.StatusBadRequest, "year and month required")
		return
	}

	data, err := models.ExportCSVRaw(r.Context(), h.db, userID, year, time.Month(month))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=billing.csv")
	w.Write(data)
}

func (h *BillingHandler) ListReports(w http.ResponseWriter, r *http.Request, userID string) {
	reports, err := models.ListBillingReports(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list reports")
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

func (h *BillingHandler) AddBalance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Amount int    `json:"amount_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" || req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "user_id and positive amount_cents required")
		return
	}
	if err := models.AddBalance(r.Context(), h.db, req.UserID, req.Amount); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add balance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
