CREATE TABLE IF NOT EXISTS user_prompt_events (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(128),
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    model VARCHAR(255) NOT NULL DEFAULT '',
    requested_model VARCHAR(255) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL DEFAULT '',
    source_protocol VARCHAR(64) NOT NULL DEFAULT '',
    prompt_text TEXT,
    prompt_excerpt TEXT NOT NULL DEFAULT '',
    prompt_hash CHAR(64) NOT NULL,
    prompt_chars INT NOT NULL DEFAULT 0,
    prompt_segments INT NOT NULL DEFAULT 0,
    prompt_tokens_estimated INT NOT NULL DEFAULT 0,
    project_name VARCHAR(255),
    git_branch VARCHAR(255),
    client_name VARCHAR(128),
    client_version VARCHAR(64),
    user_agent TEXT,
    ip_address VARCHAR(45),
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    analysis_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_prompt_events_request_api_key_unique
    ON user_prompt_events(request_id, api_key_id)
    WHERE request_id IS NOT NULL AND api_key_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_user_prompt_events_created_at ON user_prompt_events(created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_user_created ON user_prompt_events(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_api_key_created ON user_prompt_events(api_key_id, created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_group_created ON user_prompt_events(group_id, created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_project_created ON user_prompt_events(project_name, created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_branch_created ON user_prompt_events(git_branch, created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_client_created ON user_prompt_events(client_name, created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_model_created ON user_prompt_events(requested_model, created_at);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_hash ON user_prompt_events(prompt_hash);
CREATE INDEX IF NOT EXISTS idx_user_prompt_events_analysis_status ON user_prompt_events(analysis_status);

CREATE TABLE IF NOT EXISTS user_prompt_analysis (
    id BIGSERIAL PRIMARY KEY,
    prompt_event_id BIGINT NOT NULL REFERENCES user_prompt_events(id) ON DELETE CASCADE,
    summary TEXT NOT NULL DEFAULT '',
    quality_score SMALLINT NOT NULL DEFAULT 0,
    clarity_score SMALLINT NOT NULL DEFAULT 0,
    context_score SMALLINT NOT NULL DEFAULT 0,
    actionability_score SMALLINT NOT NULL DEFAULT 0,
    constraint_score SMALLINT NOT NULL DEFAULT 0,
    risk_score SMALLINT NOT NULL DEFAULT 0,
    categories JSONB NOT NULL DEFAULT '[]'::jsonb,
    improvement_suggestions JSONB NOT NULL DEFAULT '[]'::jsonb,
    analyzer_model VARCHAR(128) NOT NULL DEFAULT 'local-rules-v1',
    analyzed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_prompt_analysis_event_unique UNIQUE(prompt_event_id),
    CONSTRAINT user_prompt_analysis_quality_range CHECK (quality_score BETWEEN 0 AND 100),
    CONSTRAINT user_prompt_analysis_clarity_range CHECK (clarity_score BETWEEN 0 AND 100),
    CONSTRAINT user_prompt_analysis_context_range CHECK (context_score BETWEEN 0 AND 100),
    CONSTRAINT user_prompt_analysis_actionability_range CHECK (actionability_score BETWEEN 0 AND 100),
    CONSTRAINT user_prompt_analysis_constraint_range CHECK (constraint_score BETWEEN 0 AND 100),
    CONSTRAINT user_prompt_analysis_risk_range CHECK (risk_score BETWEEN 0 AND 100)
);

CREATE INDEX IF NOT EXISTS idx_user_prompt_analysis_quality ON user_prompt_analysis(quality_score);
CREATE INDEX IF NOT EXISTS idx_user_prompt_analysis_risk ON user_prompt_analysis(risk_score);
