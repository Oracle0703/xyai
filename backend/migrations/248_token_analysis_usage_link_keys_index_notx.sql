-- 计费关联的精确归属保护按关联键反查摘要行, 存量行键为空, 部分索引只收新行,
-- 待重新关联的行按标记跨日期查找。CONCURRENTLY 避免建索引期间阻塞索引器写入。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_client_request_id
    ON token_analysis_request_summaries (client_request_id)
    WHERE client_request_id <> '';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_response_id
    ON token_analysis_request_summaries (response_id)
    WHERE response_id <> '';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_usage_link_pending
    ON token_analysis_request_summaries (id)
    WHERE usage_link_version <> usage_link_checked_version;
