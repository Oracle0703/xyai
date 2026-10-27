-- Token Analysis 分类版本拆成两对(风险分类设计 10.1/10.2)。
-- cls_input_version 只在本行自身的分类输入变化时加一, 且总在同一事务内登记失效;
-- 子请求记下的 parent_cls_version_used 与它比较。
-- cls_dep_version 由依赖传播、规则版本扫描、候选变化标记加一, 只表示"需要重算",
-- 不改变本行对子请求的版本, 因此传播(包括过期的消费者)不会让子请求的父版本失配。
-- 两对任一不等即待分类。设计文档: docs/features/token-analysis-risk-classification-design-cn.md

ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS cls_dep_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cls_dep_checked_version BIGINT NOT NULL DEFAULT 0;
