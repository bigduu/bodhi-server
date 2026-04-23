package proxy

import (
	"context"
	"database/sql"
	"log"
)

type ProviderTarget struct {
	InstanceID     string
	ProviderType   string
	BaseURL        string
	Priority       int
	TimeoutSeconds int
}

type ProviderRouter struct {
	db *sql.DB
}

func NewProviderRouter(db *sql.DB) *ProviderRouter {
	return &ProviderRouter{db: db}
}

// ResolveModel returns ordered list of provider instances for a model name.
// It checks the models table, then provider_instances, skipping recently failed instances.
func (r *ProviderRouter) ResolveModel(ctx context.Context, modelName string) ([]ProviderTarget, error) {
	// First check if model exists in registry
	var modelID string
	err := r.db.QueryRowContext(ctx,
		`SELECT id::text FROM models WHERE name = $1 AND is_active = true`, modelName,
	).Scan(&modelID)
	if err == sql.ErrNoRows {
		return nil, nil // No model in registry, return empty (caller falls back to direct routing)
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT pi.id::text, pi.provider_type, COALESCE(pi.base_url, ''), pi.priority, COALESCE(pi.timeout_seconds, 0)
		 FROM provider_instances pi
		 WHERE pi.model_id = $1::uuid AND pi.is_active = true
		 ORDER BY pi.priority ASC`,
		modelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []ProviderTarget
	for rows.Next() {
		var t ProviderTarget
		rows.Scan(&t.InstanceID, &t.ProviderType, &t.BaseURL, &t.Priority, &t.TimeoutSeconds)

		// Check recent failure count for this instance
		var failCount int
		r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM provider_failures
			 WHERE instance_id = $1::uuid AND created_at > NOW() - INTERVAL '5 minutes'`,
			t.InstanceID,
		).Scan(&failCount)

		if failCount >= 3 {
			log.Printf("skipping instance %s (%s): %d recent failures", t.InstanceID, t.ProviderType, failCount)
			continue
		}

		targets = append(targets, t)
	}
	return targets, nil
}

// RecordFailure logs a provider instance failure for circuit-breaking.
func (r *ProviderRouter) RecordFailure(ctx context.Context, instanceID, errMsg string) {
	if instanceID == "" {
		return
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO provider_failures (instance_id, error_msg) VALUES ($1::uuid, $2)`,
		instanceID, errMsg,
	)
	if err != nil {
		log.Printf("failed to record provider failure: %v", err)
	}
}

// CleanupFailures removes old failure records (call periodically).
func (r *ProviderRouter) CleanupFailures(ctx context.Context) {
	r.db.ExecContext(ctx,
		`DELETE FROM provider_failures WHERE created_at < NOW() - INTERVAL '1 hour'`,
	)
}

// ProviderInstance CRUD for admin API

type ProviderInstance struct {
	ID           string `json:"id"`
	ModelID      string `json:"model_id"`
	ProviderType string `json:"provider_type"`
	InstanceName string `json:"instance_name"`
	Priority     int    `json:"priority"`
	BaseURL      string `json:"base_url"`
	IsActive     bool   `json:"is_active"`
	HealthStatus string `json:"health_status"`
	LastCheckAt  string `json:"last_check_at"`
	CreatedAt    string `json:"created_at"`
}

func ListProviderInstances(ctx context.Context, db *sql.DB) ([]*ProviderInstance, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT pi.id::text, pi.model_id::text, pi.provider_type, pi.instance_name,
		        pi.priority, COALESCE(pi.base_url, ''), pi.is_active,
		        pi.health_status, COALESCE(pi.last_check_at::text, ''), pi.created_at::text
		 FROM provider_instances pi ORDER BY pi.priority, pi.created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*ProviderInstance
	for rows.Next() {
		pi := &ProviderInstance{}
		rows.Scan(&pi.ID, &pi.ModelID, &pi.ProviderType, &pi.InstanceName,
			&pi.Priority, &pi.BaseURL, &pi.IsActive,
			&pi.HealthStatus, &pi.LastCheckAt, &pi.CreatedAt)
		result = append(result, pi)
	}
	return result, nil
}

func CreateProviderInstance(ctx context.Context, db *sql.DB, pi *ProviderInstance) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO provider_instances (model_id, provider_type, instance_name, priority, base_url)
		 VALUES ($1::uuid, $2, $3, $4, $5)`,
		pi.ModelID, pi.ProviderType, pi.InstanceName, pi.Priority, pi.BaseURL,
	)
	return err
}

func UpdateProviderInstance(ctx context.Context, db *sql.DB, id string, pi *ProviderInstance) error {
	_, err := db.ExecContext(ctx,
		`UPDATE provider_instances SET provider_type=$1, instance_name=$2, priority=$3, base_url=$4, is_active=$5
		 WHERE id = $6::uuid`,
		pi.ProviderType, pi.InstanceName, pi.Priority, pi.BaseURL, pi.IsActive, id,
	)
	return err
}

func DeleteProviderInstance(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM provider_instances WHERE id = $1::uuid`, id)
	return err
}
