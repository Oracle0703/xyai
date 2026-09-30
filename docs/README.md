# docs 目录索引

本目录存放上游自带文档和本地设计、审核、交付、上游同步记录。AI 开发前仍先读 `llm-wiki/wiki/README.md`；本页只说明文档放在哪里、新文档该放哪里。

## 目录结构

| 位置 | 内容 | 维护方式 |
| --- | --- | --- |
| `PAYMENT.md`、`PAYMENT_CN.md`、`ADMIN_PAYMENT_INTEGRATION_API.md`、`ASYNC_IMAGE_TASKS.md`、`BATCH_IMAGE_MVP.md`、`COMPOSITE_GROUPS.md`、`PLUGIN_DEVELOPMENT.md`、`channel-monitor-v2-safe-defaults.md` | 上游 `Wei-Shaw/sub2api` 自带文档 | 保持原位，随上游合并；README 和设置页链接指向这些路径 |
| `legal/` | 管理员合规文档（上游） | 不可移动：前端 `?raw` 打包、`Dockerfile`、`.dockerignore` 和后端常量引用该路径 |
| `ARCHITECTURE_AND_OPS_HANDBOOK.md` | 本地架构与运维总手册（给人读） | 根 README 引用，保持原位 |
| `安装与操作手册.md` | 历史安装包 `main@b3ff5895`（2026-05-27）的 Ubuntu 20.04 一键安装说明，不是现行手册 | 历史快照；配套脚本只在该安装包内，不在仓库 |
| `features/` | 功能设计、技术说明、实施计划、实现交付记录 | 活文档，结论变化时直接更新正文 |
| `reviews/` | 功能设计/代码审核、审计、裁决 | 历史快照，不改写结论 |
| `delivery/<日期>-<主题>/` | 功能阶段交付：验收、性能、截图 | 历史快照 |
| `upstream-sync/` | 上游合并台账、流程和每轮审核材料 | 见下节 |
| `superpowers/plans/`、`superpowers/specs/` | AI 协作生成的实施计划与设计规格（2026-05 至 2026-08） | 历史快照 |

### upstream-sync/

| 位置 | 内容 |
| --- | --- |
| `merge-log.md` | 上游合并台账，**只追加**，每次合并后必须写入（见 `AGENTS.md`） |
| `playbook.md` | 合并流程、必须保留的本地能力和验证清单 |
| `<YYYY-MM-DD>-v<版本>/` | 每轮同步材料：`review.md`（审核与验证），早期轮次为 `requirements/specs/plan/test-review/delivery-*.md`，独立复审为 `claude-review-*.md` |
| `reviews/` | 早期按提交或版本命名的合并审核报告 |

## 新文档放置规则

- 设计、计划、实现说明放 `features/`，命名 `<主题>-<类型>-cn.md`；不要把 review、audit 放进 `features/`。
- 审核原文放 `reviews/`；已采纳结论回写到 `features/` 对应正文。
- 功能验收、性能数据、截图放 `delivery/<YYYY-MM-DD>-<主题>/`。
- 上游合并：在 `upstream-sync/<YYYY-MM-DD>-v<版本>/review.md` 写审核报告，并向 `upstream-sync/merge-log.md` 追加条目。
- 新增目录已被 `.gitignore` 放行的范围：`README.md`、`features/`、`reviews/`、`delivery/`、`upstream-sync/`、`superpowers/`。`docs/` 根目录新增文件默认被忽略，需在 `.gitignore` 单独放行。

## 保持原路径的文件

以下文件被源码或 SQL migration 注释引用。migration 文件受 checksum 校验，不能为改路径而修改，因此对应设计文档不要移动：

- `features/token-analysis-project-attribution-design-cn.md`（`backend/migrations/145_*.sql`、`project_attribution.go`）
- `features/token-analysis-user-input-store-design-cn.md`（`backend/migrations/146_*.sql`）
- `features/gpt-account-quota-display-design-cn.md`（`backend/migrations/243_*.sql`、`gpt_quota_display.go`）
- `features/codex-prompt-risk-filter-design-cn.md`（`prompt_risk.go`）
- `features/organization-department-usage-design-cn.md`（`frontend/src/components/admin/organization-usage/README.md`）

## 路径迁移记录

历史报告和合并台账保留当时的路径，按下表查找现位置。2026-09-23 的审核文档迁移见 [reviews/features-review-archive-index.md](reviews/features-review-archive-index.md)。

### 2026-09-30

| 原位置 | 现位置 |
| --- | --- |
| `docs/features/sub2api -merage-list.md` | [upstream-sync/merge-log.md](upstream-sync/merge-log.md) |
| `docs/upstream-merge-playbook.md` | [upstream-sync/playbook.md](upstream-sync/playbook.md) |
| `docs/delivery/2026-07-13-sub2api-v0.1.153-sync/` | [upstream-sync/2026-07-13-v0.1.153/](upstream-sync/2026-07-13-v0.1.153/) |
| `docs/delivery/2026-07-14-sync-sub2api-v0-1-155/` | [upstream-sync/2026-07-14-v0.1.155/](upstream-sync/2026-07-14-v0.1.155/) |
| `docs/delivery/2026-07-16-sub2api-v0.1.156-sync/` | [upstream-sync/2026-07-16-v0.1.156/](upstream-sync/2026-07-16-v0.1.156/) |
| `docs/delivery/2026-07-17-sub2api-v0.1.159-sync/` | [upstream-sync/2026-07-17-v0.1.159/](upstream-sync/2026-07-17-v0.1.159/) |
| `docs/delivery/2026-08-09-sub2api-v0.1.173-sync/` | [upstream-sync/2026-08-09-v0.1.173/](upstream-sync/2026-08-09-v0.1.173/) |
| `docs/delivery/2026-09-09-sub2api-v0.2.4-sync/` | [upstream-sync/2026-09-09-v0.2.4/](upstream-sync/2026-09-09-v0.2.4/) |
| `docs/delivery/2026-09-18-sub2api-v0.2.6-sync/` | [upstream-sync/2026-09-18-v0.2.6/](upstream-sync/2026-09-18-v0.2.6/) |
| `docs/delivery/2026-09-20-sub2api-v0.2.7-sync/` | [upstream-sync/2026-09-20-v0.2.7/](upstream-sync/2026-09-20-v0.2.7/) |
| `docs/delivery/2026-09-29-sub2api-v0.2.9-sync/` | [upstream-sync/2026-09-29-v0.2.9/](upstream-sync/2026-09-29-v0.2.9/) |
| `docs/delivery/2026-09-30-sub2api-v0.2.10-sync/` | [upstream-sync/2026-09-30-v0.2.10/](upstream-sync/2026-09-30-v0.2.10/) |
| `docs/reviews/2026-07-10-upstream-9a2f11b-merge-review.md` 等 10 份上游合并审核（`*-merge-review*`、`2026-07-16-sub2api-v0.1.156-acceptance-review.md`、`2026-07-19-sub2api-v0.1.161-conflict-resolution-audit.md`） | [upstream-sync/reviews/](upstream-sync/reviews/) 下同名文件 |
| `docs/features/organization-department-usage-code-review-cn.md` | [reviews/](reviews/organization-department-usage-code-review-cn.md) 同名 |
| `docs/features/organization-department-usage-design-review-cn.md` | [reviews/](reviews/organization-department-usage-design-review-cn.md) 同名 |
| `docs/features/organization-department-usage-implementation-audit-cn.md` | [reviews/](reviews/organization-department-usage-implementation-audit-cn.md) 同名 |
| `docs/features/shared-compute-pool-claude-revision-review-cn.md` | [reviews/](reviews/shared-compute-pool-claude-revision-review-cn.md) 同名 |
| `docs/features/shared-compute-pool-three-options-review-cn.md` | [reviews/](reviews/shared-compute-pool-three-options-review-cn.md) 同名 |
| `docs/features/subscription-self-daily-reset-implementation-review-cn.md` | [reviews/](reviews/subscription-self-daily-reset-implementation-review-cn.md) 同名 |

同日从 `d120d499e` 恢复了在后续功能分支合并中丢失的 `upstream-sync/2026-09-20-v0.2.7/review.md` 与 `claude-review-request.md`（wiki 与台账引用）。
