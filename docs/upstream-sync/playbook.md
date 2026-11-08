# 上游合并依据文档

本文档用于在合并源仓库 `Wei-Shaw/sub2api` 时给人工维护者或 Codex 提供判断依据。目标不是自动解决冲突，而是在冲突出现时明确哪些本地能力必须保留，哪些上游变更需要兼容融合。

## 基本原则

| 原则 | 要求 |
| --- | --- |
| 先读文档 | 合并 `upstream/main` 前必须先阅读本文档、`docs/README.md` 与 `llm-wiki/wiki/ai-workflow.md` 的文档归档规则。 |
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
| 建合并分支 | `git checkout main && git checkout -b feature/hy/<需求号>_merge_sub2api_<版本>`，例如 `feature/hy/10213_merge_sub2api_210` |
| 合并上游 | 记录 `upstream/main` 当前 SHA，`git merge --no-commit --no-ff <固定 SHA>`；合并期间上游前进也不跟随，用户审核后才 commit |
| 解决冲突 | 按本文档的本地能力和高风险文件逐项检查。 |
| 验证 | 后端测试、前端测试/构建、关键接口/页面冒烟。 |
| 合回 main | 验证通过后再合并或提交 PR，不要直接强推。 |

## 记录位置

| 产出 | 位置 |
| --- | --- |
| 合并台账（只追加） | `docs/upstream-sync/merge-log.md` |
| 本轮审核与验证报告（每个版本只此一份） | `docs/upstream-sync/<YYYY-MM-DD>-v<版本>/review.md`；结构：元数据行、1 范围、2 冲突与处理、3 保留的本地能力、4 采用/未采用的上游行为、5 验证、6 遗留问题 |
| 独立复审（如有） | 结论直接并入同目录 `review.md`，不另存任务书/结果文件；新原文先归档到仓库外并确认可读取。此前已合并的 `claude-review-*.md`、requirements/specs/plan/delivery 材料可从提交 `26f6363af` 查看；不能假定未提交的新证据也能从 Git 恢复 |

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
| 文档收口 | 检查工作区、暂存区和新增文件：每轮同步目录仅一份 `review.md`，无新建或恢复的 reviews/delivery/superpowers、重复功能文档或逐轮任务报告；对照 `docs/README.md` 迁移表归并有效增量，不能只凭 `.gitignore` 或文件总数判断 |
| 后端重点测试 | `cd backend && go test -count=1 ./cmd/server ./internal/pkg/apicompat ./internal/service ./internal/handler ./internal/handler/admin` |
| 前端测试 | `cd frontend && pnpm test:run` |
| 前端构建 | `cd frontend && pnpm build` |
| 忽略规则 | `git status --short` 不应出现 `fixtures/`、`report/`、`runtime-report*`。 |
| Redis | `redis-cli INFO server` 应显示 Redis 7+ 兼容版本；`redis-cli PING` 返回 `PONG`。 |
| 健康检查 | `/health` 返回 200。 |
| 日志检查 | 不应出现 Redis 3 Lua 错误、误分类 429、大量空 200 响应。 |
| 后端三种 tag 全量 | `cd backend && go test -p 1 -count=1 ./...`、`-tags=unit`、`-tags=integration` 各跑一次；CI 单测门禁是 `-tags=unit`，默认 tag 跑不到 unit 专属用例（0.1.170 起三者分列记录） |
| 增量 lint | `golangci-lint run --new-from-rev=<第一父> ./...` 必须 0 issues。2026-11-08 起全量 lint 为 0（TD-004 关闭），合并后还要跑一次全量（加 `--max-same-issues=0 --max-issues-per-linter=0`）；上游代码带入的新问题记入 review.md，合并后另开 chore 修，不在合并分支改上游代码（0.1.153 起）。golangci-lint 必须用不低于仓库 Go 版本构建的二进制，旧版会报不兼容或 `no go files to analyze`，此时不能记为 0 issues（0.1.170、0.2.4） |
| 生成一致性 | `go generate ./ent` 与 `go generate ./cmd/server` 各跑两次，第二次无漂移（比对 `wire_gen.go` 哈希）；Wire 工具临时写入 `go.sum` 的 checksum（如 `github.com/google/subcommands`）要还原（0.1.135、0.1.156、0.2.6、0.2.15、0.2.17） |
| 嵌入式构建 | 前端 `pnpm run build` 成功之后再 `go build -tags embed -trimpath ./cmd/server`，确保嵌入的是本轮 dist（0.1.153、0.1.156） |
| 依赖元数据 | `go mod tidy -diff` 只记录结果，不为合并顺手整理 `go.sum`（0.1.156、0.1.159、0.2.4） |
| Git 硬门 | `git ls-files -u` 为空；无冲突标记；`git diff --cached --check` 通过；仅上游路径逐 blob 等于目标、仅本地路径逐 blob 等于第一父；`docs/features/` tracked 删除为 0（0.1.156 起每轮执行） |

## 跨版本固定规则

以下规则来自历次同步记录，括号内为来源版本（对应 `docs/upstream-sync/*-v<版本>/review.md`）。

### 边界与流程

| 规则 | 要求 |
| --- | --- |
| 固定 SHA，停在提交前 | 用 `git merge --no-commit --no-ff <固定 SHA>`，保留 `MERGE_HEAD` 等用户审核后再 commit；会话期间 `upstream/main` 前进也不跟随（0.1.153、0.1.156 起） |
| tag 的 VERSION 可能滞后 | 上游常先打 tag 再单独提交 VERSION 同步；先看 `git show <tag>^{}:backend/cmd/server/VERSION`，滞后时以其后的 version-sync 提交为边界（0.1.156、0.1.160、0.1.161、0.1.162） |
| 目标是"最新 main"时 | 收尾前再 fetch；若上游前进，保存无提交 tree、中止并重新合并新目标，再核对新旧快照差异恰好等于上游新增增量（0.1.159） |
| 上游重写历史 | `forced update` 导致已合入的上游功能不在新目标历史中时，先问用户是否跟随移除；跟随时用"旧上游 → 共同祖先"的精确反向差异删除，不动数据库与线上配置（0.2.7 移除 Codex ticket） |
| 只合入 PR head | 叠加上游 PR 时合入 PR head SHA，不用 GitHub 临时 merge ref（可能夹带更新的主线）；先确认 PR base 等于已合入边界（0.2.4 PR #6924、0.2.17 PR #8139） |

### 冲突与语义审查

| 规则 | 要求 |
| --- | --- |
| 双方修改集合 | 用 `merge-base..第一父` 与 `merge-base..目标` 的文件集合求交，逐个三方审查；"无文本冲突"不等于"无行为冲突"（0.1.155、0.1.156 起） |
| 合并决策五分法 | 仅本地→保留；仅上游→采用；双方互补→手工组合并验证入口/数据结构/调用链；双方等价且上游完整覆盖→采用上游并删除本地重复实现；上游缺本地安全或业务边界→保留本地边界并把上游能力接进来（0.1.156） |
| `wire_gen.go` | 不选 ours/theirs，先解决 `wire.go` / ProviderSet 源图再重新生成；生成报缺 provider 时补 `wire.Bind`（0.1.156 `GrokOAuthTokenService`）；生成指令必须是 `//go:generate go run -mod=mod github.com/google/wire/cmd/wire`（0.1.135、0.1.155） |
| `ProvidePluginManager` 本地分叉 | 上游在生成代码里手写 `pluginManager.SetAccountDirectory(...)`，本地已移到 `backend/internal/service/wire.go#ProvidePluginManager`。冲突以本地重新生成结果为准；上游改了 `NewPluginManager` 参数或接线时同步改 `ProvidePluginManager`，重新生成后确认仍有 `SetAccountDirectory`；上游自己并入源图后删除本地 provider（0.2.7 W1、2026-09-23 台账） |
| 兼容 helper 只补缺 | 本地兼容解析（如 compatible cache usage）叠在上游统一解析器之后时，只在解析出正值时写回，不能用 0 覆盖上游已解析字段（0.1.150 cache write 被清零） |
| 上游接口变更导致本地代码不编译 | 只做最小转接（如 0.2.7 `moderationTextCollector` 薄转接、0.1.159 测试桩补 `BatchUpdateLimits`），保持第一父行为，不改上游路径 |
| 上游测试改角色 | 本地 `userRepository.Update` 改 Role/AdminPermissions 必须带 `ExpectedAdminAccessVersion`（否则 `ADMIN_ACCESS_VERSION_REQUIRED`）；上游测试直接改角色时用 `GetByIDWithAdminAccess` 取版本再传入，断言不改。这类用例多在 integration tag，需隔离 PG 才能跑到（0.2.19、0.2.20） |
| i18n 拆分类合并 | `vue-tsc` 与现有守卫测试对缺键无感；需对本地组件引用的 key 做存在性检索，并确认 `git status` 中 `frontend/src/i18n` 没有 `??` 新模块（0.1.146） |
| 上游自带的格式问题 | 上游 blob 自带 trailing whitespace（如 source-freeze patch、Channel Monitor 文档）时原样保留，diff check 排除该文件并在审核中说明（0.1.160、0.1.173） |
| migration | 只新增不改已应用文件；上游重复数字前缀（如双 `194_`）按完整文件名排序执行，不重命名；上游 migration 的数据风险登记到部署前检查（0.1.173 migration 206/220） |
| 已整合文档的 modify/delete 冲突 | 保持现行目录结构，把旧分支的有效新增规则、未解决问题和验证边界合入对应唯一文档，不恢复旧文件。上游原生文档、正式补丁及源码/migration 引用文件按既有约束处理，不为控制数量批量删除 |

### 本地横切能力的接线

| 规则 | 要求 |
| --- | --- |
| 新网关入口 | 上游新增的路由（含根级别名）必须经过 `RequestArchive -> RequestIntercept`；Responses path guard 必须位于 `RequestIntercept` 之前（`guardResponsesSubpath(requestIntercept)`），否则 Gemini wildcard 等路由会短路 guard（0.1.170、0.1.171、0.1.173、0.1.176 根级 `/x_search`、0.2.7 Seedance）。当前根级路由统一走 `rootRoute` helper |
| 新管理路由 | 上游新增 `/api/v1/admin/**` 路由时，对照 `backend/internal/service/admin_permission.go` 做显式授权决策；保持子管理员默认拒绝也是有效结论，扩权需独立产品/安全决策（0.1.161 Ops ingress/auth-cache 路由） |
| 防递归签名头 | 上游重构审核/安全链路时，确认 `X-Sub2API-Prompt-Risk-Judge` 仍传给本地 legacy moderation（0.1.160） |
| 新认证链复用管理路由 | 上游新增能进入 `/api/v1/admin/**` 的认证方式（受限密钥 0.2.19、管理 MCP 内部分发 0.2.20）必须注入 `service.WithDepartmentActor(已认证用户)`，否则本地订阅接口 fail-closed 返回部门范围拒绝；上游测试多用 nil departments，测不出来，要看生产 Wire 接线。受限判断用 `adminaccess.RestrictedFromContext`，不要用 `FromContext`（0.2.20 起全权密钥也带 Principal） |

### 范围与失败归属

| 规则 | 要求 |
| --- | --- |
| 不修上游 bug | 只解决冲突与合并必需的兼容；发现上游原生问题，用 `MERGE_HEAD` 原文件复现后登记到 review.md 第 6 节，等待上游。已做的主动修补要撤销并核对相关文件与 `MERGE_HEAD` / merge-tree 一致（0.1.159、0.1.160 起） |
| 失败归属五类 | 本轮合并引入（需修）/ 上游固有 / 第一父既有 / 环境限制 / 未确认；每类都要证据（第一父快照复跑、四方 blob 比对、错误原文），不能凭"这是上游测试"直接归责（0.2.6） |
| 第一父对照 | 用 `git archive HEAD backend` 导出第一父快照到忽略目录复跑，不建分支、不改工作区（0.2.6、0.2.19） |
| Windows 测试环境 | Go 用仓库内 `GOCACHE`/`GOMODCACHE` 与每轮全新 `GOTMPDIR`，`-p 1 -count=1`；`.test.exe` 被锁或被安全软件拒绝时换目录重跑或编译到系统临时目录执行，不关安全软件、不改业务代码（0.1.150–0.1.159）；`backup_pg_dumper` 需要 `sh.exe`，只在测试进程 PATH 加入动态定位的 Git `usr/bin`（0.2.4、0.2.6） |
| 集成覆盖 | Docker 不可用时 repository 的 `TestMain` 会跳过整包（或 `CI=true` 时 fail-closed），不计作数据库用例通过；显式 skip 逐项列出原因（0.2.6、0.2.7） |
| 时序波动用例 | 精确 `-count=3` 复跑并记录次数；"flaky"标签不能降低风险，一次转绿不算修复（0.2.6 Ollama CAS） |

## 给服务器 Codex 的可复制提示

```text
请按仓库内 docs/upstream-sync/playbook.md 合并源仓库 Wei-Shaw/sub2api 的最新 main。

要求：
1. 先只读检查 git status、remote、当前分支、最近提交和未跟踪文件。
2. fetch --all --prune 后，从当前 main 创建 feature/hy/<需求号>_merge_sub2api_<版本> 分支。
3. 固定 upstream/main 当前 SHA 后 merge 该 SHA，如有冲突，禁止无脑 ours/theirs，必须逐文件说明双方改动和融合理由。
4. 必须保留本地能力：token 分析、生图工具、用户并发方案、Redis 7+ 要求、并发错误分类、OpenAI Responses/Chat 兼容、大请求 role=tool 压缩、空响应兜底。
5. 不要提交 fixtures/、report/、runtime-report* 或生产请求样本。
6. 合并后运行后端重点测试、前端测试和前端构建；如无法运行，说明原因。
7. 输出：修改文件清单、冲突解决摘要、保留的本地能力检查结果、测试结果、是否建议上线。
8. 文档先按 docs/README.md 对照迁移：每轮仅一份 review.md，复核结论直接并入；merge-log.md 只追加。证据原文归档仓库外，确认未恢复旧目录或重复文档；wiki 只更新现行规则，不重复追加版本流水。
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
