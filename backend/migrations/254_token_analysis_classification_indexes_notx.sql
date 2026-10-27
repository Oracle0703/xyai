-- 分类任务按 id 取待重算行的部分索引, 已追平的行不进入索引。
-- CONCURRENTLY 避免建索引期间阻塞索引器写入。注意: 本文件按分号拆分语句, 注释里不能出现分号。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_cls_pending
    ON token_analysis_request_summaries (id)
    WHERE cls_input_version <> cls_checked_version;
