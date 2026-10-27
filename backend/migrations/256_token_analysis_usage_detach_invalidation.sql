-- Token Analysis 账单删除的失效登记(风险分类设计 10.7)。
-- usage_logs 被保留期/管理员清理删除时, 外键 ON DELETE SET NULL 在库内把摘要行的
-- usage_log_id 置空, 应用代码看不到这次变化。触发器在同一语句内给该行加分类版本,
-- 并写入失效登记表; 分类作业处理登记时再沿前缀边通知子孙。登记独立于分类结果,
-- 不会被后续分类覆盖。应用代码自己置空 usage_log_id 时同样会登记, 重复通知无害。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md

CREATE TABLE IF NOT EXISTS token_analysis_cls_invalidations (
    archive_id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION public.token_analysis_usage_detached()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.cls_input_version := NEW.cls_input_version + 1;
    INSERT INTO token_analysis_cls_invalidations (archive_id)
    VALUES (NEW.archive_id)
    ON CONFLICT (archive_id) DO NOTHING;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS token_analysis_usage_detached ON token_analysis_request_summaries;
CREATE TRIGGER token_analysis_usage_detached
BEFORE UPDATE
ON token_analysis_request_summaries
FOR EACH ROW
WHEN (OLD.usage_log_id IS NOT NULL AND NEW.usage_log_id IS NULL)
EXECUTE FUNCTION public.token_analysis_usage_detached();
