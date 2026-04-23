package pricing

import (
	"context"
	"database/sql"
	"strings"

	"github.com/bigduu/bodhi-server/internal/cache"
)

var pricingCache *cache.TTLCache

func InitCache(c *cache.TTLCache) {
	pricingCache = c
}

func InvalidatePricingCache() {
	if pricingCache != nil {
		pricingCache.PurgeAll()
	}
}

type pricingEntry struct {
	inputPer1M  int
	outputPer1M int
}

func CalculateCost(ctx context.Context, db *sql.DB, provider, model string, inputTokens, outputTokens int) int {
	if inputTokens == 0 && outputTokens == 0 {
		return 0
	}

	// Try cache first
	key := "pricing:" + provider + ":" + model
	if pricingCache != nil {
		if v, ok := pricingCache.Get(key); ok {
			p := v.(*pricingEntry)
			return calcCost(p.inputPer1M, p.outputPer1M, inputTokens, outputTokens)
		}
	}

	var inputPer1M, outputPer1M int
	err := db.QueryRowContext(ctx,
		`SELECT input_per_1m_cents, output_per_1m_cents
		 FROM model_pricing
		 WHERE provider = $1 AND ($2 LIKE REPLACE(model_pattern, '*', '%'))
		 ORDER BY LENGTH(model_pattern) DESC LIMIT 1`,
		provider, model,
	).Scan(&inputPer1M, &outputPer1M)

	if err != nil {
		return 0
	}

	// Cache the result
	if pricingCache != nil {
		pricingCache.Set(key, &pricingEntry{inputPer1M: inputPer1M, outputPer1M: outputPer1M})
	}

	return calcCost(inputPer1M, outputPer1M, inputTokens, outputTokens)
}

func calcCost(inputPer1M, outputPer1M, inputTokens, outputTokens int) int {
	inputCost := float64(inputTokens) * float64(inputPer1M) / 1_000_000
	outputCost := float64(outputTokens) * float64(outputPer1M) / 1_000_000
	return int(inputCost + outputCost)
}

func ListPricing(ctx context.Context, db *sql.DB) ([]map[string]interface{}, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT provider, model_pattern, input_per_1m_cents, output_per_1m_cents, updated_at
		 FROM model_pricing ORDER BY provider, model_pattern`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var provider, pattern string
		var input, output int
		var updatedAt string
		rows.Scan(&provider, &pattern, &input, &output, &updatedAt)
		result = append(result, map[string]interface{}{
			"provider":            provider,
			"model_pattern":       pattern,
			"input_per_1m_cents":  input,
			"output_per_1m_cents": output,
			"updated_at":          updatedAt,
		})
	}
	return result, nil
}

func UpdatePricing(ctx context.Context, db *sql.DB, provider, pattern string, inputPer1M, outputPer1M int) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO model_pricing (provider, model_pattern, input_per_1m_cents, output_per_1m_cents)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (provider, model_pattern) DO UPDATE SET
		    input_per_1m_cents = EXCLUDED.input_per_1m_cents,
		    output_per_1m_cents = EXCLUDED.output_per_1m_cents,
		    updated_at = NOW()`,
		provider, pattern, inputPer1M, outputPer1M,
	)
	if err == nil {
		InvalidatePricingCache()
	}
	return err
}

// ModelMatches checks if a model string matches a pattern (supports * wildcard)
func ModelMatches(pattern, model string) bool {
	p := strings.ToLower(strings.TrimSpace(pattern))
	m := strings.ToLower(strings.TrimSpace(model))
	if p == m {
		return true
	}
	if strings.HasSuffix(p, "*") {
		return strings.HasPrefix(m, strings.TrimSuffix(p, "*"))
	}
	return false
}
