-- 前缀指纹候选查找与待重新匹配行的部分索引, 存量行指纹为空, 部分索引只收新行。
-- CONCURRENTLY 避免建索引期间阻塞索引器写入。注意: 本文件按分号拆分语句, 注释里不能出现分号。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_prefix_end_hash
    ON token_analysis_request_summaries (api_key_id, prefix_end_hash)
    WHERE prefix_end_hash IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_prefix_tail1_hash
    ON token_analysis_request_summaries (api_key_id, prefix_tail1_hash)
    WHERE prefix_tail1_hash IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_items_end_hash
    ON token_analysis_request_summaries (api_key_id, items_end_hash)
    WHERE items_end_hash IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_cand_pending
    ON token_analysis_request_summaries (id)
    WHERE cand_input_version <> cand_checked_version;
