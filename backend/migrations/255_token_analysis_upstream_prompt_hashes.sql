-- Token Analysis 转发侧记录补充最终请求体的提示词哈希(风险分类设计 11.2)。
-- upstream_items_hash 是最终 input 条目链末端, 与入口正文的 items_end_hash 同算法同密钥:
-- 相等表示网关没有改写条目; upstream_root_hash 是最终顶层字段的 h0。空串=未记录。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md

ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS upstream_items_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_root_hash TEXT NOT NULL DEFAULT '';

ALTER TABLE token_analysis_pending_responses
    ADD COLUMN IF NOT EXISTS upstream_items_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_root_hash TEXT NOT NULL DEFAULT '';
