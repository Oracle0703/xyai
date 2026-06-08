-- Token Analysis 用户净输入留存: 索引时把每次请求的最后一条用户输入全文
-- (脱敏+截断)单独入库, 质量字段为占位, 评价标准确定后由评估任务回填。
-- 设计文档: docs/features/token-analysis-user-input-store-design-cn.md

CREATE TABLE IF NOT EXISTS token_analysis_user_inputs (
    id BIGSERIAL PRIMARY KEY,
    archive_id TEXT NOT NULL UNIQUE,
    event_time TIMESTAMPTZ NOT NULL,
    user_id BIGINT,
    content TEXT NOT NULL,
    content_sha256 TEXT NOT NULL DEFAULT '',
    chars INTEGER NOT NULL DEFAULT 0,
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    quality_score SMALLINT,
    quality_findings JSONB,
    quality_version TEXT NOT NULL DEFAULT '',
    evaluated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_token_analysis_user_inputs_time
    ON token_analysis_user_inputs (event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_user_inputs_user_time
    ON token_analysis_user_inputs (user_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_user_inputs_sha
    ON token_analysis_user_inputs (content_sha256);
