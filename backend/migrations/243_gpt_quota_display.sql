-- GPT 账号额度共享展示：单例配置、管理员勾选的展示条目、独立额度快照。
-- 快照与 accounts.extra 完全隔离，展示采集不写任何调度或自动用卡读取的键。
-- 设计见 docs/features/gpt-account-quota-display-design-cn.md。
CREATE TABLE IF NOT EXISTS gpt_quota_display_config (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id = TRUE),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    interval_minutes INTEGER NOT NULL DEFAULT 30 CHECK (interval_minutes IN (30, 60)),
    start_time TIME NOT NULL DEFAULT '09:30:00',
    end_time TIME NOT NULL DEFAULT '18:00:00',
    version BIGINT NOT NULL DEFAULT 1,
    -- 计划槽位条件更新去重：多实例与重启都只执行一次。
    last_slot_at TIMESTAMPTZ,
    updated_by BIGINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO gpt_quota_display_config (id)
VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;

-- 账号软删除不会触发级联，读取时按 accounts.deleted_at 排除。
CREATE TABLE IF NOT EXISTS gpt_quota_display_entries (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    selected BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_gpt_quota_display_entries_selected
    ON gpt_quota_display_entries (id)
    WHERE selected;

-- 每账号一行。缺失窗口为 SQL NULL；失败只更新尝试状态，不覆盖成功额度与 sampled_at。
CREATE TABLE IF NOT EXISTS gpt_quota_display_snapshots (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    five_hour JSONB,
    seven_day JSONB,
    sampled_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    last_attempt_status VARCHAR(40) NOT NULL DEFAULT 'never',
    retry_after TIMESTAMPTZ,
    source VARCHAR(20) NOT NULL DEFAULT 'active',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
