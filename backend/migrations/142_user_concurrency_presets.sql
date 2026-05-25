CREATE TABLE IF NOT EXISTS user_concurrency_presets (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    target_concurrency INTEGER NOT NULL CHECK (target_concurrency >= 1),
    user_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    schedule_enabled BOOLEAN NOT NULL DEFAULT false,
    schedule_time TEXT NOT NULL DEFAULT '',
    last_scheduled_run_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_concurrency_presets_schedule
    ON user_concurrency_presets(schedule_enabled, schedule_time)
    WHERE schedule_enabled = true;

CREATE TABLE IF NOT EXISTS user_concurrency_preset_runs (
    id BIGSERIAL PRIMARY KEY,
    preset_id BIGINT NOT NULL REFERENCES user_concurrency_presets(id) ON DELETE CASCADE,
    trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled')),
    target_concurrency INTEGER NOT NULL CHECK (target_concurrency >= 1),
    user_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    affected_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed')),
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_concurrency_preset_runs_preset_created
    ON user_concurrency_preset_runs(preset_id, created_at DESC);
