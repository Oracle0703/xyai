# 0.2.22 上游同步审核

日期：2026-11-17；分支：`feature/hy/10235_merge_sub2api_222`；状态：合并待用户审核；#8680 兼容性回退已修复，并于 2026-11-18 通过独立复核，原 P1 关闭（第 2、5 节）；未 commit / push / 合回 main / 部署。

## 1 范围

| 项目 | 固定值 |
| --- | --- |
| 第一父、本地 main | `be6d173efcda73bcbd8313d7bb7d7627702e35e6`（= github/main，已含 0.2.21 及 PR #8759、#8680、#8829、#8793 cherry-pick） |
| 指定上游 | `Wei-Shaw/sub2api main@15871f904056f548dd874a1d069ffc6cdf777b78`，VERSION 0.2.22；fetch 时 upstream/main 即该提交 |
| 共同祖先 | `6dd5a045b62e27e5b5b139adef91a171097f639b`（0.2.21） |
| 增量 | 187 commits / 340 上游路径；61 个双方修改路径、279 个仅上游路径、775 个仅本地路径 |
| 操作边界 | 从干净的本地 main 建分支，`git merge --no-commit --no-ff <固定 SHA>` |
| 修补边界 | 只解决冲突与合并必需的转接；功能重叠采用上游；不修上游 bug，不扩展本地功能 |

## 2 冲突与处理

### 冲突与语义修正（10 个文本冲突文件 + 1 个验证发现的语义冲突）

| 文件 | 双方改动 | 处理 |
| --- | --- | --- |
| `handler/openai_page_context.go`、`openai_page_context_test.go` | 本地 cherry-pick 的 #8680 改为原样转发、只在校验副本剔除；上游 #8769 继续改写为 user message 并扩展 `browser_info`/`goal_objective`/`untrusted_context` | 两文件 blob 等于目标（上游识别的生产者按上游改写）；上游未覆盖的生产者由本地新文件 `handler/openai_standalone_context.go` 保留原样转发，见下节 |
| `handler/openai_gateway_handler.go` | #8680 删除 automation/delegation 改写；上游保留并扩展 | 以 #8680 之前的本地版本（`dcb40e360`）与目标重新三方合并，保留上游三个 normalizer；结果相对目标只多本地 `service.IsSafeRetryAfter` 收敛，以及 `validateFunctionCallOutputRequest` 里一行 `codexStandaloneContextValidationBody` 调用 |
| `service/openai_responses_tool_schema.go` | 本地新增 `text.format` schema `format` 清洗；上游 #8772 把 lookaround 检测改为 Python `re` 规则（函数改名） | 两者都保留：本地函数块在前，上游新函数名与实现在后；结果相对目标的差异恰好等于本地增量 |
| `repository/dashboard_aggregation_repo.go` | 本地在分区删除事务里给组织用量日汇总整月标脏；上游 #8693 重写分区删除（`dropUsageLogsPartition`，`lock_timeout` 3s、按保留下界失效） | 采用上游流程；`dropUsageLogsPartition` 增加 `monthStart` 参数，在 `SET LOCAL lock_timeout` 之后、`drop(tx)` 之前调用本地 `markOrganizationUsageRollupRangeDirty`，与第一父“同事务、DROP 前标脏”一致 |
| `repository/dashboard_aggregation_group_usage_test.go` | 本地给旧分区删除用例加标脏期望；上游重写为新流程用例 | 采用上游用例，4 处分区删除事务补本地标脏期望（`expectOrganizationUsageRollupMonthDirty` helper），其余断言不改 |
| `migrations/group_usage_rollup_migration_test.go` | 本地新增 migration 246 用例；上游新增 251 用例 | 两个测试函数并存 |
| `frontend/.../CreateAccountModal.vue`、`EditAccountModal.vue` | 本地 OpenAI 请求时区字段与 Spark 影子账号清理；上游 passthrough 按模型筛选开关 | 状态、表单回填、重置、提交写回全部并存；Edit 提交处两个独立 `if` 顺序排列；结果相对目标差异恰好等于本地增量 |
| `repository/group_usage_rollup_nonblocking_integration_test.go`（自动合并后的语义冲突，PG 验证发现） | 上游新用例在只含 222/223/251 迁移的隔离 schema 里删除分区；本地标脏调用的 `mark_user_usage_rollup_dirty_day`（migration 246）在该 schema 中不存在，`CleanupUsageLogs` 返回“函数不存在” | 生产库有 246，行为不受影响；只修测试夹具：新增本地 `organization_usage_rollup_fixture_integration_test.go#installOrganizationUsageRollupDirtyMarks`，在该用例建 schema 后安装同定义的表与函数（一行调用），断言不改 |
| `cmd/server/wire_gen.go` | 上游新增 `usageBillingLaneCombiner`；本地 Token Analysis、组织汇总等 cleanup 依赖 | 不选边，重新生成；唯一差异为 `provideCleanup` 参数并集；`ProvidePluginManager` 本地 provider 仍在 |

### 本地 #8680 与上游 #8769：重叠部分采用上游，其余保留本地

2026-11-15 本地 cherry-pick 的上游 PR #8680（Codex 独立上下文原样转发，只在校验副本剔除）于 2026-11-16 被上游关闭未合入，上游改合 #8769（改写为 user message）。两者只部分重叠：

| 生产者（无 `call_id` 的 `function_call_output`） | 归属 | 合并结果 |
| --- | --- | --- |
| `agent` page snapshot、`goal_objective`；`codex_app`/`codex_tui` 的 automation/delegation bootstrap | 双方都支持 | 采用上游：改写为 user message（与 0.2.21 及更早一致） |
| `agent/browser_info`、`agent/untrusted_context` | 上游新增 | 采用上游：改写为 user message |
| `agent/browser_tabs`、`agent/proposed_plan`、`agent/untrusted_content`、`guardian/guardian_housekeeper_input`、`guardian/guardian_channel_selection_input`、`guardian/guardian_original_evidence`、`cloud_threads/send_message` | 仅本地（#8680） | 保留本地：通过校验并原样转发 |
| 上述仅本地生产者与 automation/delegation bootstrap 出现在同一请求 | 上游 bootstrap normalizer 遇到不识别的无 `call_id` 输出会整体放弃改写 | 保留本地：整条请求放行并原样转发（第一父行为） |

实现：

- 上游文件保持目标 blob：`handler/openai_page_context.go`、`openai_page_context_test.go`、`openai_automation_bootstrap_test.go`、`openai_delegation_bootstrap_test.go`；Responses 入口先执行上游三个 normalizer。
- 新增本地文件 `handler/openai_standalone_context.go`：取自第一父的 `codexStandaloneContextValidationBody`、`isCodexStandaloneContextItem`、`validCodexCloudDelegationEnvelope`、`validCodexBrowserContext`。白名单为上表 7 个仅本地生产者，加 automation/delegation（只为并存请求兜底）；page snapshot 与 `goal_objective` 由上游逐项改写、不会残留，不再列入，上游测试文件因此无需改动。`browser_tabs` 判断放在本地函数里，不改上游 `validCodexPageContext`。它只作用于 normalizer 之后仍是 `function_call_output` 的条目，不改变上游已改写的结果。
- `service/openai_codex_transform.go#filterCodexInputWithOptions`、`service/openai_responses_namespace.go#stripOpenAIResponsesInputNamespaces`：保留 #8680 对“带 `name`、无 `call_id` 键的 `function_call_output`”的原样保留（不由 `id` 合成 `call_id`，不删 `id`/`namespace`）；两文件等于自动三方合并结果。
- 测试：`handler/openai_standalone_context_test.go` 保留第一父用例（去掉已由上游改写的 `goal_objective`），并入原在 `openai_page_context_test.go` 的 `browser_tabs` 信封用例，新增按入口顺序（normalizer → 校验）的 `TestCodexStandaloneContextResponsesEntry`（11 个生产者、并存请求、未知名称仍拒绝）；`service/openai_desktop_context_forward_test.go` 等于第一父。
- 相对第一父的有意变化只有一项：page snapshot、`goal_objective`、automation/delegation 在 HTTP Responses 上由原样转发改回上游的 user message 改写。RequestArchive 在路由层保存改写前的原始请求体，Token Analysis 不受影响。

首版合并曾把 #8680 当作完全重叠整体撤除。独立复核用 11 个第一父已支持的场景对照，确认其中 7 个仅本地生产者回退为整条请求 HTTP 400（`function_call_output requires call_id on HTTP requests`），纯上游同样拒绝；上述处理即为该问题的修复，验证见第 5 节。

2026-11-18 独立复核确认该修复同时恢复入口校验与转发字段保留，原 7 项失败全部恢复，上游原有改写及精确白名单边界保留；原 P1 关闭。本次修复范围内未发现新的阻断问题，不改变第 6 节的其他验证限制。

### 其他已 cherry-pick 的上游 PR

| PR | 上游状态 | 结果 |
| --- | --- | --- |
| #8759 schema `items` | 已合入 0.2.22 | 双方改动相同，`schema_cleaner.go`、`antigravity_gateway_compat_test.go` 等于目标 |
| #8829 Dual-use 句、#8793 Pi 身份 + 本地通用客户端身份改写 | 仍 open | `request_transformer.go` 等本地改动完整保留，相对目标差异仅为这些本地增量 |
| #8836 裸 429 冷却 | 仍 open，本地未合入 | 不涉及 |

### 自动合并复核

61 个双方修改路径逐个做双向增删行集合比对（结果相对目标 = 本地增量；结果相对第一父 = 上游增量）：53 个两侧完全保留（含恢复 #8680 service 增量后等于自动三方合并的 `openai_codex_transform.go`、`openai_responses_namespace.go`）；其余 8 个均为上文有意处理的文件（#8680/#8769 的 3 个 handler 文件、#8759 双方相同 2 个、分区删除 2 个、`wire_gen.go`）。主要组合点：

| 范围 | 组合结论 |
| --- | --- |
| Ent 生成文件、`ent/schema/group.go` | 上游 `force_openai_ultrafast` 与本地实体并存；Ent/Wire 各生成两次无漂移 |
| config / `deploy/config.example.yaml` | 上游 `concurrency.ping_interval=0` 禁用 ping、DeepSeek 显式价说明 + 本地配置项并存 |
| OpenAI 网关（forward/messages/chat/passthrough/request_body/response_handling/usage/WS） | 上游零拷贝读取、keepalive 后错误、首输出前 failover、spool 回退、Fork prompt cache key + 本地默认 effort、compatible cache 补缺、时区改写、Token Analysis 接线、空响应兜底、`text.format` 清洗 |
| DTO / 前端类型 / 分组页 | 上游 `force_openai_ultrafast`（仅管理 DTO）+ 本地部门/权限/分组组织归属 |
| 用户用量页、账号用量、i18n | 上游额度刷新失败提示、通配白名单拒绝 + 本地账户列与只读行为 |

## 3 保留的本地能力

| 本地功能 | 结果 |
| --- | --- |
| Token Analysis / RequestArchive / RequestIntercept / Prompt Metrics/Risk | 源码、迁移、路由（`guardResponsesSubpath(requestIntercept)`）、Wire 与前端入口保留；上游未新增路由 |
| 组织/部门/组织负责人/分组组织归属 | 保留；分区删除整月标脏接入上游新流程 |
| 子管理员与账号操作员 | 上游未新增 `/api/v1/admin/**` 路由，权限白名单不变 |
| 订阅自助日重置/批量重置、GPT 额度、并发预设 | 仅本地路径，blob 等于第一父 |
| OpenAI 本地适配 | 默认 effort、compatible cache、时区改写、大请求保护、并发错误分类、空响应兜底、`text.format` 清洗、插件账号目录 provider 保留 |
| Codex 独立上下文的额外白名单 | 保留：上游未覆盖的 7 个生产者及并存请求继续原样转发（第 2 节）；上游已覆盖的生产者改用上游改写 |
| Antigravity 本地改写 | #8829/#8793 与通用客户端身份改写保留 |
| 生图与用户用量账户列 | 保留 |
| `docs/features/` | 零修改、零删除 |

775 个仅本地路径中，763 个 blob 等于第一父；例外为 #8680 的 3 个测试文件（automation/delegation 两个改回目标，`openai_standalone_context_test.go` 按第 2 节调整）和本轮更新的 9 个文档/wiki/图谱文件。279 个仅上游路径中，除 `group_usage_rollup_nonblocking_integration_test.go`（第 2 节夹具一行）外 blob 全部等于目标；新增本地文件 2 个（集成测试夹具、`handler/openai_standalone_context.go`）。

## 4 采用的上游行为

| 上游增量 | 行为与边界 |
| --- | --- |
| 分组强制 Ultrafast（#8812） | `groups.force_openai_ultrafast`，对支持的模型优先于强制 Fast；仅管理 DTO |
| passthrough 按模型筛选（#8748） | 账号 extra `openai_passthrough_model_filter`，默认关闭，开启后才按白名单/映射来源名筛选 |
| 分组日汇总非阻塞（#8693） | 新增 `251_group_usage_rollup_nonblocking_build.sql`（`lock_timeout` 5s）；保留下界 `expired_before`、重建登记/脏日；分区删除锁等待 3s 超时则跳过并按行删除 |
| Codex 上下文（#8769、#8754、#8664） | 上游识别的 HTTP Responses 独立上下文改写为 user message（其余生产者见第 2 节）；additional_tools ID 保留/规范化；Fork 沿用父 prompt cache key |
| OpenAI 流与容错（#8724、#8808、#8811、#8813、#8814、#8826、#8800） | keepalive 后的 SSE 错误、spool 失败回退内存、Messages bridge 首内容前 failover、首输出前转发 keepalive、上游 TCP keepalive 约 30s 发现死连接、空流释放粘性账号 |
| 模型/计费 | Gemini 4 Carbon/Hydrogen、Vertex 多区域、DeepSeek 显式价不叠峰谷、Qwen3 Embedding 兜底价、按用户通道合并扣费、RPM/倍率同步事务化 |
| 其他 | 工具 schema Python `re` 规则、通配白名单拒绝、定价启动后台校验、设置注入 HTML 不缓存、`concurrency.ping_interval=0`、Go 1.27.4 |

## 5 验证

| 验证 | 实际结果 |
| --- | --- |
| 前端 frozen install / lint:check / typecheck | 全部通过，package.json/lockfile 零漂移 |
| 完整 Vitest | 首轮被外部进程清理 `%TEMP%` 打断（19 个套件 ENOENT、0 用例执行，环境问题）；改用仓库外独立 TEMP 重跑：440 files / 3657 tests 全部通过 |
| 前端 build | 通过（含 check:i18n），1220 modules；沿用已有 chunk/动态导入提示 |
| Go default 全量：`go test -json -p 1 -count=1 ./...` | 退出 1；56 包通过，7998 顶层 / 16871 含子用例 PASS；唯一失败为上游 TTFT 时序用例（semantic 498ms < 500ms），见第 6 节；14 显式 skip |
| Go unit 全量（CI 门禁 `-tags=unit`） | 退出 0；63 包、13681 顶层 / 29424 含子用例 PASS，0 FAIL；41 显式 skip |
| Go integration 全量 | 退出 1；56 包通过，7496 顶层 / 16029 含子用例 PASS；唯一失败为同一 TTFT 用例（visible）；18 显式 skip；repository 包因无 Docker 整包跳过，不计作数据库通过 |
| TTFT 归属 | 测试文件 blob 等于 MERGE_HEAD；合并结果精确 `-count=3`：1 PASS / 2 FAIL（1.098s < 1.1s）；`git archive` 纯上游快照同条件：2 PASS / 1 FAIL（499ms < 500ms）。上游固有，未改断言或实现 |
| 隔离 PG 18.3 升级 | 新库先跑第一父 332 条迁移，再跑合并结果成 334 条；集合差恰好为两个新 `251_group_*`，既有 checksum 不变；`groups.force_openai_ultrafast`、state 三列类型/默认值核对一致 |
| 隔离 PG repository 包 | postgres-only harness 两处环境限制：Redis 为 nil 时 `TestSchedulerCacheSnapshot…` panic 会中止整包；上游隔离 schema 用例依赖 Docker 分支才设置的 `integrationDSN`。按 Redis 文件跳过后：合并结果 845 顶层 PASS，第一父快照 823 顶层 PASS，两侧共同失败为 `TestAccountRepoSuite` 的 Redis 子用例与 `TestServerTimingConnector…` 时序断言；合并结果另有 6 个上游 schema 用例因缺 DSN 失败 |
| 仓库外副本补 DSN 后 | 只在仓库外副本把 `integrationDSN = dsn` 补进 postgres-only 分支：先发现第 2 节的分区删除夹具冲突，修复后 rollup/aggregation/organization/partition 52 个顶层用例全部通过；整包 852 顶层 / 891 子用例 PASS，唯一失败为 Redis 子用例；6 个 skip 均为性能/EXPLAIN 开关 |
| Ent / Wire | 各连续生成两次，第二次无漂移；Wire 临时写入的 `google/subcommands` go.sum 两行已还原；`ProvidePluginManager` 仍含 `SetAccountDirectory` |
| Go normal / embed build | 均通过；embed 使用本轮前端 dist，`-version` 返回 0.2.22（embed 首次构建遇 Windows `compile.exe` Access is denied，重试通过，环境问题） |
| golangci-lint 2.14.0（go1.27.4 重建） | 增量、Windows 全量、GOOS=linux 全量均 0 issues；repository 加 `--build-tags=integration` 增量 0 issues。旧二进制为 go1.27.0 构建，未使用 |
| go mod tidy -diff | 退出 1，仅提议删除 go-runewidth v0.0.15、tablewriter v0.0.5、uniseg v0.2.0、cobra v1.7.0 各两行，与 0.2.21 记录相同；go.sum 双方均未改动，保持原样 |
| Git / 文档硬门 | 未合并索引条目 0、冲突标记 0、`git diff --cached --check` 通过；279 仅上游路径除夹具调整的 1 个测试外 blob 等于目标；775 仅本地路径除 #8680 的 3 个测试及 9 个文档/wiki/图谱文件外等于第一父；`docs/features/` 零变更，旧文档目录未恢复；台账只追加 |
| Wiki 图谱 | #8680 兼容修复后重新生成：35 nodes / 76 edges，62 wikilinks / 0 unresolved |
| 独立复核：冲突相关回归 | 后端 5 包 `-tags=unit` 专项 174 个顶层 / 663 个含子用例 PASS，0 FAIL/0 SKIP；前端 Create/Edit/分组配置 5 文件 149/149 PASS。三方逐 blob/双向增删行核对与前述处理一致；这些已有用例未覆盖被删除的独立上下文白名单 |
| 独立复核：被撤除能力对照 | 仓库外 Go overlay 增加 11 个历史支持场景，按各版本 Responses 入口顺序执行 normalization -> validation；第一父 `git archive` 快照 11/11 放行，首版合并与纯上游快照均 4/11 放行、7 项 HTTP 400。只测本地前置校验，不发送真实上游请求。原始日志、用例、三方差异与汇总见仓库外 `E:/tmp/xyai-merge-222-codex-review-20261117/` |
| #8680 兼容修复后复验 | 同一组 11 个场景的 overlay 在修复后的工作区 11/11 放行；handler 的 `TestCodexSourceContext*`、`TestCodexBrowserTabsEnvelope`、`TestCodexStandaloneContextResponsesEntry` 与 service 的 `TestOpenAIForwardPreservesStandaloneDesktopContext`（OAuth/API Key × passthrough 开关 4 个子用例）通过，上游 page/automation/delegation 用例文件未改动且通过。Go unit 全量重跑退出 1：62 包通过，13685 顶层 / 29449 含子用例 PASS，唯一失败是 `TestNativeImagesRoutesProxyTimeout` 的 `/v1/images/generations/native=false`（期望 200 实际 504；1.3s 代理首包超时的真实时序用例，当时三个全量 lint 并行运行；测试文件 blob 等于第一父和目标，与本次改动无关），隔离 `-count=5` 全部通过，handler 四个包 unit 重跑退出 0（1650 顶层 / 3368 含子用例 PASS）。handler + service default 重跑：5958 顶层 PASS，唯一失败为上游 TTFT 用例。golangci-lint 增量、Windows 全量、GOOS=linux 全量重跑均 0 issues。修复只涉及 handler/service 的 Codex 上下文代码、测试与文档；上面的 default/integration 全量、PG、前端结果来自修复前的暂存树，未重跑 |

2026-11-18 独立复核（暂存树 `162d9d72dadb6857570165d3947f3b8a8df4ebfe`）：原复现 11/11 放行；127 个本地/上游混合场景全部通过；84 个异常输入按预期 HTTP 400，7 个合法 JSON 转义场景放行；将 127 份实际 normalizer 输出交给 service，OAuth/API Key × 透传开/关共 508 次模拟上游转发的 input 内容、id、namespace 保留检查全部通过。连同相关既有回归，handler/service 两包共 91 个顶层 / 1020 个含子用例 PASS，0 FAIL/0 SKIP。上游 page 实现及 page/automation/delegation 测试文件均与固定目标同 blob。独立用例通过仓库外 overlay 运行，没有修改业务代码；原始日志、复现脚本和差异证据位于 `E:/tmp/xyai-merge-222-codex-recheck-20261118/`。这关闭原 P1，不代表全量测试全绿或真实模型验收通过。

Go 1.27.4，pnpm 9.15.4，Node 24.13.0；Go default/unit/integration 顺序执行，各自全新 GOTMPDIR、仓库固定 GOCACHE。原始 JSON 日志、三方差异、PG 迁移清单和快照位于仓库外 `E:/tmp/xyai-merge-222-20261117/`，不入库。

## 6 遗留与验证边界

| 事项 | 归属 / 处理 |
| --- | --- |
| `TestNativeImagesRoutesProxyTimeout` 时序 | #8680 兼容修复后的 unit 全量重跑中失败一次，隔离重跑与 handler 包重跑均通过（第 5 节）；首轮 unit 全量为 0 失败。属于既有的真实时序用例，未改 |
| 上游 TTFT 时序用例 | `TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode` 在 default、integration 全量各失败一次，纯上游快照复现（0.2.21 已登记），按要求不修，等待上游；default/integration 不是全绿 |
| repository 完整 Docker/Redis 集成 | 本机无 Docker/Redis；PG 覆盖来自 postgres-only harness 与仓库外补 DSN 的副本，Redis 依赖套件未运行，不等于完整套件 |
| postgres-only harness 限制 | 不设置 `integrationDSN`、Redis 为 nil 时会 panic 中止整包；属于测试工具既有边界，本轮未改 harness，只在仓库外副本临时补 DSN |
| 第一父既有 | `TestServerTimingConnectorRecordsDriverCallsWithoutRowLifetime` 时序断言在第一父快照同样失败一次，补 DSN 的整包重跑中通过；本地文件 `organization_usage_repo_integration_test.go` 的 gofmt 空白差异与 main 相同，未在合并分支整理 |
| 部署注意 | `251_group_usage_rollup_nonblocking_build.sql` 带 `SET LOCAL lock_timeout = '5s'`；state 行被长事务占用时迁移失败并整体回滚、不登记，等事务结束后重启即可 |
| go.sum 整理差异 | 同 0.2.21，按合并边界原样保留 |
| 实际外部行为 | 未做生产、真实模型/支付、浏览器端到端验收；7 个仅本地生产者原样转发后的上游实际响应未用真实模型复验（第一父自 2026-11-15 起即为该行为）；本轮是本地合并验证，不是上线验收 |
| 分支名 | 需求号 `10235` 按此前分支序号顺延，若有正式编号可在提交前改名 |

## 7 提交前状态

HEAD 和 main 仍为第一父，MERGE_HEAD 固定为指定上游。合并与 #8680 兼容修复保持已暂存；2026-11-18 独立复核只补充本 review，随合并一并暂存。原 P1 已关闭，未创建合并提交，等待用户审核后明确授权 commit。
