-- 待分类行的部分索引改为覆盖两对版本(输入版本与依赖版本任一不等), 替换 254 的索引。
-- CONCURRENTLY 避免建索引期间阻塞索引器写入。注意: 本文件按分号拆分语句, 注释里不能出现分号。
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_token_analysis_cls_due
    ON token_analysis_request_summaries (id)
    WHERE cls_input_version <> cls_checked_version OR cls_dep_version <> cls_dep_checked_version;
DROP INDEX CONCURRENTLY IF EXISTS idx_token_analysis_cls_pending;
