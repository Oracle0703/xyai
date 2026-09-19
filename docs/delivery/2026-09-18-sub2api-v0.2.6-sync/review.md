# Sub2API 0.2.6 合并审核

2026-09-19 Claude 独立复审结论为 GO，未发现本轮合并引入的 finding；尚无 commit 授权。详见同目录 `claude-review-result.md`，其中登记 F1/F2 上游风险、F3 未确认项及接收结论时的两处表述修正。以下测试表保留 2026-09-18 的原始验证口径。

## 已确认的问题归属

| 分类 | 证据 | 处理 |
| --- | --- | --- |
| 第一父前端测试装配 | `SubscriptionsView.userUsageLink.spec.ts` 6 个测试缺少 Pinia/auth mock；测试和组件本轮均未改动，第一父源码快照精确复跑同样 6/6 失败 | 保留，不修改既有测试 |
| 第一父 API golden | `TestAPIContracts/GET_/api/v1/auth/me` 实际响应有 `admin_permissions:null`，旧 golden 不包含；第一父源码快照精确复跑同样失败 | 保留，不改变 DTO 或 golden |
| 上游既有 Ollama CAS 时序问题 | unit 中 `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` 失败，当前精确 `-count=3` 复跑 3 次失败；`ratelimit_service.go`、`ratelimit_service_ollama_429.go` 和对应测试在 base/第一父/上游/index 四方 blob 一致 | 只记录等待上游；历史存在通过/失败波动，继续按 flaky 跟踪 |
| Windows shell 环境 | 初轮 default/unit 的 3 个 PgDumper 测试缺少 `sh.exe`；仅在测试 PATH 加入现有 Git `usr/bin` 后，repository default/unit 全包通过；实现和测试四方 blob 一致 | 环境复跑已通过，未改代码 |
| 数据库集成覆盖缺口 | repository integration 的 `TestMain` 因 Docker 不可用直接退出 0，详细日志明确 skipping integration tests | 不把退出码 0 计作数据库用例执行通过；完整集成跳过项见下表 |

## 审核范围与边界

本轮仅解决文本冲突及合并接线，保留本地独有能力；没有额外修复上游或第一父的业务/测试问题。工作区保持未提交 merge，等待用户审核，不创建 commit、push、PR 或部署。

| 项目 | 值 |
| --- | --- |
| 日期 | 2026-09-18 |
| 工作分支 | `feature/hy/10206_merge_sub2api_206` |
| 第一父 / 本地 main | `5ec57e4fc51a9052e8812f4cb925565c984856cc`；任务开始时与远端 GitHub main 一致 |
| 固定上游 | `Wei-Shaw/sub2api main@8b69738d782ccaa7fd26511e1cca26ba8d1b58db` |
| merge base | `881f3202694c6bc932446931a30c27d9675178b9` |
| VERSION | `0.2.6` |
| merge commit | 待审核，未创建；`MERGE_HEAD` 为固定上游 SHA |
| 上游增量 | 60 commits、132 paths、`+4836/-304` |
| 三方路径集合 | 31 双方修改、495 仅本地修改、101 仅上游修改 |
| 实际文本冲突 | `.gitignore`、`backend/go.mod`、`backend/cmd/server/wire_gen.go`、`backend/internal/service/setting_service.go` |

## 本地能力保留

最终候选的 495 个仅本地修改路径中，479 个存在且 blob 与第一父一致，1 个本地删除路径 `backend/internal/server/middleware/admin_only.go` 保持删除，其余 15 个差异均为本轮文档或图谱更新。101 个仅上游修改路径与固定目标 blob 一致。24 个原有 tracked `docs/features/` 文件全部保留，23 个内容未改，合并台账仅在末尾追加；独立未跟踪文档不计入此集合。

| 能力 | 保留证据与检查重点 |
| --- | --- |
| RequestArchive / RequestIntercept | 原路由、middleware、运行态设置和设置页保留；`RequestArchive -> guardResponsesSubpath(RequestIntercept)` 顺序未变 |
| Prompt Metrics / Prompt Risk / LLM judge | provider/router/后台清理保留；`SettingService.onRiskControlUpdate` 和调用方保留，风险设置热更新专项通过 |
| Token Analysis / 组织用量 | 本地 service、repository、API、前端页面及 Wire handler 保留；上游分组日汇总优化没有替换组织用量仓储 |
| 子管理员权限 | DTO/AdminPermission、路由默认拒绝白名单、页面 guards 与订阅批量操作的完整管理员限制保留 |
| 用户并发预设 / quota flusher | provider、runner、cleanup、API 和前端控制保留 |
| OpenAI-compatible | cache usage 别名与 fill-missing、thinking/options/schema 清洗、provider preset、默认 reasoning、大请求保护和 reasoning-only failover 保留 |
| 图片生成与本地运维 | 本地图片工具页、状态持久化、Redis 7+ 与并发基础设施错误分类保留 |

真正重叠的 Gemini 模型发现、严格 Chat role 规范化、Codex ticket 及相关设置采用目标上游实现；本轮不额外维护第二套相同能力。

## 双方修改路径逐项核对

| 文件 | 处理结论 |
| --- | --- |
| `.gitignore` | 文本冲突：保留本地 features/reviews/superpowers/playbook 跟踪例外，追加上游 Antigravity 说明；运行数据/缓存忽略规则保留。 |
| `backend/cmd/server/wire.go` | 保留本地 provider 与 cleanup，接入上游 ticket harvester 停止钩子。 |
| `backend/cmd/server/wire_gen.go` | 文本冲突：从合并后的 provider source 生成；本地全部 handler/provider 与新增 SettingService 参数共存，两次生成哈希一致。 |
| `backend/go.mod` | 文本冲突：版本采用上游（含 gRPC 1.83.2）；本地直接使用的 x/sys、x/text 仍是直接依赖。 |
| `backend/internal/config/config.go` | 新增默认关闭的 ticket 配置；本地 reasoning、archive/intercept、large-request 与 Prompt Metrics 配置保留。 |
| `backend/internal/handler/dto/mappers.go` | 上游票据脱敏与状态投影共存于本地 AdminPermissions 映射。 |
| `backend/internal/handler/dto/settings.go` | 添加 ticket 设置字段；保留本地 RequestArchiveSettings DTO。 |
| `backend/internal/handler/dto/types.go` | 添加票据状态摘要；保留本地 User.AdminPermissions。 |
| `backend/internal/handler/gemini_v1beta_handler.go` | 采用上游 Gemini/Antigravity 混合模型列表；保留本地共享 path parser 和并发依赖错误分类。 |
| `backend/internal/handler/gemini_v1beta_handler_test.go` | 接入上游模型列表所需 service/repository fixture；本地 path/并发断言保留。 |
| `backend/internal/handler/wire.go` | ProvideAdminHandlers 同时保留本地 handler 集合和上游 SetCodexTicketSettings。 |
| `backend/internal/server/api_contract_test.go` | 添加上游 ticket settings golden；保留本地窄接口 mock。auth/me 既有 golden 未在本轮改写。 |
| `backend/internal/service/domain_constants.go` | 添加上游两个 ticket settings key；本地归档/拦截与风控 key 保留。 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | 采用上游严格 Chat developer role 规范化；保留本地 thinking 清理及 compatible cache usage。 |
| `backend/internal/service/openai_gateway_forward.go` | 采用上游 ticket 注入，位于原 turn-state guard 后；保留本地 text schema 清洗与 thinking 删除。 |
| `backend/internal/service/openai_gateway_messages.go` | 采用上游 Anthropic bridge ticket 注入；保留本地 compatible usage/cache details 补齐。 |
| `backend/internal/service/openai_gateway_passthrough.go` | 采用上游 ticket 注入；保留本地 official body 清理及有界 Retry-After 校验。 |
| `backend/internal/service/openai_gateway_request_body.go` | 采用上游 DeepSeek tool output media 提取；保留本地 official body 清洗 helpers。 |
| `backend/internal/service/openai_gateway_response_handling.go` | 采用上游脱离下游取消的有界 affinity 写入；保留本地 cache usage alias/fill-missing 合同。 |
| `backend/internal/service/openai_gateway_service_test.go` | 保留本地 compatible usage 回归断言，添加上游取消 context 下 affinity 写入测试及独立 probe cache。 |
| `backend/internal/service/setting_service.go` | 文本冲突：在上游新增 ticket cache/singleflight 字段中保留本地 onRiskControlUpdate，未变更回调行为。 |
| `backend/internal/service/setting_update.go` | 采用 ticket URL 校验、持久化及 cache 失效；保留本地风控热更新回调。 |
| `backend/internal/service/settings_view.go` | 添加上游 ticket 设置；本地归档运行态配置与持久化结构保留。 |
| `deploy/config.example.yaml` | 添加上游 ticket 示例/default/env 说明；本地归档/拦截、默认 reasoning 等配置保留。 |
| `frontend/src/api/admin/settings.ts` | 增加 ticket 读写类型，保留本地 auth-source fallback 和归档 API client。 |
| `frontend/src/i18n/locales/en/admin/accounts.ts` | 上游票据状态文案与本地 OpenAI-compatible 账号文案共存。 |
| `frontend/src/i18n/locales/en/admin/settings.ts` | 上游 ticket 开关/代理文案与本地请求归档文案共存。 |
| `frontend/src/i18n/locales/zh/admin/accounts.ts` | 上游票据状态文案与本地 OpenAI-compatible 账号文案共存。 |
| `frontend/src/i18n/locales/zh/admin/settings.ts` | 上游 ticket 开关/代理文案与本地请求归档文案共存。 |
| `frontend/src/types/index.ts` | 上游 Account.codex_turn_tickets 与本地 UserRole/AdminPermission 同时保留。 |
| `frontend/src/views/admin/SettingsView.vue` | 采用上游 ticket 展示/初始化/提交，保留本地 RequestArchive 加载、保存与目录配置。 |

49 个仅上游修改的测试文件与目标上游逐 blob 一致；本轮未为清除既有失败而修改测试。

## 生成物与依赖

- Wire 根据合并后的源图连续生成两次，SHA256 为 `0ED69076DCE05B2A86919A6FD68D60B7882EB0D2DF9793D69BE86178265FF683`，无漂移；未手工维护最终生成文件。
- 本次没有 SQL migration 或 Ent schema 修改，不需要重新生成 Ent；全部本地 schema/migration 保留。
- `go.mod` 采用上游新版本但保留 x/sys、x/text 的本地直接依赖属性。`go.sum` 与目标上游 blob 一致；Wire 临时添加的 `github.com/google/subcommands v1.2.0` 两个 checksum 已移除。
- 前端 package/lockfile 本轮未变，使用现有 pnpm 9 依赖；本地已移除 vite-plugin-checker 的状态保留。

## 验证与问题归属

| 验证 | 结果 |
| --- | --- |
| Wire 生成 | 两次成功，最终 SHA256 一致；无 Ent schema 变化 |
| 聚焦 Go unit | 配置、handler/admin/DTO、routes、service、repository、apicompat 相关专项通过；命令与过滤条件在本机 `verify-backend.ps1` / `focused.log` |
| `go test -p 1 -count=1 ./...` | 首轮退出 1，仅 repository 的 3 个 shell 环境失败；其余包全部通过。补齐测试 PATH 后 repository 全包复跑退出 0 |
| `go test -tags=unit -p 1 -count=1 ./...` | 首轮退出 1：上述 3 个环境失败、`/auth/me` golden 和 Ollama CAS；补齐 PATH 后 repository 全包通过，保留两类已确认的既有失败 |
| `go test -tags=integration -p 1 -count=1 -json ./...` | 退出 0；显式跳过 17 个测试，另有 repository 整包因 Docker 不可用未执行，详见下表 |
| `go build -p 1 ./...` / `go build -tags=embed -p 1 ./...` | 均退出 0；embed 使用本轮前端构建产物 |
| `go mod tidy -diff` | 退出 0，无依赖整理差异 |
| golangci-lint v2.13.0 / Go 1.27 | `run --timeout=10m --new-from-rev=HEAD ./...` 退出 0，0 issues；属于增量 lint，未宣称全库历史 lint 债务已清除 |
| `pnpm --dir frontend run lint:check` / `typecheck` | 均退出 0 |
| `pnpm --dir frontend run test:run` | 325/326 files、2422/2428 tests 通过；仅既有订阅 Pinia 装配的 6 个失败 |
| `pnpm --dir frontend run build` | 退出 0，i18n 前置 3/3、1089 modules；未绕过 build 前置检查 |
| 第一父对照 | 独立导出的 `HEAD` 源码快照中，订阅 6/6 失败及 `/auth/me` golden 差异均精确复现；未改动当前工作区源码来做对照 |
| Wiki 图谱 | 刷新为 33 nodes / 67 edges，49 wikilinks、0 unresolved；`tools\check-understand-status.cmd -AllowDirtyWiki` 为 READY |

所有 Go 命令在 `backend` 执行，复用仓库缓存，每轮使用 fresh `GOTMPDIR`。shell 修正只影响独立复跑进程 PATH，不修改系统配置。尚未提供所需服务/凭据的在线、e2e 和数据库场景不作为通过项。

| integration 未执行部分 | 数量 | 原因 |
| --- | --- | --- |
| repository 全包 | 1 个 package，未枚举为单项 skip | Docker 不可用，`TestMain` 直接退出 0；包含真实 PostgreSQL/Redis 与 migration 场景 |
| 限流 / 认证限流 | 3 个测试 | Docker/Testcontainers 不可用 |
| Prompt Audit Redis | 3 个测试 | 未设置 `PROMPT_AUDIT_TEST_REDIS_ADDR` |
| Prompt Audit PostgreSQL | 6 个测试 | 未设置 `PROMPT_AUDIT_TEST_POSTGRES_DSN` |
| Windows symlink escape | 1 个测试 | 当前进程没有创建符号链接的权限 |
| TLS 指纹 / OpenAI token API / 本地插件进程 | 各 1 个测试 | 未设置 capture URL、OpenAI API Key 或本地插件包 |
| DingTalk disabled sentinel | 1 个测试 | 上游测试自身标记为哨兵 skip |

## 文档与交付

更新六个 wiki 页面与六个受影响组件 README；纠正 0.2.5 历史测试装配/时序波动归因，并按本轮结果记录 0.2.6。合并台账按追加方式维护。原始测试日志保存在本机 `.git/codex-merge-026/`，不作为业务文件提交。


2026-09-18 完成时快照：148 个暂存路径；当时 0 unmerged、0 unstaged、0 untracked，`git diff --cached --check` 通过。本地 main 与 HEAD 均保持 `5ec57e4fc51a9052e8812f4cb925565c984856cc`，`MERGE_HEAD=8b69738d782ccaa7fd26511e1cca26ba8d1b58db`；仅本地代码的 349 个路径与第一父一致，101 个仅上游路径与目标一致。此为历史快照，后续复审任务书、结果文档及独立工作区文档的状态见各自记录。
