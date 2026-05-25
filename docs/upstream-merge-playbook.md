# 上游合并依据文档

本文档用于在合并源仓库 `Wei-Shaw/sub2api` 时给人工维护者或 Codex 提供判断依据。目标不是自动解决冲突，而是在冲突出现时明确哪些本地能力必须保留，哪些上游变更需要兼容融合。

## 基本原则

| 原则 | 要求 |
| --- | --- |
| 先读文档 | 合并 `upstream/main` 前必须先阅读本文档。 |
| 语义合并 | 冲突文件不能无脑选择 `ours` 或 `theirs`，必须理解双方改动后融合。 |
| 保留本地能力 | token 分析、生图、并发方案、Redis 7+、大请求保护等本地能力不能被上游覆盖掉。 |
| 吸收上游修复 | 上游 bugfix、安全修复、模型兼容、依赖升级应尽量保留。 |
| 不做 Redis 3 兼容 | 正式环境要求 Redis 7+ 或兼容 Redis 7+ 的 Memurai，不应重新引入 Redis 3 workaround。 |
| 不提交运行数据 | `fixtures/`、`report/`、`runtime-report*`、生产请求样本和本地运行报告不得提交。 |
| 可验证后再上线 | 合并后必须跑测试，并输出具体通过/失败项。 |

## 推荐流程

| 步骤 | 命令/动作 |
| --- | --- |
| 确认工作树 | `git status --short --branch`，若有无关改动先说明，不要覆盖。 |
| 更新远端 | `git fetch --all --prune` |
| 建合并分支 | `git checkout main && git checkout -b codex/merge-upstream-sub2api-YYYY-MM-DD` |
| 合并上游 | `git merge upstream/main` |
| 解决冲突 | 按本文档的本地能力和高风险文件逐项检查。 |
| 验证 | 后端测试、前端测试/构建、关键接口/页面冒烟。 |
| 合回 main | 验证通过后再合并或提交 PR，不要直接强推。 |

## 本地长期维护能力

| 能力 | 主要目的 | 合并时必须保留 |
| --- | --- | --- |
| token 分析页面 | 从 request archive 和 usage 日志分析 token 使用、浪费、风险请求和生产样本。 | `token_analysis` 配置、索引、API、前端页面、风险原因和大 tool 输出指标。 |
| request archive / intercept 管理 | 捕获请求样本、支持请求拦截管理和调试。 | 请求归档写入链路、拦截规则、管理页面和相关配置。 |
| 生图工具页面 | 提供 `/image-gen` 操作页，使用用户 API Key 调用图片生成网关并保留本地历史。 | 前端页面、图片生成网关适配、Codex image generation bridge 和相关并发控制。 |
| 用户并发方案 | 管理员保存、手动应用、定时应用一组用户并发配置。 | 数据库迁移、repository、service、admin API、前端按钮/弹窗和后台 runner。 |
| Redis 7+ 要求 | 避免 Redis 3 Lua 限制导致并发槽位脚本失败。 | Redis 版本校验、Redis/cache 错误分类、不要恢复 Redis 3 `TIME` workaround。 |
| 并发错误分类 | 区分真实并发满和 Redis/cache 基础设施错误。 | Redis/cache/Lua 错误不能伪装成用户 429，应返回明确依赖错误或触发降级策略。 |
| OpenAI Responses/Chat 兼容 | 兼容 Chat Completions、Responses、Codex/OAuth、第三方 OpenAI 兼容上游。 | `prompt_cache_key`、`previous_response_id`、reasoning、service_tier、Responses 支持探测和 fallback。 |
| 大请求保护 | 针对超大 Chat Completions 请求中的历史 `role=tool` 输出进行观测和可控压缩。 | 默认 `warn` 只读；只有 `tool_output_compact` 且命中 allowlist 才改写请求。 |
| 空响应兜底 | 上游 200 但无可见输出/无 usage/reasoning-only 时不能返回成功空流。 | reasoning-only empty stream 必须触发 failover 或错误兜底。 |
| 本地启动/运维兼容 | Windows 本地启动、CORS、本地配置和运维监控适配。 | `start-local`、`stop-local`、本地 config 生成逻辑和健康检查约定。 |

## 高风险冲突区域

| 区域 | 典型文件 | 合并注意事项 |
| --- | --- | --- |
| 配置 | `backend/internal/config/config.go` | 新配置必须有默认值和校验；不要让默认值改变生产请求行为。 |
| OpenAI 网关 | `backend/internal/service/openai_gateway*.go` | 这里冲突最多，必须保留上游新兼容逻辑，同时保留本地 Responses/Chat、图片、生图、大请求和空响应兜底。 |
| 协议转换 | `backend/internal/pkg/apicompat/*` | 不能丢 `prompt_cache_key`、`previous_response_id`、reasoning、tool call、usage/cache token 兼容。 |
| Redis 并发 | `backend/internal/repository/concurrency_cache.go`、`backend/internal/service/concurrency_service.go` | 不要重新引入 Redis 3 workaround；Redis 错误不能被包装成真实并发 429。 |
| 账号调度 | `backend/internal/repository/group_repo.go`、`backend/internal/service/*scheduler*` | 上游若改调度或 cooldown，要和本地 sticky、并发、账号可调度状态融合。 |
| 后台任务 | `backend/internal/server/*`、`backend/internal/service/*runner*` | 用户并发方案 runner、token analysis indexer、清理任务不能丢。 |
| 数据库迁移 | `backend/migrations/*.sql`、`backend/ent/*` | 迁移编号冲突时要重排或确认已执行状态；不要删除本地迁移。 |
| 管理 API | `backend/internal/handler/admin/*`、`backend/internal/service/admin_*` | 并发方案、token 分析、生图/运维接口需要保留权限和路由。 |
| 前端 API | `frontend/src/api/*` | 上游 API 类型变化要同步，但不能删除本地 admin client。 |
| 前端页面 | `frontend/src/views/*`、`frontend/src/components/*` | token 分析、生图、并发方案入口和交互需要保留。 |
| 国际化 | `frontend/src/i18n/*` | 上游新增 key 要合入，本地新增页面 key 不能丢。 |
| 忽略规则 | `.gitignore` | 必须继续忽略 report、fixtures、runtime-report 和生产样本。 |

## 必须保留的行为断言

| 行为 | 期望 |
| --- | --- |
| 大请求默认模式 | `gateway.large_request.mode=warn` 时只记录日志，不压缩、不注入 `prompt_cache_key`。 |
| 大请求压缩范围 | 只压缩超大历史 `role=tool` 字符串输出；最近 6 条 tool 和最后 user 之后的 tool 强保护。 |
| 大请求开关 | 只有 `mode=tool_output_compact` 且命中 `enabled_user_ids`、`enabled_api_key_ids` 或 `enabled_group_ids` 才改写。 |
| Redis 版本 | 正式环境使用 Redis 7+ 或 Memurai Redis 7+ 兼容版本。 |
| Redis 错误 | Redis 连接、Lua、cache 错误不能表现成用户真实并发满 429。 |
| 用户并发方案 | 空方案列表不能导致前端失败；方案只能修改用户并发数，不应误改账号并发。 |
| 生图 | `/image-gen` 页面和图片生成网关可用，不能被普通文本 Codex 请求误触发。 |
| 空上游响应 | 200 但无可见内容、无 tool calls、无 usage，不能返回成功空响应。 |
| 生产样本 | `fixtures/`、`report/`、`runtime-report*` 不提交。 |

## 验证清单

| 类型 | 命令/动作 |
| --- | --- |
| 工作树 | `git status --short --branch` |
| 后端重点测试 | `cd backend && go test -count=1 ./cmd/server ./internal/pkg/apicompat ./internal/service ./internal/handler ./internal/handler/admin` |
| 前端测试 | `cd frontend && pnpm test:run` |
| 前端构建 | `cd frontend && pnpm build` |
| 忽略规则 | `git status --short` 不应出现 `fixtures/`、`report/`、`runtime-report*`。 |
| Redis | `redis-cli INFO server` 应显示 Redis 7+ 兼容版本；`redis-cli PING` 返回 `PONG`。 |
| 健康检查 | `/health` 返回 200。 |
| 日志检查 | 不应出现 Redis 3 Lua 错误、误分类 429、大量空 200 响应。 |

## 给服务器 Codex 的可复制提示

```text
请按仓库内 docs/upstream-merge-playbook.md 合并源仓库 Wei-Shaw/sub2api 的最新 main。

要求：
1. 先只读检查 git status、remote、当前分支、最近提交和未跟踪文件。
2. fetch --all --prune 后，从当前 main 创建临时合并分支。
3. merge upstream/main，如有冲突，禁止无脑 ours/theirs，必须逐文件说明双方改动和融合理由。
4. 必须保留本地能力：token 分析、生图工具、用户并发方案、Redis 7+ 要求、并发错误分类、OpenAI Responses/Chat 兼容、大请求 role=tool 压缩、空响应兜底。
5. 不要提交 fixtures/、report/、runtime-report* 或生产请求样本。
6. 合并后运行后端重点测试、前端测试和前端构建；如无法运行，说明原因。
7. 输出：修改文件清单、冲突解决摘要、保留的本地能力检查结果、测试结果、是否建议上线。
```

## 合并结果摘要模板

| 项目 | 结果 |
| --- | --- |
| 当前分支 |  |
| 合并的 upstream commit |  |
| 冲突文件 |  |
| 本地能力是否保留 |  |
| 上游新增功能是否合入 |  |
| 后端测试 |  |
| 前端测试 |  |
| 前端构建 |  |
| 剩余风险 |  |
