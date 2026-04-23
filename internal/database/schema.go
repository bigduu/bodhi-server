package database

import (
	"database/sql"
)

func Migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}

const schema = `
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(64) NOT NULL UNIQUE,
    email           VARCHAR(255),
    password_hash   VARCHAR(255) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at   TIMESTAMPTZ,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

CREATE TABLE IF NOT EXISTS api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            VARCHAR(128) NOT NULL,
    key_prefix      VARCHAR(12) NOT NULL,
    key_hash        VARCHAR(255) NOT NULL,
    key_suffix      VARCHAR(8) NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at      TIMESTAMPTZ,
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_from    UUID REFERENCES api_keys(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_prefix ON api_keys(key_prefix);

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS allowed_models TEXT[] DEFAULT '{}';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS allowed_providers TEXT[] DEFAULT '{}';
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS ip_whitelist TEXT[] DEFAULT '{}';

CREATE TABLE IF NOT EXISTS provider_credentials (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            VARCHAR(32) NOT NULL,
    encrypted_api_key   TEXT NOT NULL,
    base_url            TEXT,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_provider UNIQUE (user_id, provider)
);
CREATE INDEX IF NOT EXISTS idx_provider_creds_user_id ON provider_credentials(user_id);

CREATE TABLE IF NOT EXISTS usage_tracking (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id      UUID REFERENCES api_keys(id) ON DELETE SET NULL,
    provider        VARCHAR(32) NOT NULL,
    model           VARCHAR(128),
    endpoint        VARCHAR(128),
    input_tokens    INTEGER DEFAULT 0,
    output_tokens   INTEGER DEFAULT 0,
    duration_ms     INTEGER DEFAULT 0,
    status_code     INTEGER,
    cost_cents      INTEGER DEFAULT 0,
    is_error        BOOLEAN DEFAULT FALSE,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_usage_user_id ON usage_tracking(user_id);
CREATE INDEX IF NOT EXISTS idx_usage_created_at ON usage_tracking(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_provider ON usage_tracking(provider);
CREATE INDEX IF NOT EXISTS idx_usage_is_error ON usage_tracking(is_error);

CREATE TABLE IF NOT EXISTS user_quotas (
    user_id        UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    rpm_limit      INTEGER DEFAULT 60,
    rpd_limit      INTEGER DEFAULT 0,
    token_daily    BIGINT DEFAULT 0,
    token_monthly  BIGINT DEFAULT 0,
    spend_daily    INTEGER DEFAULT 0,
    spend_monthly  INTEGER DEFAULT 0,
    allowed_models TEXT[] DEFAULT '{}',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_usage_counters (
    user_id           UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    minute_requests   INTEGER DEFAULT 0,
    day_requests      INTEGER DEFAULT 0,
    day_tokens        BIGINT DEFAULT 0,
    month_tokens      BIGINT DEFAULT 0,
    day_spend_cents   INTEGER DEFAULT 0,
    month_spend_cents INTEGER DEFAULT 0,
    minute_start      TIMESTAMPTZ,
    day_start         TIMESTAMPTZ,
    month_start       TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS model_pricing (
    provider            VARCHAR(32) NOT NULL,
    model_pattern       VARCHAR(128) NOT NULL,
    input_per_1m_cents  INTEGER NOT NULL,
    output_per_1m_cents INTEGER NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, model_pattern)
);

CREATE TABLE IF NOT EXISTS model_mappings (
    user_id  UUID REFERENCES users(id) ON DELETE CASCADE,
    alias    VARCHAR(128) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    model    VARCHAR(128) NOT NULL,
    PRIMARY KEY (user_id, alias)
);

INSERT INTO model_pricing (provider, model_pattern, input_per_1m_cents, output_per_1m_cents) VALUES
    ('openai', 'gpt-4o', 250, 1000),
    ('openai', 'gpt-4o-mini', 15, 60),
    ('openai', 'gpt-4.1', 200, 800),
    ('openai', 'gpt-4.1-mini', 40, 160),
    ('openai', 'gpt-4.1-nano', 10, 40),
    ('openai', 'o1', 1500, 6000),
    ('openai', 'o3', 200, 800),
    ('openai', 'o4-mini', 110, 440),
    ('anthropic', 'claude-opus-4%', 1500, 7500),
    ('anthropic', 'claude-sonnet-4%', 300, 1500),
    ('anthropic', 'claude-haiku-4%', 100, 500),
    ('gemini', 'gemini-2.5-pro', 125, 500),
    ('gemini', 'gemini-2.5-flash', 15, 60)
ON CONFLICT (provider, model_pattern) DO NOTHING;

	CREATE TABLE IF NOT EXISTS system_settings (
	    key       VARCHAR(64) PRIMARY KEY,
	    value     JSONB NOT NULL DEFAULT '{}',
	    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS invite_codes (
	    code       VARCHAR(32) PRIMARY KEY DEFAULT replace(gen_random_uuid()::text, '-', ''),
	    created_by UUID REFERENCES users(id),
	    max_uses   INTEGER DEFAULT 1,
	    used_count INTEGER DEFAULT 0,
	    expires_at TIMESTAMPTZ,
	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS models (
	    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    name           VARCHAR(128) NOT NULL UNIQUE,
	    display_name   VARCHAR(128),
	    provider       VARCHAR(32) NOT NULL,
	    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
	    is_featured    BOOLEAN NOT NULL DEFAULT FALSE,
	    context_window INTEGER,
	    max_output     INTEGER,
	    capabilities   TEXT[] DEFAULT '{}',
	    sort_order     INTEGER DEFAULT 0,
	    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS provider_instances (
	    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    model_id      UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
	    provider_type VARCHAR(32) NOT NULL,
	    instance_name VARCHAR(64) NOT NULL,
	    priority      INTEGER DEFAULT 0,
	    base_url      TEXT,
	    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
	    health_status VARCHAR(16) DEFAULT 'unknown',
	    last_check_at TIMESTAMPTZ,
	    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	ALTER TABLE provider_instances ADD COLUMN IF NOT EXISTS timeout_seconds INTEGER DEFAULT 0;

	INSERT INTO system_settings (key, value) VALUES
	    ('registration_enabled', 'true'),
	    ('invite_required', 'false')
	ON CONFLICT (key) DO NOTHING;

	CREATE TABLE IF NOT EXISTS provider_failures (
	    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    instance_id UUID NOT NULL REFERENCES provider_instances(id) ON DELETE CASCADE,
	    error_msg   TEXT,
	    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_failures_instance ON provider_failures(instance_id);
	CREATE INDEX IF NOT EXISTS idx_failures_created ON provider_failures(created_at);

	CREATE TABLE IF NOT EXISTS billing_periods (
	    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	    period_start        TIMESTAMPTZ NOT NULL,
	    period_end          TIMESTAMPTZ NOT NULL,
	    total_requests      INTEGER DEFAULT 0,
	    total_input_tokens  BIGINT DEFAULT 0,
	    total_output_tokens BIGINT DEFAULT 0,
	    total_cost_cents    INTEGER DEFAULT 0,
	    generated_at        TIMESTAMPTZ DEFAULT NOW(),
	    UNIQUE (user_id, period_start)
	);

	ALTER TABLE user_quotas ADD COLUMN IF NOT EXISTS balance_cents INTEGER DEFAULT 0;
	ALTER TABLE user_quotas ADD COLUMN IF NOT EXISTS balance_alert_threshold INTEGER DEFAULT 0;
	ALTER TABLE user_quotas ADD COLUMN IF NOT EXISTS balance_last_alerted_at TIMESTAMPTZ;

	CREATE TABLE IF NOT EXISTS groups (
	    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    name        VARCHAR(128) NOT NULL,
	    description TEXT,
	    created_by  UUID REFERENCES users(id),
	    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS group_members (
	    group_id  UUID REFERENCES groups(id) ON DELETE CASCADE,
	    user_id   UUID REFERENCES users(id) ON DELETE CASCADE,
	    role      VARCHAR(16) NOT NULL DEFAULT 'member',
	    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	    PRIMARY KEY (group_id, user_id)
	);
	CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members(user_id);

	CREATE TABLE IF NOT EXISTS group_credentials (
	    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    group_id          UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
	    provider          VARCHAR(32) NOT NULL,
	    encrypted_api_key TEXT NOT NULL,
	    base_url          TEXT,
	    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
	    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	    CONSTRAINT uq_group_provider UNIQUE (group_id, provider)
	);

	CREATE TABLE IF NOT EXISTS group_quotas (
	    group_id       UUID PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
	    rpm_limit      INTEGER DEFAULT 0,
	    rpd_limit      INTEGER DEFAULT 0,
	    token_daily    BIGINT DEFAULT 0,
	    token_monthly  BIGINT DEFAULT 0,
	    spend_daily    INTEGER DEFAULT 0,
	    spend_monthly  INTEGER DEFAULT 0,
	    allowed_models TEXT[] DEFAULT '{}',
	    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS request_cache (
	    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    cache_key       VARCHAR(64) NOT NULL UNIQUE,
	    model           VARCHAR(128) NOT NULL,
	    response_body   BYTEA NOT NULL,
	    input_tokens    INTEGER,
	    output_tokens   INTEGER,
	    cost_cents      INTEGER,
	    hit_count       INTEGER DEFAULT 1,
	    expires_at      TIMESTAMPTZ NOT NULL,
	    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_cache_expires ON request_cache(expires_at);
	CREATE INDEX IF NOT EXISTS idx_cache_key ON request_cache(cache_key);

	ALTER TABLE usage_tracking ADD COLUMN IF NOT EXISTS is_cached BOOLEAN DEFAULT FALSE;

	CREATE TABLE IF NOT EXISTS audit_log (
	    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    user_id     UUID,
	    api_key_id  UUID,
	    action      VARCHAR(64) NOT NULL,
	    resource    VARCHAR(128),
	    detail      JSONB,
	    ip_address  VARCHAR(45),
	    user_agent  TEXT,
	    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_log(user_id);
	CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);
	CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at);

	CREATE TABLE IF NOT EXISTS content_rules (
	    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    name       VARCHAR(128) NOT NULL,
	    pattern    TEXT NOT NULL,
	    action     VARCHAR(16) NOT NULL DEFAULT 'block',
	    scope      VARCHAR(16) NOT NULL DEFAULT 'both',
	    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS distributed_rate_limits (
	    key_prefix   VARCHAR(64) NOT NULL,
	    window_start TIMESTAMPTZ NOT NULL,
	    count        INTEGER DEFAULT 0,
	    PRIMARY KEY (key_prefix, window_start)
	);

	CREATE TABLE IF NOT EXISTS webhooks (
	    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	    url       TEXT NOT NULL,
	    events    TEXT[] NOT NULL,
	    secret    VARCHAR(64),
	    is_active BOOLEAN NOT NULL DEFAULT TRUE,
	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS retention_policies (
	    table_name     VARCHAR(64) PRIMARY KEY,
	    retention_days INTEGER NOT NULL DEFAULT 90,
	    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
	    last_purged_at TIMESTAMPTZ,
	    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	INSERT INTO retention_policies (table_name, retention_days) VALUES
	    ('usage_tracking', 90),
	    ('audit_log', 180),
	    ('provider_failures', 7),
	    ('request_cache', 30),
	    ('distributed_rate_limits', 1),
	    ('billing_periods', 365)
	ON CONFLICT (table_name) DO NOTHING;

	CREATE TABLE IF NOT EXISTS token_revocations (
	    jti_hash   VARCHAR(64) PRIMARY KEY,
	    expires_at TIMESTAMPTZ NOT NULL,
	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_token_rev_expires ON token_revocations(expires_at);

		CREATE TABLE IF NOT EXISTS versions (
		    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		    version      VARCHAR(32) NOT NULL UNIQUE,
		    platform     VARCHAR(16) NOT NULL DEFAULT 'all',
		    changelog    TEXT NOT NULL DEFAULT '',
		    download_url TEXT,
		    is_latest    BOOLEAN NOT NULL DEFAULT FALSE,
		    force_update BOOLEAN NOT NULL DEFAULT FALSE,
		    is_draft     BOOLEAN NOT NULL DEFAULT TRUE,
		    published_at TIMESTAMPTZ,
		    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_versions_platform ON versions(platform);
		CREATE INDEX IF NOT EXISTS idx_versions_is_latest ON versions(is_latest) WHERE is_latest = TRUE;
	`