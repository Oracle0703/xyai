CREATE TABLE subscription_self_daily_reset_usage (
    subscription_id BIGINT PRIMARY KEY REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    quota_date DATE NOT NULL,
    used_count INTEGER NOT NULL CHECK (used_count >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO settings (key, value, updated_at)
VALUES ('subscription_self_daily_reset_policy', '{"rollout":"admin","daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}', NOW())
ON CONFLICT (key) DO NOTHING;
