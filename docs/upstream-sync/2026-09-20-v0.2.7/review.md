# Sub2API 0.2.7 合并审核

本轮按“仅解决冲突”执行：合入固定上游、适配本地接口冲突、同步文档并运行测试。用户已明确选择跟随目标移除 Codex ticket。当前停在 commit 前，不创建提交、不推送、不部署。

已完成合并冲突处理和测试，当前可供提交前审核；完整测试并非全绿。原有 auth/me golden、Ollama CAS、前端 Pinia 失败及 Docker 集成覆盖缺口保持原样，详见验证表。

## 合并边界

| 项目 | 结果 |
| --- | --- |
| 日期 | 2026-09-20 |
| 工作分支 | `feature/hy/10207_merge_sub2api_207` |
| 本地 main / 第一父 | `de5a3e383cd8eb197c1a83f12a71fb04d9e4e049` |
| 固定上游 | `Wei-Shaw/sub2api main@fbb9006adef852c46f0c7f18b0a8a740722cfac7` |
| 共同祖先 | `efe9aab1e4ec89a42ba45e8dac20e882c5409a6a` |
| VERSION | `0.2.7` |
| 上游相对共同祖先 | 34 commits、96 paths、+7652/-615 |
| 初始三方集合 | 30 双方修改、540 仅本地修改、66 仅上游修改（最终均与目标 blob 一致） |
| 文本冲突 | 普通 merge 的 VERSION；移除 ticket 三方补丁中的 setting_service.go |
| 语义冲突 | Prompt Risk 调用的旧抽取函数被上游改为 collector 方法 |
| merge commit | 未创建；`MERGE_HEAD` 固定为用户指定 SHA |

## 上游历史重写与用户选择

抓取上游时 `8b69738d7...fbb9006ad main -> upstream/main (forced update)`。旧 0.2.6 的 8 个提交（票据 PR #7315 及 VERSION 同步）不再位于目标历史；共同祖先的 VERSION 是 0.2.5，直接 merge 会把 ticket 当作本地增量保留下来。经用户选择，本轮用旧上游 `8b69738d...` → 共同祖先的精确反向差异移除 ticket，再保留目标 0.2.7 的新增逻辑。

移除范围包括 harvester、请求注入、调度 gate、设置缓存/读写/DTO、账号票据状态、前端展示和专属测试。普通 turn-state echo guard、response affinity、Codex 指纹机制继续保留。没有操作数据库、迁移旧 settings/extra 或修改线上配置。

`SettingService` 文本冲突保留本地 `onRiskControlUpdate func(bool)`，删除四个 ticket cache/singleflight 字段。Wire 从源图生成以去掉 ticket 参数/cleanup，并保留本地功能 provider。

## 本地功能保留与重叠判断

原有 25 个 tracked `docs/features/` 文件全部保留，包括本地 main 最新的部门用量/额度重置设计；部门设计不是本轮实现目标。台账仅追加，其他 feature 文档不改写。相对上一版纯上游的本地代码增量涉及 369 个 backend/frontend/deploy 路径；归一化 +/- 行内容比对除 Wire ticket 参数、本地 Prompt Risk 适配和两个组件 README 外均一致。

| 本地能力 | 保留依据 |
| --- | --- |
| RequestArchive / RequestIntercept | 路由、middleware、provider、运行态设置与设置页保留；新 Seedance 路由也走原 rootRoute；Responses path guard 先于拦截 |
| Prompt Risk / LLM judge | 独立配置、前置阶段、newest/full、掩码/回环保护、热更新和 action 筛选保留；抽取 helper 只适配新的 collector 接口 |
| Prompt Metrics / Token Analysis | 原 service/repository/API/router/Wire/后台生命周期保留 |
| 组织用量 / 并发预设 | 原独立仓储、管理 API、页面、定时 runner 和 quota flusher 保留 |
| 子管理员 | UserRole/AdminPermission、后端默认拒绝白名单、前端 guards 保留 |
| OpenAI-compatible | usage/cache alias 与 fill-missing、thinking/options/schema 清洗、preset、默认 reasoning、大请求保护和空响应 failover 保留 |
| 图片工具与本地运维 | 工具页/Pinia 状态持久化、Redis 7+ 与依赖错误分类保留 |

本地 Prompt Risk 与新增 TypeSafe 内容审计具有不同输入范围/动作/配置，不按重复能力删除。公开模型名还原、DeepSeek reasoning 占位、关键词 reminder 检查、Seedance、TypeSafe 与插件 HostService 采用目标上游实现。

## 冲突处理与上游生成问题

| 项目 | 证据 / 处理 |
| --- | --- |
| VERSION | 选择 0.2.7，不恢复被上游撤下的 0.2.6 边界 |
| SettingService | 精确删除 ticket 字段，保留 onRiskControlUpdate |
| Prompt Risk 接口 | 聚焦测试曾报 addModerationText/collectContentValue/collectAnthropicUserContentValue undefined；上游已改为 moderationTextCollector 方法。在本地 prompt_risk_input.go 添加三个薄转接，filterReminders=true 保持第一父行为；上游关键词不过滤路径不改 |
| Wire 上游固有问题 | 目标 wire_gen.go 包含 pluginManager.SetAccountDirectory(openAIGatewayService)，源 wire.go/provider 未声明等价接线。go generate 会删掉这行。本轮保留目标生成物接线，不补改上游源图；后续重生成须人工核对 |

Wire 工具新增的两个 google/subcommands 校验和已恢复，go.sum 沿本地 main；本轮没有 Ent schema 变化，不生成 Ent。新增 SQL 为上游原始 `238b_content_moderation_engine_meta.sql`，不修改历史迁移。

## 双方修改路径逐项核对

| 文件 | 合并结论 |
| --- | --- |
| `backend/cmd/server/VERSION` | 文本冲突，采用目标 0.2.7。 |
| `backend/cmd/server/wire_gen.go` | 重生成本地 provider/cleanup 并移除 ticket；保留目标上游手工存在的 SetAccountDirectory 接线，见生成边界。 |
| `backend/internal/handler/admin/content_moderation_handler.go` | 接入 TypeSafe engine/profile/test 参数，保留本地 Prompt Risk 三个 handler。 |
| `backend/internal/handler/endpoint.go` | 加入 Seedance 归一化，保留本地 Responses input_tokens/subpath 合同。 |
| `backend/internal/repository/content_moderation_repo.go` | 新增 engine_meta 插入/读取，保留 Prompt Risk block/observe 筛选和封禁计数排除。 |
| `backend/internal/repository/content_moderation_repo_test.go` | 保留本地 action 筛选断言，合入上游 engine_meta 读写测试。 |
| `backend/internal/repository/http_upstream.go` | 采用上游 HTTP/2 keepalive 容错；按授权移除 ticket harvest transport。 |
| `backend/internal/repository/wire.go` | 新增 PluginKVStore，同时保留组织用量、Token Analysis、并发预设等 provider。 |
| `backend/internal/server/routes/admin.go` | 增加插件只读 status 路由，本地管理功能与权限中间件保留。 |
| `backend/internal/server/routes/gateway.go` | Seedance 四组别名走既有 rootRoute，共享本地归档/拦截；Responses guard 顺序保留。 |
| `backend/internal/service/account_usage_service.go` | 采用上游查询不清除 refresh error，保留本地配额/usage 逻辑。 |
| `backend/internal/service/account_usage_service_batch_test.go` | 保留本地聚合断言，合入上游错误状态保留用例。 |
| `backend/internal/service/content_moderation.go` | 采用上游独立引擎与关键词输入逻辑；保留 Prompt Risk 前置阶段、LLM judge、runtime hash、日志/action/热更新。 |
| `backend/internal/service/openai_gateway_cc_pipeline.go` | 合入 DeepSeek reasoning 占位入口，本地 Responses→Chat 过滤/兼容保留。 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | 增加响应公开模型名还原，本地 thinking 清理、compatible usage/cache normalization 保留。 |
| `backend/internal/service/openai_gateway_chat_completions_raw_test.go` | 合入公开别名断言，本地兼容回归保留。 |
| `backend/internal/service/openai_gateway_passthrough.go` | 采用上游 model 重写，移除 ticket 注入；本地 body 清洗/Retry-After 校验保留。 |
| `backend/internal/service/openai_gateway_request_body.go` | 采用上游 model 别名还原，本地 official body 清洗 helpers 保留。 |
| `backend/internal/service/openai_gateway_response_handling.go` | 采用上游顶层/nested model 重写，本地 cache alias/fill-missing 与 reasoning-only failover 保留。 |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 采用上游 DeepSeek 缺失 reasoning_content 占位，本地 options adapter 继续存在。 |
| `backend/internal/service/openai_gateway_service_test.go` | 沿用上游公开模型名新断言，本地 compatible cache 测试保留。 |
| `backend/internal/service/setting_gateway_runtime.go` | 采用上游非法 UA 保留独立 client version；按授权移除 ticket cache methods。 |
| `frontend/src/api/admin/riskControl.ts` | 新增引擎/profile/metadata 参数，保留本地 Prompt Risk client 和类型。 |
| `frontend/src/components/account/CreateAccountModal.vue` | 合入 Seedance opt-in，保留 OpenAI-compatible preset 与既有创建合同。 |
| `frontend/src/components/account/EditAccountModal.vue` | 合入 Seedance opt-in，移除 ticket 展示，保留本地 preset 和编辑合同。 |
| `frontend/src/i18n/locales/en/admin/channels.ts` | 合入上游标签和文案，本地渠道功能文案保留。 |
| `frontend/src/i18n/locales/zh/admin/channels.ts` | 合入上游标签和文案，本地渠道功能文案保留。 |
| `frontend/src/i18n/locales/zh/common.ts` | 采用上游“内容审计”文案，本地功能导航 key 保留。 |
| `frontend/src/types/index.ts` | 合入 Seedance capability、移除 ticket DTO；保留 UserRole/sub_admin/AdminPermission。 |
| `frontend/src/views/admin/RiskControlView.vue` | 合入 TypeSafe 独立配置和草稿测试 key，同步保留 PromptRiskPanel 独立保存/结果筛选。 |

## 验证

| 验证 | 命令 / 结果 |
| --- | --- |
| 后端专项 | unit tag，cmd/server、service、repository、handler/admin、routes、apicompat、typesafe、pluginapi 按 Wire/Seedance/Plugin/Moderation/PromptRisk/本地能力等过滤；通过。TypeSafe client/pluginapi 无匹配用例，已由完整测试覆盖 |
| 后端 default | `go test -json -p 1 -count=1 ./...`：退出 0，54 个测试包通过；14 个显式测试 skip，58 个包无测试。12302 个 pass 事件（包含子测试） |
| 后端 unit | `go test -json -tags=unit -p 1 -count=1 ./...`：58 个测试包通过，2 个包失败（server、service）；18 个显式 skip，52 个包无测试。两项实际失败见下表，父 TestAPIContracts 同时标记 fail，不重复计作第三项 |
| 后端 integration | `go test -json -tags=integration -p 1 -count=1 ./...`，`CI=true`：53 个测试包通过，repository 因 Docker 不可用在 TestMain fail-closed；18 个显式 skip，58 个包无测试。不能写成全量集成通过 |
| 后端构建 | `go build ./cmd/server`、`go build -tags=embed ./cmd/server` 通过；输出到忽略目录 |
| 依赖 / lint | `go mod tidy -diff` 退出 0；golangci-lint v2.13.0 / Go 1.27 `run --new-from-rev=HEAD --timeout=30m ./...`：0 issues |
| 前端 lint / typecheck | `pnpm run lint:check`、`pnpm run typecheck` 通过 |
| 前端完整测试 | `pnpm exec vitest run`：325/326 files、2427/2433 tests；6 个第一父 Pinia 装配失败保留 |
| 前端生产构建 | `pnpm run build` 通过（i18n 3/3，1089 modules）；仅已有的大 chunk 提示 |
| Wiki 图谱 | refresh + `check-understand-status.cmd -AllowDirtyWiki`：34 nodes、72 edges、54 wikilinks、0 unresolved、READY |

| 未通过 / 覆盖边界 | 本轮取证与处理 |
| --- | --- |
| 第一父 auth/me golden | 当前 unit 与独立导出的 `main@de5a3e383` 精确单测均失败；实际 DTO 带 `admin_permissions:null`，旧 golden 缺失。未修改 DTO/golden |
| 上游既有 Ollama CAS | 当前精确 `-count=3` 为 3 失败；第一父独立快照 `-count=3` 为 1 通过 / 2 失败，具有时序波动。`ratelimit_service_ollama_429.go` 与其测试在 base/第一父/目标/index 四方 blob 一致。只记录不修复 |
| 第一父前端 Pinia | `SubscriptionsView.userUsageLink.spec.ts` 6 个测试报 no active Pinia，第一父源码快照同文件精确复跑同样 6 失败；测试和页面本轮未改 |
| Docker 不可用 | repository 集成 TestMain 明确打印 `docker is not available (CI=true); failing integration tests`，该包未执行数据库用例。不通过移除 CI 标志把失败伪装为成功 |
| 18 个 integration 显式 skip | DingTalk sentinel 1、Windows symlink 权限 1、Docker 依赖 middleware/routes 3、TLS capture URL 1、Prompt Audit Redis 3 / PostgreSQL 6、TypeSafe live 凭据 1、OpenAI API key 1、本地插件 fixture 1。未连接真实外部凭据服务或启动生产服务 |
| 上游 Wire 源图漂移 | 已确认目标源图缺少 SetAccountDirectory；最终保留上游生成物行为，未来重新生成仍有丢接线风险。本轮未修复该上游问题 |

测试已全部运行结束；合并引入的 Prompt Risk 编译冲突已解决，剩余红项属于复现确认的第一父/上游或环境边界。此结论支持提交前人工审核，不代表完整集成或生产验收通过。


## 本机复查

原始日志保存在忽略目录 `backend/.gocache/merge-207/`，不提交测试运行数据。`verify.ps1` 为本轮本机验证入口，源码与构建产物分离；第一父失败复现快照位于同目录 `first-parent/`。
