-- Token Analysis 前缀指纹与候选边: 摘要行保存前缀链末端哈希(及去掉末尾 1 条的哈希)、
-- 对话关联链末端哈希与归档原文行位置; 候选父请求关系存边表, 候选快照用版本对
-- (cand_input_version / cand_checked_version)判定是否需要重新匹配。
-- 指纹为空或 fp_state 非 ok 的行不参与匹配。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md 第 5 节

ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS fp_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS fp_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS prefix_end_hash BYTEA,
    ADD COLUMN IF NOT EXISTS prefix_tail1_hash BYTEA,
    ADD COLUMN IF NOT EXISTS items_end_hash BYTEA,
    ADD COLUMN IF NOT EXISTS client_cache_key_hash BYTEA,
    ADD COLUMN IF NOT EXISTS prefix_items INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS has_continuation_ref BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS source_line_start BIGINT,
    ADD COLUMN IF NOT EXISTS source_line_length BIGINT,
    ADD COLUMN IF NOT EXISTS cand_input_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cand_checked_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cand_state TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cand_overflow BOOLEAN NOT NULL DEFAULT FALSE;

-- kind: full_prefix(父请求全部条目是子请求前缀) / prefix_minus_tail(去掉父请求末尾
-- 1 条后是前缀) / items_link(不含 system/tools 的条目链相同, 只作关联线索)。
CREATE TABLE IF NOT EXISTS token_analysis_prefix_edges (
    child_archive_id TEXT NOT NULL,
    parent_archive_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    matched_items INTEGER NOT NULL,
    PRIMARY KEY (child_archive_id, parent_archive_id, kind)
);

CREATE INDEX IF NOT EXISTS idx_token_analysis_prefix_edges_parent
    ON token_analysis_prefix_edges (parent_archive_id);
