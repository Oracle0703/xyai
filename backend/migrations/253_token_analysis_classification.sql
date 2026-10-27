-- Token Analysis 冷暖观察、原因归类与条件性复用机会金额(设计第 6-8、10 节)。
-- cls_input_version 在任何分类输入变化时加一(同一事务内并沿前缀边传播给子请求),
-- 分类任务按版本条件写回并设 cls_checked_version。金额只在读路径满足发布条件时可见。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md

ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS cls_input_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cls_checked_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cls_rule_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cls_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cache_obs_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cause TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS prefix_evidence TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cache_eligibility TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS price_evidence TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS parent_archive_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS parent_cls_version_used BIGINT,
    ADD COLUMN IF NOT EXISTS reusable_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS recoverable_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS amount NUMERIC(20, 10),
    ADD COLUMN IF NOT EXISTS cold_input_cost NUMERIC(20, 10),
    ADD COLUMN IF NOT EXISTS amount_inputs_hash BYTEA,
    ADD COLUMN IF NOT EXISTS classified_at TIMESTAMPTZ;
