-- Token Analysis 转发侧记录: 网关在定型发往上游的请求时写下的缓存相关事实,
-- 随归档 response 行进入摘要; 响应先于请求入库时经暂存表合并。
-- 只有原生 OpenAI Responses 路径写入, 其余路径与本列上线前的行为空串(未知)。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md 11.2

ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS forward_path TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cache_key_handling TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_cache_key_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS usage_cache_source TEXT NOT NULL DEFAULT '';

ALTER TABLE token_analysis_pending_responses
    ADD COLUMN IF NOT EXISTS forward_path TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cache_key_handling TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_cache_key_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS usage_cache_source TEXT NOT NULL DEFAULT '';
