-- Append-only history of successful self resets; written in the same transaction as the usage row.
-- No foreign keys: history must survive subscription/user/group deletion.
CREATE TABLE IF NOT EXISTS subscription_self_reset_events (
    id BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    organization VARCHAR(32) NOT NULL,
    quota_date DATE NOT NULL,
    used_count INTEGER NOT NULL,
    daily_limit INTEGER NOT NULL,
    daily_usage_usd_before DECIMAL(20, 10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_self_reset_events_quota_date ON subscription_self_reset_events (quota_date);
CREATE INDEX IF NOT EXISTS idx_subscription_self_reset_events_user_id ON subscription_self_reset_events (user_id, id DESC);
