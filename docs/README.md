# docs 目录索引

本目录存放上游自带文档、本地功能文档、演示材料和上游同步记录。AI 开发前仍先读 `llm-wiki/wiki/README.md`；本页说明文档放在哪里、新文档该放哪里。

## 核心规则：一个功能一份文档

- 每个功能只保留一份文档 `features/<feature>-design-cn.md`，面向 Claude 阅读：现行规则、接口、数据模型/迁移、配置、代码地图（函数名，不写行号）、关键约束与易错点（规则 + 依据）、验证命令与最近结果、状态与待办。
- 不建 v2/v3 文档，不写"第 N 轮审核"过程章节。审核、复核、交付证据、执行计划不进仓库：采纳的结论直接改进设计正文或易错点；未采纳但对后续有约束的写成"已否决：X，原因 Y"；原文先归档到仓库外再删除。
- 2026-11-04 之前的审核报告、证据目录和执行计划已全部按此规则并入对应功能文档，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<旧路径>`），对照见下方「2026-11-04 合并对照」。

## 目录结构

| 位置 | 内容 | 维护方式 |
| --- | --- | --- |
| `PAYMENT.md`、`PAYMENT_CN.md`、`ADMIN_PAYMENT_INTEGRATION_API.md`、`ASYNC_IMAGE_TASKS.md`、`BATCH_IMAGE_MVP.md`、`COMPOSITE_GROUPS.md`、`PLUGIN_DEVELOPMENT.md`、`OPENAI_GPT55_RETIREMENT.md`、`admin-api-keys.md`、`channel-monitor-v2-safe-defaults.md` | 上游 `Wei-Shaw/sub2api` 自带文档 | 保持原位，随上游合并；README 和设置页链接指向这些路径 |
| `legal/` | 管理员合规文档（上游） | 不可移动：前端 `?raw` 打包、`Dockerfile`、`.dockerignore` 和后端常量引用该路径 |
| `ARCHITECTURE_AND_OPS_HANDBOOK.md` | 本地架构与运维总手册（给人读） | 根 README 引用，保持原位 |
| `安装与操作手册.md` | 历史安装包 `main@b3ff5895`（2026-05-27）的 Ubuntu 20.04 一键安装说明，不是现行手册 | 历史快照；配套脚本只在该安装包内 |
| `features/` | 每个功能一份唯一文档（见下表），另有技术债看板 | 活文档，结论变化时直接改正文 |
| `presentations/` | 面向人的 HTML 演示材料（汇报页、介绍页） | 文字结论以对应功能文档为准；不放审核或证据 |
| `upstream-sync/` | 上游合并台账、流程和每轮同步记录 | 见下节 |

### features/ 功能文档

| 文档 | 功能 |
| --- | --- |
| `token-analysis-design-cn.md` | Token Analysis 基础：页面、归档索引器、概览/排行、紧凑数字、选中用户趋势、故障排查与恢复（子功能文档的入口） |
| `token-analysis-risk-classification-design-cn.md` | Token Analysis 风险分类（成本观察与缓存复用） |
| `token-analysis-request-origin-design-cn.md` | Token Analysis 请求来源（人工/机器）识别、样本与证据集 |
| `token-analysis-usage-link-design-cn.md` | 归档请求与 usage 计费记录关联、存量恢复、部署核对 |
| `token-analysis-project-attribution-design-cn.md` | 项目归因（含 2026-11-03 新分类规则与采集结论） |
| `token-analysis-user-input-store-design-cn.md` | 用户净输入留存与请求明细页 |
| `request-archive-design-cn.md` | 网关请求本地 JSONL 归档（异步写入、响应只存 usage、目录热切换） |
| `organization-usage-report-design-cn.md` | 组织用量报表（口径、SQL、趋势图、日汇总、性能基线、展示、Excel） |
| `organization-department-usage-design-cn.md` | 组织下部门维度、部门负责人报表与受限额度重置 |
| `department-member-management-design-cn.md` | 部门负责人管理本部门成员 |
| `organization-leader-design-cn.md` | 组织负责人 |
| `sub-admin-role-design-cn.md` | 子管理员角色与全部权限码 |
| `sub-admin-accounts-operator-design-cn.md` | 子管理员受限账号运维 `admin.accounts_operator` |
| `subscription-self-daily-reset-design-cn.md` | 用户订阅自助日重置 |
| `subscription-daily-remaining-sort-bulk-reset-design-cn.md` | 管理端订阅按每日剩余排序与批量重置 |
| `gpt-account-quota-display-design-cn.md` | GPT 账号额度展示 |
| `codex-prompt-risk-filter-design-cn.md` | Codex Prompt 风险过滤器与 Prompt Risk Judge |
| `openai-request-locale-design-cn.md` | OpenAI 请求时区改写（分支保留、未合入 main） |
| `openai-default-reasoning-effort-design-cn.md` | Chat Completions 缺省 reasoning_effort 注入 |
| `prompt-metrics-feature-technical-notes-cn.md` | Prompt Metrics |
| `image-gen-design-cn.md` | 图片生成页 |
| `claude-client-setup-design-cn.md` | Claude Code for VS Code 接入页与 Windows / macOS 本地配置脚本 |
| `user-concurrency-presets-design-cn.md` | 用户并发预设 |
| `large-chat-tool-output-compaction-design-cn.md` | 大 Chat 请求历史 tool 输出压缩 |
| `shared-compute-pool-design-cn.md` | 共享算力池普通用户保障（建议方案，未上线） |
| `technical-debt-board-cn.md` | 技术债看板（含 golangci-lint 基线、未合并分支清理等遗留项） |

### upstream-sync/

| 位置 | 内容 |
| --- | --- |
| `merge-log.md` | 上游合并台账，**只追加**，每次合并后必须写入（见 `AGENTS.md`）；历史条目保留当时的路径 |
| `playbook.md` | 合并流程、必须保留的本地能力和验证清单 |
| `<YYYY-MM-DD>-v<版本>/review.md` | 每轮同步唯一记录：范围、冲突与处理、保留的本地能力、验证、遗留问题 |

## 新文档放置规则

- 新功能：在 `features/` 新建 `<feature>-design-cn.md`；已有功能的设计修订、审核结论、交付验证结果都直接改这份文档。
- 审核报告、复核证据、验收截图、执行计划：不入库。需要留档时放仓库外（例如 `E:\tmp\<feature>_archive\<日期>\`），先归档再删除。
- 面向人的演示材料：放 `presentations/`，并在对应功能文档里链接。
- 上游合并：写 `upstream-sync/<YYYY-MM-DD>-v<版本>/review.md`，并向 `upstream-sync/merge-log.md` 追加条目。
- `.gitignore` 只放行 `README.md`、`features/`、`presentations/`、`upstream-sync/` 及根目录列出的上游文档；`docs/` 下其他新目录默认被忽略。
- 合并旧分支时，对照下方迁移表把有效新增结论并入现行文档，不恢复已整合的旧路径或另建审核/实施文档。`.gitignore` 不阻止已跟踪文件回流；提交前同时检查新增、恢复及未跟踪文件。
- 功能文档就地更新规则、决策和最近验证边界，不连续追加每轮过程；wiki 保留现行维护知识，历史 SHA、冲突过程和完整测试输出不在各页重复堆积。上游每轮一份 `review.md` 与只追加台账允许随版本增长；上游自带文档和受源码引用的文件按既有约束保留。

## 保持原路径与章节号的文件

以下文档被源码或 SQL migration 注释按路径（部分按章节号）引用。migration 文件受 checksum 校验，不能为改路径而修改，因此这些文档不要移动，被引用的章节号不要重排：

- `features/token-analysis-risk-classification-design-cn.md`：migration 249（4.2）、250（11.2）、251（第 5 节）、253、255–259；服务代码按第 5/7/8 节引用；测试按审核编号 R1–R8 引用（编号对照在第 17 节）
- `features/token-analysis-project-attribution-design-cn.md`：migration 145、`project_attribution.go`、`token_analysis_types.go`
- `features/token-analysis-user-input-store-design-cn.md`：migration 146
- `features/token-analysis-usage-link-design-cn.md`：migration 247
- `features/token-analysis-request-origin-design-cn.md`：`token_analysis_origin.go`（§3「分类规则（v2 修订）」）、`token_analysis_origin_test.go`（§6 样本与证据集）
- `features/gpt-account-quota-display-design-cn.md`：migration 243、`gpt_quota_display.go`
- `features/codex-prompt-risk-filter-design-cn.md`：`prompt_risk.go`
- `features/organization-department-usage-design-cn.md`：`frontend/src/components/admin/organization-usage/README.md`
- `features/department-member-management-design-cn.md`：成员管理测试按 §2、§3、§3.1、§4.4、§9.1 和审核编号 R3 引用
- `features/organization-leader-design-cn.md`：`organization_leader_integration_test.go`（§11）；组织负责人相关仓储/服务注释按节号引用（如 `design §4.4`）
- `features/image-gen-design-cn.md`：`frontend/src/stores/imageGen.ts`、`frontend/src/utils/imageGenModels.ts`

## 2026-11-04 合并对照

旧路径均可用 `git show 26f6363af:<旧路径>` 查看。

| 原位置 | 现位置 |
| --- | --- |
| `reviews/2026-10-26-token-analysis-data-layer-code-review-cn.md`、`reviews/2026-10-27-token-analysis-branch-*`（5 份）、`delivery/2026-10-26-token-analysis-code-review/`、`delivery/2026-10-27-token-analysis-branch-*`（5 个目录） | `features/token-analysis-risk-classification-design-cn.md`（编号对照第 17 节） |
| `reviews/2026-10-24-token-analysis-request-origin-implementation-review-cn.md`、`reviews/2026-10-27-token-analysis-production-origin-audit-cn.md`、`reviews/2026-10-28-token-origin-v2-implementation-review-cn.md`、`delivery/2026-10-24-token-origin-samples/`、`delivery/2026-10-24-token-origin-human-input/`、`delivery/2026-10-27-token-origin-audit/`、`delivery/2026-10-27-compaction-followup/`、`delivery/2026-10-27-token-origin-v2-evidence/`、`delivery/2026-10-28-token-origin-v2-review/` | `features/token-analysis-request-origin-design-cn.md`（证据集 §6、审计 §7） |
| `reviews/2026-10-24-token-origin-human-input-investigation-cn.md`、`reviews/2026-10-24-token-analysis-usage-link-*`（3 份）、`reviews/2026-10-25-token-analysis-usage-link-fourth-review-cn.md`、`delivery/2026-10-25-token-analysis-deployment.md` | `features/token-analysis-usage-link-design-cn.md` |
| `superpowers/*/2026-05-19-token-waste-analysis*`、`superpowers/plans/2026-05-21-token-analysis-production-data.md`、`superpowers/*/2026-07-13-token-analysis-compact-number-format*`、`reviews/2026-07-14-token-analysis-compact-number-format-design-review.md`、`superpowers/*/2026-07-13-token-analysis-selected-user-trend*`、`features/token-analysis-recovery-design-cn.md`、`reviews/2026-10-25-token-analysis-index-errors-investigation-cn.md`、`reviews/2026-10-27-token-analysis-eight-index-errors-investigation-cn.md`、`reviews/2026-11-02-token-analysis-disk-errors-investigation-cn.md`、`delivery/2026-11-02-token-analysis-repair/` | `features/token-analysis-design-cn.md`（来源清单 §17） |
| `features/token-analysis-project-classification-rules-cn.md`、`delivery/2026-11-03-project-attribution-survey/` | `features/token-analysis-project-attribution-design-cn.md`（§7、§8） |
| `reviews/token-analysis-user-input-retention-*`（2 份） | `features/token-analysis-user-input-store-design-cn.md` |
| `superpowers/*/2026-05-15-local-request-archive*`、`superpowers/plans/2026-06-07-request-archive-response-usage-storage.md`、`features/request-archive-async-writer-technical-notes-cn.md`、`features/request-archive-response-usage-storage-design-cn.md`、`features/request-archive-dir-runtime-config-design-cn.md`、`reviews/request-archive-*`（2 份） | `features/request-archive-design-cn.md` |
| `features/organization-usage-report-performance-cn.md`、`features/organization-usage-daily-rollup-design-cn.md`、`features/organization-usage-trend-chart-design-cn.md`、`reviews/2026-07-13-organization-usage-report-review.md`、`reviews/2026-07-27-organization-usage-trend-chart-*`（3 份）、`superpowers/*/2026-07-13-organization-usage-review-fixes*`、`superpowers/*/2026-08-09-organization-usage-report-display*`、`superpowers/*/2026-08-10-organization-usage-dashboard-polish*`、`delivery/2026-08-09-organization-usage-report-display/` | `features/organization-usage-report-design-cn.md`（性能 §7.1、日汇总 §6、趋势图 §4.4/§5.4/§8.5） |
| `features/organization-department-usage-implementation-plan-cn.md`、`reviews/organization-department-usage-*`（3 份）、`reviews/2026-09-29-feature-10207-release-audit.md`、`delivery/2026-09-19-department-usage/` | `features/organization-department-usage-design-cn.md`（验收与性能 §11、易错点 §14） |
| `features/department-member-management-implementation-plan-cn.md` | `features/department-member-management-design-cn.md`（§4.4、§9） |
| `delivery/2026-07-15-sub-admin-role/`、`reviews/2026-07-15-sub-admin-role-code-review.md`、`reviews/2026-07-17-sub-admin-role-pre-merge-fixes.md` | `features/sub-admin-role-design-cn.md` |
| `reviews/sub-admin-accounts-operator-implementation-audit-cn.md` | `features/sub-admin-accounts-operator-design-cn.md` |
| `features/subscription-self-daily-reset-implementation-cn.md`、`reviews/subscription-self-daily-reset-*`（4 份） | `features/subscription-self-daily-reset-design-cn.md` |
| `superpowers/*/2026-08-12-subscription-daily-remaining-sort-bulk-reset*`、`superpowers/plans/2026-08-12-subscription-sort-param-hotfix.md` | `features/subscription-daily-remaining-sort-bulk-reset-design-cn.md` |
| `features/image-gen-model-selection-design-cn.md`、`features/image-gen-state-persistence-multi-image-design-cn.md`、`superpowers/*/2026-05-24-image-gen-tool*` | `features/image-gen-design-cn.md` |
| `superpowers/*/2026-05-23-user-concurrency-presets*` | `features/user-concurrency-presets-design-cn.md` |
| `superpowers/*/2026-05-25-large-chat-tool-output-compaction*` | `features/large-chat-tool-output-compaction-design-cn.md` |
| `reviews/codex-prompt-risk-filter-*`（2 份）、`reviews/prompt-risk-judge-release-review-cn.md` | `features/codex-prompt-risk-filter-design-cn.md` |
| `reviews/gpt-account-quota-display-design-review-cn.md` | `features/gpt-account-quota-display-design-cn.md` |
| `reviews/2026-09-29-sub4api-request-locale-review-cn.md`、2026-11-03 未合并分支复审中的 U4 部分 | `features/openai-request-locale-design-cn.md` |
| `features/shared-compute-pool-independent-design-brief-cn.md`、`features/shared-compute-pool-protection-report-cn.md`、`reviews/shared-compute-pool-*`（2 份） | `features/shared-compute-pool-design-cn.md` |
| `features/shared-compute-pool-protection-report-cn.html`、`features/shared-compute-pool-three-options-comparison-cn.html`、`features/shared-compute-pool-protection-assets/`、`delivery/2026-10-29-company-introduction/` | `presentations/` 下同名 |
| `features/golangci-lint-debt-cleanup-plan-cn.md`、`reviews/2026-11-03-unmerged-work-*`（3 份）、`reviews/2026-11-03-main-pr45-47-validation-cn.md` | `features/technical-debt-board-cn.md`（TD-004、TD-044–046） |
| `reviews/2026-10-27-huangyu-pankun-ttft-investigation-cn.md`、`delivery/2026-10-26-main-source-deployment.md` | `llm-wiki/wiki/ops.md`（首字耗时排查、部署后核对清单） |
| `upstream-sync/<版本>/` 下的 requirements/specs/plan/test-review/delivery-*/claude-review-*、`upstream-sync/reviews/*`、`superpowers/plans/*sync*`/`*upstream*` | 对应版本的 `upstream-sync/<YYYY-MM-DD>-v<版本>/review.md` |
| `reviews/features-review-archive-index.md`（2026-09-23 迁移索引） | 本页；2026-09-23、2026-09-30 两次迁移涉及的文件已全部并入上表目标 |
