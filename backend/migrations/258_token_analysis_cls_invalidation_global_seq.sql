-- Token Analysis 失效登记的 seq 改为全局序列取值(风险分类设计 10.2)。
-- 行内 seq 从 1 起、冲突时加一, 登记被删除后重建又从 1 开始: 持有旧快照 (archive_id, 1)
-- 的另一消费者会误删新一代登记(ABA)。改为每次登记(新建或重复)都从全局序列取新值,
-- 删除后重建也不会与任何旧快照相等。源写入与账单删除触发器遵守同一合同。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md

CREATE SEQUENCE IF NOT EXISTS token_analysis_cls_invalidation_seq;

SELECT setval('token_analysis_cls_invalidation_seq',
    GREATEST((SELECT COALESCE(MAX(seq), 0) FROM token_analysis_cls_invalidations), 1));

ALTER TABLE token_analysis_cls_invalidations
    ALTER COLUMN seq SET DEFAULT nextval('token_analysis_cls_invalidation_seq');

CREATE OR REPLACE FUNCTION public.token_analysis_usage_detached()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.cls_input_version := NEW.cls_input_version + 1;
    INSERT INTO token_analysis_cls_invalidations (archive_id)
    VALUES (NEW.archive_id)
    ON CONFLICT (archive_id) DO UPDATE SET seq = nextval('token_analysis_cls_invalidation_seq');
    RETURN NEW;
END;
$$;
