-- GPT 降智测试：单例配置、测试项（可变展示属性）与不可变题目版本、批次、逐次结果、
-- 每日派发预算与 429 退避。账号集合复用 gpt_quota_display_entries，不单独选账号。
-- 执行器不写 accounts.extra、不改账号调度状态。设计见 docs/features/gpt-iq-test-design-cn.md。
CREATE TABLE IF NOT EXISTS gpt_iq_test_config (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id = TRUE),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    schedule_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    run_time TIME NOT NULL DEFAULT '08:30:00',
    -- 每日定时去重（Asia/Shanghai 日期）：条件更新只成功一次。
    last_run_date DATE,
    min_accuracy_percent INTEGER NOT NULL DEFAULT 60 CHECK (min_accuracy_percent BETWEEN 0 AND 100),
    min_graded_runs INTEGER NOT NULL DEFAULT 2 CHECK (min_graded_runs BETWEEN 1 AND 5),
    reasoning_ratio_percent INTEGER NOT NULL DEFAULT 30 CHECK (reasoning_ratio_percent BETWEEN 0 AND 100),
    min_five_hour_percent INTEGER NOT NULL DEFAULT 10 CHECK (min_five_hour_percent BETWEEN 0 AND 100),
    min_seven_day_percent INTEGER NOT NULL DEFAULT 5 CHECK (min_seven_day_percent BETWEEN 0 AND 100),
    daily_call_budget INTEGER NOT NULL DEFAULT 300 CHECK (daily_call_budget BETWEEN 0 AND 5000),
    retention_days INTEGER NOT NULL DEFAULT 90 CHECK (retention_days BETWEEN 7 AND 365),
    version BIGINT NOT NULL DEFAULT 1,
    updated_by BIGINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO gpt_iq_test_config (id)
VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;

-- 测试项：名称、启停、排序、公开开关可原地修改（用户侧始终按当前开关过滤历史）；
-- 题面、模型、判分等影响结果解释的字段写入不可变版本表。
CREATE TABLE IF NOT EXISTS gpt_iq_test_cases (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    public_prompt BOOLEAN NOT NULL DEFAULT TRUE,
    public_answer BOOLEAN NOT NULL DEFAULT TRUE,
    current_revision_id BIGINT,
    version BIGINT NOT NULL DEFAULT 1,
    deleted_at TIMESTAMPTZ,
    created_by BIGINT,
    updated_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gpt_iq_test_case_revisions (
    id BIGSERIAL PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES gpt_iq_test_cases(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL,
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('chat', 'graded')),
    prompt TEXT NOT NULL,
    prompt_sha256 CHAR(64) NOT NULL,
    model VARCHAR(100) NOT NULL,
    reasoning_effort VARCHAR(10) NOT NULL DEFAULT '',
    runs INTEGER NOT NULL CHECK (runs BETWEEN 1 AND 5),
    timeout_seconds INTEGER NOT NULL CHECK (timeout_seconds BETWEEN 30 AND 600),
    scoring_mode VARCHAR(20) NOT NULL DEFAULT 'none'
        CHECK (scoring_mode IN ('none', 'final_line', 'number_any', 'contains', 'regex')),
    scoring_expected TEXT NOT NULL DEFAULT '',
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (case_id, revision)
);

ALTER TABLE gpt_iq_test_cases
    DROP CONSTRAINT IF EXISTS gpt_iq_test_cases_current_revision_fk;
ALTER TABLE gpt_iq_test_cases
    ADD CONSTRAINT gpt_iq_test_cases_current_revision_fk
    FOREIGN KEY (current_revision_id) REFERENCES gpt_iq_test_case_revisions(id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE IF NOT EXISTS gpt_iq_test_batches (
    id BIGSERIAL PRIMARY KEY,
    trigger VARCHAR(10) NOT NULL CHECK (trigger IN ('schedule', 'manual')),
    run_date DATE NOT NULL,
    triggered_by BIGINT,
    -- 本批次允许的展示分组（xunyou/wsdashi），组织负责人手动触发时为其授权组织。
    scope_groups TEXT[] NOT NULL DEFAULT '{}',
    entry_ids BIGINT[] NOT NULL DEFAULT '{}',
    case_ids BIGINT[] NOT NULL DEFAULT '{}',
    status VARCHAR(12) NOT NULL CHECK (status IN ('running', 'finished', 'cancelled', 'aborted', 'abandoned')),
    -- 执行权令牌：结果写入、心跳与终态更新都校验令牌，被回收的旧执行者无法回写。
    lease_token VARCHAR(64) NOT NULL,
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    total_calls INTEGER NOT NULL DEFAULT 0,
    -- 未正常完成的原因：timeout（超过批次上限）、stopping（服务停止）、persist_failed（结果写入失败）。
    failure_reason VARCHAR(40) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

-- 全站同一时刻只有一个运行中批次（定时与手动共用）。
CREATE UNIQUE INDEX IF NOT EXISTS uq_gpt_iq_test_batches_running
    ON gpt_iq_test_batches ((TRUE))
    WHERE status = 'running';

CREATE INDEX IF NOT EXISTS idx_gpt_iq_test_batches_run_date
    ON gpt_iq_test_batches (run_date DESC, id DESC);

-- 每行是一次逻辑调用（某账号 × 某测试项 × 第 run_index 次）；重试复用同一行并累加 attempts。
-- status 只表示执行结果，正确与否单独存于 correct（NULL 表示不评分）。
CREATE TABLE IF NOT EXISTS gpt_iq_test_results (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES gpt_iq_test_batches(id) ON DELETE CASCADE,
    run_date DATE NOT NULL,
    trigger VARCHAR(10) NOT NULL,
    entry_id BIGINT NOT NULL REFERENCES gpt_quota_display_entries(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL,
    case_id BIGINT NOT NULL REFERENCES gpt_iq_test_cases(id) ON DELETE CASCADE,
    revision_id BIGINT NOT NULL REFERENCES gpt_iq_test_case_revisions(id) ON DELETE CASCADE,
    scored_revision_id BIGINT REFERENCES gpt_iq_test_case_revisions(id) ON DELETE SET NULL,
    run_index INTEGER NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(12) NOT NULL CHECK (status IN ('running', 'ok', 'error', 'skipped', 'cancelled', 'abandoned')),
    reason VARCHAR(40) NOT NULL DEFAULT '',
    error_summary VARCHAR(500) NOT NULL DEFAULT '',
    http_status INTEGER,
    correct BOOLEAN,
    model_requested VARCHAR(100) NOT NULL DEFAULT '',
    model_responded VARCHAR(100) NOT NULL DEFAULT '',
    reasoning_effort VARCHAR(10) NOT NULL DEFAULT '',
    answer_text TEXT NOT NULL DEFAULT '',
    answer_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    input_tokens INTEGER,
    cached_tokens INTEGER,
    output_tokens INTEGER,
    reasoning_tokens INTEGER,
    ttft_ms INTEGER,
    duration_ms INTEGER,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    UNIQUE (batch_id, entry_id, case_id, run_index)
);

CREATE INDEX IF NOT EXISTS idx_gpt_iq_test_results_entry_case
    ON gpt_iq_test_results (entry_id, case_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_gpt_iq_test_results_run_date
    ON gpt_iq_test_results (run_date);

-- 每日实际派发名额（首发与重试各占一次，跳过不占）。
CREATE TABLE IF NOT EXISTS gpt_iq_test_daily_usage (
    run_date DATE PRIMARY KEY,
    dispatched INTEGER NOT NULL DEFAULT 0
);

-- 429 退避，跨实例生效；只属于本功能，不影响网关调度。
CREATE TABLE IF NOT EXISTS gpt_iq_test_backoff (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    retry_after TIMESTAMPTZ NOT NULL,
    reason VARCHAR(40) NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
