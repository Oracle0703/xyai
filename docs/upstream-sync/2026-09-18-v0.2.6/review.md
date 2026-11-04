# Sub2API 0.2.6 上游同步记录（固定提交）

合并日期 2026-09-18（合并提交创建于 2026-09-19）｜ 工作分支 `feature/hy/10206_merge_sub2api_206`（第一父 = 本地 `main@5ec57e4fc51a9052e8812f4cb925565c984856cc`，与 GitHub main 一致）｜ 上游 `Wei-Shaw/sub2api main@8b69738d782ccaa7fd26511e1cca26ba8d1b58db`（VERSION 0.2.6，merge base `881f3202694c`）｜ 合并提交 `ee829b7773bb3148c915a98de5d951486f6ec41b`（父 `5ec57e4fc` + `8b69738d7`）｜ 状态：在 main 第一父历史上

本文是该轮同步的唯一记录。独立复审任务书 `claude-review-request.md` 与复审结果 `claude-review-result.md`（本目录）已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-09-18 main sync (v0.2.6)`、`2026-09-19 Claude independent review supplement` 与 `2026-09-19 v0.2.6 merge completion` 三节。

## 1. 范围

- 上游 60 commits、132 paths（`+4836 / -304`）。三方路径集合：双方修改 31、仅本地 495、仅上游 101。4 个文本冲突。提交前 staged 148 个路径（`+5069 / -328`）。
- 边界：仅解决文本冲突及合并接线，保留本地独有能力；不额外修复上游或第一父的业务/测试问题。
- 结论：独立复审（Claude，2026-09-19）为 GO，未发现本轮合并引入的 finding；复审对源码范围索引（`git ls-files --stage -z -- .gitignore backend frontend deploy` 的 SHA256 `a5306abf...1a34`）前后一致，确认审查对象未漂移。用户随后授权提交。
- 后续：本轮引入的 Codex ticket 能力在 0.2.7 同步时随目标上游移除（当前源码已无 `openai_codex_ticket.go`），第 4、6 节中 ticket 相关内容仅为历史记录。

## 2. 冲突与处理

### 文本冲突

| 文件 | 处理 | 复审核对 |
| --- | --- | --- |
| `.gitignore` | 保留本地 features/reviews/superpowers/playbook 跟踪例外，追加上游 `docs/ANTIGRAVITY_ATTRIBUTION_429.md` 例外；运行数据/缓存忽略规则保留 | `git check-ignore` 确认文档可跟踪，缓存、report、fixtures 仍忽略 |
| `backend/go.mod` | 版本采用上游（含 gRPC 1.83.2）；本地直接 import 的 `x/sys`、`x/text` 保持 direct | 与上游差异仅 direct 属性；`go.sum` 与固定上游一致，Wire 临时引入的 `github.com/google/subcommands v1.2.0` 两条 checksum 已移除 |
| `backend/internal/service/setting_service.go` | 在上游新增 ticket cache/singleflight 字段中保留本地 `onRiskControlUpdate`，回调行为不变 | 沿 `service/wire.go` 注册、`setting_update.go#refreshCachedSettings` 消费及归档运行态消费者确认接线 |
| `backend/cmd/server/wire_gen.go` | 从合并后的 provider source 连续生成两次，SHA256 `0ED69076...F683` 一致；未手工维护 | cleanup 参数中 Token Analysis、并发预设、Prompt Metrics、quota flusher 与 ticket harvester 无漏接或重复 |

### 双方修改路径（31 个）

复审对 31 个路径做行级并集核对：28 个零误差，另 3 个为 Wire 单行实参串、`go.mod` direct/indirect 块移动、SettingService 的 gofmt 对齐，均逐行确认。

| 文件 | 处理 |
| --- | --- |
| `backend/cmd/server/wire.go` | 保留本地 provider 与 cleanup，接入上游 ticket harvester 停止钩子 |
| `backend/internal/config/config.go` | 新增默认关闭的 ticket 配置；本地 reasoning、archive/intercept、large-request、Prompt Metrics 配置保留 |
| `backend/internal/handler/dto/mappers.go`、`types.go`、`settings.go` | 上游票据脱敏、状态摘要与 ticket 设置字段，与本地 `User.AdminPermissions`、RequestArchiveSettings DTO 共存 |
| `backend/internal/handler/gemini_v1beta_handler.go` / `_test.go` | 采用上游 Gemini/Antigravity 混合模型列表及所需 fixture；保留本地共享 path parser 与并发依赖错误分类断言 |
| `backend/internal/handler/wire.go` | `ProvideAdminHandlers` 同时保留本地 handler 集合与上游 `SetCodexTicketSettings` |
| `backend/internal/server/api_contract_test.go` | 加上游 ticket settings golden，保留本地窄接口 mock；既有 `/auth/me` golden 未改 |
| `backend/internal/service/domain_constants.go` | 加两个 ticket settings key；本地归档/拦截与风控 key 保留 |
| `openai_gateway_chat_completions_raw.go` | 采用上游严格 Chat developer role 规范化；保留本地 thinking 清理与 compatible cache usage |
| `openai_gateway_forward.go` | 上游 ticket 注入位于原 turn-state guard 后；保留本地 text schema 清洗与 thinking 删除 |
| `openai_gateway_messages.go` | 采用 Anthropic bridge ticket 注入；保留本地 compatible usage/cache details 补齐 |
| `openai_gateway_passthrough.go` | 采用 ticket 注入；保留本地 official body 清理与有界 `Retry-After` 校验 |
| `openai_gateway_request_body.go` | 采用上游 DeepSeek tool output media 提取；保留本地 official body 清洗 helpers |
| `openai_gateway_response_handling.go` / `openai_gateway_service_test.go` | 采用上游脱离下游取消的有界 affinity 写入及其测试；保留本地 cache usage alias/fill-missing 合同与回归 |
| `setting_update.go`、`settings_view.go` | 采用 ticket URL 校验、持久化与 cache 失效；保留本地风控热更新回调与归档运行态配置 |
| `deploy/config.example.yaml` | 加 ticket 示例/default/env；本地归档/拦截、默认 reasoning 等配置保留 |
| `frontend/src/api/admin/settings.ts`、`frontend/src/types/index.ts` | 加 ticket 读写类型与 `Account.codex_turn_tickets`；保留本地 auth-source fallback、归档 API client、`UserRole`/`AdminPermission` |
| `frontend/src/i18n/locales/{en,zh}/admin/{accounts,settings}.ts` | 上游票据/ticket 文案与本地 OpenAI-compatible 账号、请求归档文案共存 |
| `frontend/src/views/admin/SettingsView.vue` | 采用上游 ticket 展示/初始化/提交（显式载荷只写 enabled/proxy URL，不提交只读 configured）；保留 RequestArchive 加载、保存与目录配置 |

本轮没有 SQL migration 或 Ent schema 变化，不需重新生成 Ent；前端 package/lockfile 未变，本地已移除 `vite-plugin-checker` 的状态保留。

## 3. 保留的本地能力与语义适配

最终候选中，495 个仅本地路径里 479 个存在且 blob 与第一父一致，本地删除的 `backend/internal/server/middleware/admin_only.go` 保持删除，其余 15 个差异是合并台账、组件 README、wiki、图谱。101 个仅上游路径（含 49 个测试文件）与固定目标逐 blob 一致。24 个 tracked `docs/features/` 文件全部保留，只有台账在末尾追加。上游本轮未改 `routes/gateway.go`、`routes/admin.go`、`router.go` 及全部 middleware。

| 能力 | 保留证据 |
| --- | --- |
| RequestArchive / RequestIntercept | 路由、middleware、运行态设置与设置页保留；`RequestArchive -> guardResponsesSubpath(RequestIntercept)` 顺序未变 |
| Prompt Metrics / Prompt Risk / LLM judge | provider/router/后台清理保留；`SettingService.onRiskControlUpdate` 及调用方保留，风险设置热更新专项通过 |
| Token Analysis / 组织用量 | service、repository、API、前端页面、Wire handler 保留；上游分组日汇总优化没有替换组织用量仓储 |
| 子管理员权限 | DTO/AdminPermission、路由默认拒绝白名单、页面 guards、订阅批量操作仅完整管理员 |
| 用户并发预设 / quota flusher | provider、runner、cleanup、API、前端控制 |
| OpenAI-compatible | cache usage 别名与 fill-missing、thinking/options/schema 清洗、provider preset、默认 reasoning、大请求保护、reasoning-only failover |
| 图片生成与本地运维 | 图片工具页、状态持久化、Redis 7+ 与并发基础设施错误分类 |

## 4. 采用的上游行为

真正重叠的 Gemini 模型发现、严格 Chat role 规范化、Codex ticket 及相关设置采用上游实现，不维护第二套相同能力。

- 默认关闭的 Codex ticket 后台生命周期与脱敏状态（0.2.7 已移除）。
- Gemini/Antigravity 混合模型列表；DeepSeek tool output media 提取；脱离下游取消的有界 affinity 写入。
- 兑换历史分页；分组用量 SQL 优化（`custom_group_usage_rollup_repo.go`）。
- 两个 scheduler 文件不是"上游零改动"：`openai_account_scheduler.go`（`+2/-2`）与 `openai_gateway_scheduling.go`（`+5/-5`）共 7 处把 `requireCompact` 传给 runtime-block 判定，index blob 与固定上游一致，没有本轮改写。

## 5. 验证

所有 Go 命令在 `backend` 执行，复用仓库缓存、每轮全新 `GOTMPDIR`。

| 验证 | 结果 |
| --- | --- |
| Wire 生成 | 两次成功，SHA256 一致 |
| 聚焦 Go unit（配置、handler/admin/DTO、routes、service、repository、apicompat） | 通过 |
| `go test -p 1 -count=1 ./...` | 首轮仅 repository 3 个 PgDumper 用例因 Windows 缺 `sh.exe` 失败；只在测试进程 PATH 加入 Git `usr/bin`（动态定位，不照搬安装路径）后 repository 全包通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 除上述环境失败外，保留 `/auth/me` golden 与 Ollama CAS 两类既有失败（见下表） |
| `go test -tags=integration -p 1 -count=1 -json ./...` | 退出 0，但显式 skip 17 个测试，另有 repository 整包因 Docker 不可用在 `TestMain` 直接退出 0——**不计作数据库用例通过** |
| `go build -p 1 ./...`、`go build -tags=embed -p 1 ./...` | 通过 |
| `go mod tidy -diff` | 退出 0 |
| golangci-lint v2.13.0 / Go 1.27 `run --timeout=10m --new-from-rev=HEAD ./...` | 0 issues（增量，不代表全库债务清除） |
| 前端 lint、typecheck | 通过 |
| `pnpm --dir frontend run test:run` | 325/326 files、2422/2428 tests；6 个失败为既有订阅 Pinia 装配 |
| `pnpm --dir frontend run build` | 通过，i18n 前置 3/3、1089 modules |
| 第一父对照 | 导出的第一父源码快照中，订阅 6/6 失败与 `/auth/me` golden 差异精确复现 |
| Wiki 图谱 | 33 nodes / 67 edges、49 wikilinks、0 unresolved；暂存候选 READY |
| 独立复审补跑 | service、handler、handler/admin、handler/dto、server/routes 聚焦 5/5 通过；`go build ./...` 通过；`^TestPgDumper` 在含 `sh.exe` 的 Git Bash 中通过；SettingsView + AccountUsageCell + SubscriptionsView.bulkActions 3 files / 92 tests 通过 |

已知失败归属：

| 失败 | 证据 | 归属与处理 |
| --- | --- | --- |
| `SubscriptionsView.userUsageLink.spec.ts` 6 个测试 | 页面调用 `useAuthStore()`，fixture 未初始化 Pinia/auth mock；页面与 spec index blob 均等于第一父，第一父精确复跑 6/6 失败 | 第一父既有，不改测试 |
| `TestAPIContracts/GET_/api/v1/auth/me` | 第一父 `dto.User.AdminPermissions` 无 `omitempty`，golden 未含 `admin_permissions` | 第一父既有，不删字段、不改 golden |
| `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` | stale callback 的 CAS 计数为 1；合并验证与复审各做精确 `-count=3`，均 3/3 失败；`ratelimit_service.go`、`ratelimit_service_ollama_429.go` 及测试四方 blob 一致 | 上游既有，等待上游。"flaky"只有 0.2.5 台账"1 次通过 / 2 次失败"的历史支撑，不能据此降级风险，也不能因某次转绿就宣称已修 |
| PgDumper 3 个测试 | Windows PATH 无 `sh.exe` | 环境限制，补 PATH 后通过，未改代码 |

integration 未执行部分：repository 全包（Docker 不可用，含真实 PostgreSQL/Redis 与 migration 场景）；限流/认证限流 3 个（Docker/Testcontainers）；Prompt Audit Redis 3 个（未设 `PROMPT_AUDIT_TEST_REDIS_ADDR`）；Prompt Audit PostgreSQL 6 个（未设 `PROMPT_AUDIT_TEST_POSTGRES_DSN`）；Windows symlink escape 1 个（无创建符号链接权限）；TLS 指纹 / OpenAI token API / 本地插件进程各 1 个（缺 capture URL、API Key 或插件包）；DingTalk disabled sentinel 1 个（上游哨兵 skip）。没有运行期服务/真实上游验收，因此不能写成"完整集成全绿"或"已验证可直接上线"。

## 6. 遗留问题

| ID / 等级 / 归属 | 说明 | 当前状态 |
| --- | --- | --- |
| F1 / P2 / 上游 | `openai_codex_ticket.go#applyOpenAICodexTicket` 在开启、目标账号/模型、无有效票且 fail_closed 时返回 `ErrOpenAICodexTicketUnavailable`；普通 Responses 在 `openai_gateway_forward.go` 构造请求阶段直接返回该 error，未包装为 `UpstreamFailoverError`，缺换号路径。默认关闭不触发 | ticket 能力已在 0.2.7 随上游移除，不再适用 |
| F2 / P3 / 上游 | `openai_gateway_service.go` 无条件启动 harvester，每周期后等待默认 6 秒；关闭时跳过账号查询，但开关缓存 TTL 5 秒，过期会经 `settingRepo.GetValue` 读 settings，不能说"关闭时完全无 DB 访问"；开启时每周期 `ListByPlatform(PlatformOpenAI)` 全量取号 | 同上，已随 0.2.7 移除 |
| F3 / 未确认 | `custom_group_usage_rollup_repo.go#readGroupUsageRollupSnapshot` 把 retained_from 的日期换算从 SQL `AT TIME ZONE` 移到 Go `GroupUsageDate()`；closed_before 的 ISO 文本比较复审判断等价 | 因 repository integration 整包跳过，真实 PostgreSQL/服务时区边界未验证；材料中无后续结论，不能称为缺陷或已通过 |

其他：Ollama CAS 上游失败、`/auth/me` golden 与订阅 Pinia 装配的第一父失败、integration 覆盖缺口，均按上表保留。`/auth/me` golden 后来随部门功能（`a9c53a9f6`，经 `273b76c35` 于 2026-09-28 进入 main）已包含 `admin_permissions: null`。
