package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bigduu/bodhi-server/internal/models"
	"github.com/bigduu/bodhi-server/internal/pricing"
	"github.com/bigduu/bodhi-server/internal/cache"
	"github.com/bigduu/bodhi-server/internal/quota"
)

type AdminHandler struct {
	db *sql.DB
}

func NewAdminHandler(db *sql.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	users, total, err := models.ListUsers(r.Context(), h.db, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": users,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request, userID string) {
	user, err := models.GetUserByID(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request, userID string) {
	var req struct {
		IsActive *bool `json:"is_active,omitempty"`
		IsAdmin  *bool `json:"is_admin,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := models.UpdateUserStatus(r.Context(), h.db, userID, req.IsActive, req.IsAdmin); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request, userID string) {
	if err := models.DeleteUser(r.Context(), h.db, userID); err != nil {
		if err == models.ErrUserNotFound {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListUserKeys(w http.ResponseWriter, r *http.Request, userID string) {
	keys, err := models.ListAPIKeys(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list keys")
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (h *AdminHandler) DeleteUserKey(w http.ResponseWriter, r *http.Request, userID, keyID string) {
	if err := models.DeleteAPIKey(r.Context(), h.db, userID, keyID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListUserCredentials(w http.ResponseWriter, r *http.Request, userID string) {
	creds, err := models.ListCredentials(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}
	writeJSON(w, http.StatusOK, creds)
}

func (h *AdminHandler) DeleteUserCredential(w http.ResponseWriter, r *http.Request, userID, provider string) {
	if err := models.DeleteCredential(r.Context(), h.db, userID, provider); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete credential")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) UsageSummary(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 {
		days = 7
	}

	summary, err := models.GetUsageSummary(r.Context(), h.db, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get usage summary")
		return
	}

	daily, _ := models.GetDailyUsage(r.Context(), h.db, days)
	byProvider, _ := models.GetProviderUsage(r.Context(), h.db, days)
	byUser, _ := models.GetUserUsage(r.Context(), h.db, days)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"summary":     summary,
		"daily":       daily,
		"by_provider": byProvider,
		"by_user":     byUser,
	})
}

func (h *AdminHandler) UserUsage(w http.ResponseWriter, r *http.Request, userID string) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 {
		days = 30
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 {
		pageSize = 20
	}

	items, total, err := models.GetUserUsageDetail(r.Context(), h.db, userID, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user usage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

// Quota management

func (h *AdminHandler) GetQuota(w http.ResponseWriter, r *http.Request, userID string) {
	q, err := quota.GetQuota(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get quota")
		return
	}
	c, err := quota.GetCounter(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get counter")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"quota":   q,
		"counter": c,
	})
}

func (h *AdminHandler) SetQuota(w http.ResponseWriter, r *http.Request, userID string) {
	var q quota.Quota
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := quota.SetQuota(r.Context(), h.db, userID, &q); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set quota")
		return
	}
	cache.InvalidateQuota(userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Metrics

func (h *AdminHandler) LatencyStats(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 {
		days = 7
	}
	provider := r.URL.Query().Get("provider")

	stats, err := models.GetLatencyStats(r.Context(), h.db, days, provider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get latency stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *AdminHandler) TokenUsage(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 {
		days = 7
	}

	usage, err := models.GetTokenUsageByModel(r.Context(), h.db, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get token usage")
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (h *AdminHandler) ErrorLogs(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 {
		days = 7
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := models.GetErrorLogs(r.Context(), h.db, days, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get error logs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Pricing management

func (h *AdminHandler) ListPricing(w http.ResponseWriter, r *http.Request) {
	pricing_list, err := pricing.ListPricing(r.Context(), h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pricing")
		return
	}
	writeJSON(w, http.StatusOK, pricing_list)
}

func (h *AdminHandler) UpdatePricing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider          string `json:"provider"`
		ModelPattern      string `json:"model_pattern"`
		InputPer1MCents   int    `json:"input_per_1m_cents"`
		OutputPer1MCents  int    `json:"output_per_1m_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Provider == "" || req.ModelPattern == "" {
		writeError(w, http.StatusBadRequest, "provider and model_pattern required")
		return
	}
	if err := pricing.UpdatePricing(r.Context(), h.db, req.Provider, req.ModelPattern, req.InputPer1MCents, req.OutputPer1MCents); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update pricing")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
