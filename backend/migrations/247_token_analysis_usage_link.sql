-- Token 分析计费关联: 归档请求与 usage_logs 的稳定关联键、响应完成事实与关联方式。
-- 索引后由 DB 内关联轮次补关联(账单异步落库/长耗时请求), 不需要重读 JSONL。
-- 只加可空列或常量默认列(PostgreSQL 11+ 仅改元数据), 不回填、不重写存量行;
-- 存量行 usage_match_method='' 表示旧时间窗口关联或尚未关联, 由关联轮次复核。
-- 设计与存量恢复见 docs/features/token-analysis-usage-link-design-cn.md。
ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS client_request_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS response_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS response_status INTEGER NULL,
    ADD COLUMN IF NOT EXISTS response_output_tokens BIGINT NULL,
    ADD COLUMN IF NOT EXISTS usage_match_method TEXT NOT NULL DEFAULT '',
    -- 复核版本: 完成事实变化或账单被夺回时 usage_link_version 加一, 处理结果按读到
    -- 的版本写回 usage_link_checked_version; 两者不等即待复核(不受日期范围限制)。
    -- usage_link_retry_until 为带关联键的请求等待晚到账单的截止时间。
    ADD COLUMN IF NOT EXISTS usage_link_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS usage_link_checked_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS usage_link_retry_until TIMESTAMPTZ NULL;

-- response 行先于其请求行被索引(跨日文件逆序回填)时暂存完成事实,
-- 请求行入库后由关联轮次合并; recorded_at 用于清理请求行永远缺失的暂存。
CREATE TABLE IF NOT EXISTS token_analysis_pending_responses (
    archive_id TEXT PRIMARY KEY,
    completed_at TIMESTAMPTZ NOT NULL,
    response_status INTEGER NULL,
    response_output_tokens BIGINT NULL,
    response_id TEXT NOT NULL DEFAULT '',
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
