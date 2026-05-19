CREATE TABLE IF NOT EXISTS token_analysis_request_summaries (
    id BIGSERIAL PRIMARY KEY,
    archive_id TEXT NOT NULL UNIQUE,
    usage_log_id BIGINT NULL REFERENCES usage_logs(id) ON DELETE SET NULL,
    match_confidence SMALLINT NOT NULL DEFAULT 0,
    event_time TIMESTAMPTZ NOT NULL,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    api_key_id BIGINT NULL REFERENCES api_keys(id) ON DELETE SET NULL,
    account_id BIGINT NULL REFERENCES accounts(id) ON DELETE SET NULL,
    group_id BIGINT NULL REFERENCES groups(id) ON DELETE SET NULL,
    model TEXT NOT NULL DEFAULT '',
    endpoint TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT '',
    request_body_size BIGINT NOT NULL DEFAULT 0,
    request_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    body_sha256 TEXT NOT NULL DEFAULT '',
    message_count INTEGER NOT NULL DEFAULT 0,
    system_chars INTEGER NOT NULL DEFAULT 0,
    user_chars INTEGER NOT NULL DEFAULT 0,
    last_user_preview TEXT NOT NULL DEFAULT '',
    tools_count INTEGER NOT NULL DEFAULT 0,
    image_count INTEGER NOT NULL DEFAULT 0,
    summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    risk_score INTEGER NOT NULL DEFAULT 0,
    risk_reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    indexed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_file TEXT NOT NULL DEFAULT '',
    source_offset BIGINT NULL
);

CREATE INDEX IF NOT EXISTS idx_token_analysis_event_time
    ON token_analysis_request_summaries (event_time DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_user_time
    ON token_analysis_request_summaries (user_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_api_key_time
    ON token_analysis_request_summaries (api_key_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_group_time
    ON token_analysis_request_summaries (group_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_risk_time
    ON token_analysis_request_summaries (risk_score DESC, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_body_hash_time
    ON token_analysis_request_summaries (body_sha256, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_usage_log_id
    ON token_analysis_request_summaries (usage_log_id);

CREATE TABLE IF NOT EXISTS token_analysis_index_state (
    source_file TEXT PRIMARY KEY,
    last_offset BIGINT NOT NULL DEFAULT 0,
    last_archive_id TEXT NOT NULL DEFAULT '',
    processed_rows BIGINT NOT NULL DEFAULT 0,
    failed_rows BIGINT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_token_analysis_index_state_updated
    ON token_analysis_index_state (updated_at DESC);
