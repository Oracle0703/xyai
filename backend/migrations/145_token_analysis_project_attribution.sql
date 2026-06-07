-- Token Analysis 项目归因: 请求摘要追加客户端工作目录/项目/分支字段,
-- 并新增已知仓库根表(Copilot 附件路径前缀匹配的数据源)。
-- 设计文档: docs/features/token-analysis-project-attribution-design-cn.md

ALTER TABLE token_analysis_request_summaries
    ADD COLUMN IF NOT EXISTS client_workdir TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_project TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_branch TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attribution_source TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_token_analysis_project_time
    ON token_analysis_request_summaries (client_project, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_token_analysis_user_project_time
    ON token_analysis_request_summaries (user_id, client_project, event_time DESC);

CREATE TABLE IF NOT EXISTS token_analysis_project_roots (
    root TEXT PRIMARY KEY,
    project TEXT NOT NULL DEFAULT '',
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_token_analysis_project_roots_project
    ON token_analysis_project_roots (project);
