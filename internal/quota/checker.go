package quota

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bigduu/bodhi-server/internal/cache"
)

type Quota struct {
	RPM          int
	RPD          int
	TokenDaily   int64
	TokenMonthly int64
	SpendDaily   int
	SpendMonthly int
	AllowedModels []string
}

type Counter struct {
	MinuteRequests int
	DayRequests    int
	DayTokens      int64
	MonthTokens    int64
	DaySpend       int
	MonthSpend     int
}

var quotaCache *cache.TTLCache

func InitCache(c *cache.TTLCache) {
	quotaCache = c
}

func InvalidateQuotaCache(userID string) {
	if quotaCache != nil {
		quotaCache.Delete("quota:" + userID)
	}
}

func GetQuota(ctx context.Context, db *sql.DB, userID string) (*Quota, error) {
	if quotaCache != nil {
		if v, ok := quotaCache.Get("quota:" + userID); ok {
			return v.(*Quota), nil
		}
	}

	q := &Quota{RPM: 60}
	err := db.QueryRowContext(ctx,
		`SELECT rpm_limit, rpd_limit, token_daily, token_monthly, spend_daily, spend_monthly, allowed_models
		 FROM user_quotas WHERE user_id = $1::uuid`,
		userID,
	).Scan(&q.RPM, &q.RPD, &q.TokenDaily, &q.TokenMonthly, &q.SpendDaily, &q.SpendMonthly, &q.AllowedModels)

	if err == sql.ErrNoRows {
		return q, nil // Use defaults
	}
	if err != nil {
		return nil, err
	}
	if quotaCache != nil {
		quotaCache.Set("quota:"+userID, q)
	}
	return q, nil
}

func SetQuota(ctx context.Context, db *sql.DB, userID string, q *Quota) error {
	modelsJSON, _ := json.Marshal(q.AllowedModels)
	_, err := db.ExecContext(ctx,
		`INSERT INTO user_quotas (user_id, rpm_limit, rpd_limit, token_daily, token_monthly, spend_daily, spend_monthly, allowed_models)
		 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (user_id) DO UPDATE SET
		    rpm_limit = EXCLUDED.rpm_limit,
		    rpd_limit = EXCLUDED.rpd_limit,
		    token_daily = EXCLUDED.token_daily,
		    token_monthly = EXCLUDED.token_monthly,
		    spend_daily = EXCLUDED.spend_daily,
		    spend_monthly = EXCLUDED.spend_monthly,
		    allowed_models = EXCLUDED.allowed_models,
		    updated_at = NOW()`,
		userID, q.RPM, q.RPD, q.TokenDaily, q.TokenMonthly, q.SpendDaily, q.SpendMonthly, string(modelsJSON),
	)
	return err
}

func CheckLimit(ctx context.Context, db *sql.DB, userID string, model string) error {
	quota, err := GetQuota(ctx, db, userID)
	if err != nil {
		return nil // On error, allow through
	}

	// Check allowed models
	if len(quota.AllowedModels) > 0 {
		allowed := false
		for _, m := range quota.AllowedModels {
			if modelMatches(m, model) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("model %s not allowed", model)
		}
	}

	counter, err := getCounter(ctx, db, userID)
	if err != nil {
		return nil
	}

	// Check RPM
	if quota.RPM > 0 && counter.MinuteRequests >= quota.RPM {
		return fmt.Errorf("RPM limit exceeded (%d/%d)", counter.MinuteRequests, quota.RPM)
	}
	// Check RPD
	if quota.RPD > 0 && counter.DayRequests >= quota.RPD {
		return fmt.Errorf("daily request limit exceeded (%d/%d)", counter.DayRequests, quota.RPD)
	}
	// Check token daily
	if quota.TokenDaily > 0 && counter.DayTokens >= quota.TokenDaily {
		return fmt.Errorf("daily token limit exceeded")
	}
	// Check token monthly
	if quota.TokenMonthly > 0 && counter.MonthTokens >= quota.TokenMonthly {
		return fmt.Errorf("monthly token limit exceeded")
	}
	// Check spend daily
	if quota.SpendDaily > 0 && counter.DaySpend >= quota.SpendDaily {
		return fmt.Errorf("daily spend limit exceeded")
	}
	// Check spend monthly
	if quota.SpendMonthly > 0 && counter.MonthSpend >= quota.SpendMonthly {
		return fmt.Errorf("monthly spend limit exceeded")
	}

	return nil
}

func IncrementRequest(ctx context.Context, db *sql.DB, userID string) {
	now := time.Now().UTC()
	minuteStart := now.Truncate(time.Minute)
	dayStart := now.Truncate(24 * time.Hour)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	db.ExecContext(ctx,
		`INSERT INTO user_usage_counters (user_id, minute_requests, day_requests, minute_start, day_start, month_start)
		 VALUES ($1::uuid, 1, 1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE SET
		    minute_requests = CASE WHEN user_usage_counters.minute_start = $2 THEN user_usage_counters.minute_requests + 1 ELSE 1 END,
		    day_requests = CASE WHEN user_usage_counters.day_start = $3 THEN user_usage_counters.day_requests + 1 ELSE 1 END,
		    month_start = CASE WHEN user_usage_counters.month_start < $4 THEN $4 ELSE user_usage_counters.month_start END,
		    minute_start = $2,
		    day_start = $3`,
		userID, minuteStart, dayStart, monthStart,
	)
}

func AddTokenUsage(ctx context.Context, db *sql.DB, userID string, tokens int64, costCents int) {
	now := time.Now().UTC()
	dayStart := now.Truncate(24 * time.Hour)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	db.ExecContext(ctx,
		`INSERT INTO user_usage_counters (user_id, day_tokens, month_tokens, day_spend_cents, month_spend_cents, day_start, month_start)
		 VALUES ($1::uuid, $3, $3, $5, $5, $2, $4)
		 ON CONFLICT (user_id) DO UPDATE SET
		    day_tokens = CASE WHEN user_usage_counters.day_start = $2 THEN user_usage_counters.day_tokens + $3 ELSE $3 END,
		    month_tokens = CASE WHEN user_usage_counters.month_start >= $4 THEN user_usage_counters.month_tokens + $3 ELSE $3 END,
		    day_spend_cents = CASE WHEN user_usage_counters.day_start = $2 THEN user_usage_counters.day_spend_cents + $5 ELSE $5 END,
		    month_spend_cents = CASE WHEN user_usage_counters.month_start >= $4 THEN user_usage_counters.month_spend_cents + $5 ELSE $5 END,
		    day_start = $2,
		    month_start = $4`,
		userID, dayStart, tokens, monthStart, costCents,
	)
}

func GetCounter(ctx context.Context, db *sql.DB, userID string) (*Counter, error) {
	return getCounter(ctx, db, userID)
}

func getCounter(ctx context.Context, db *sql.DB, userID string) (*Counter, error) {
	c := &Counter{}
	err := db.QueryRowContext(ctx,
		`SELECT minute_requests, day_requests, day_tokens, month_tokens, day_spend_cents, month_spend_cents,
		        minute_start, day_start, month_start
		 FROM user_usage_counters WHERE user_id = $1::uuid`,
		userID,
	).Scan(&c.MinuteRequests, &c.DayRequests, &c.DayTokens, &c.MonthTokens, &c.DaySpend, &c.MonthSpend)

	if err == sql.ErrNoRows {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, nil
}

func modelMatches(pattern, model string) bool {
	if pattern == model {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix))
	}
	return false
}
