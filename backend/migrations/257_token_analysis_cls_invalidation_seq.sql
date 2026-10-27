-- Token Analysis 失效登记改为"提交后传播"(风险分类设计 10.2)。
-- 所有改变分类输入的源写入在事务内只给自身加版本并登记; 分类作业在源事务提交后
-- 读取登记、沿前缀边给子孙加版本。seq 在重复登记时加一: 作业只删除读取时的那一版,
-- 处理期间又有新变化的登记保留到下一轮, 不会漏掉。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md

ALTER TABLE token_analysis_cls_invalidations
    ADD COLUMN IF NOT EXISTS seq BIGINT NOT NULL DEFAULT 1;

CREATE OR REPLACE FUNCTION public.token_analysis_usage_detached()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.cls_input_version := NEW.cls_input_version + 1;
    INSERT INTO token_analysis_cls_invalidations (archive_id)
    VALUES (NEW.archive_id)
    ON CONFLICT (archive_id) DO UPDATE SET seq = token_analysis_cls_invalidations.seq + 1;
    RETURN NEW;
END;
$$;
