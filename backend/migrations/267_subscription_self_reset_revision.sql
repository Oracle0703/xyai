-- Kept outside the policy JSON so older binaries can still decode it.
INSERT INTO settings (key, value, updated_at)
VALUES ('subscription_self_daily_reset_revision', '1', NOW())
ON CONFLICT (key) DO NOTHING;
