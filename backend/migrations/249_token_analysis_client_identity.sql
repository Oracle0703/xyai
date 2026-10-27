-- Token Analysis 客户端识别: 请求摘要保存归档 User-Agent 与映射出的客户端族。
-- client_family_version 是映射规则版本, 0 表示未识别(本列上线前索引的历史行,
-- 待受控重索引补齐)。
-- 设计文档: docs/features/token-analysis-risk-classification-design-cn.md 4.2

ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS user_agent TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_family TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_family_version INTEGER NOT NULL DEFAULT 0;
