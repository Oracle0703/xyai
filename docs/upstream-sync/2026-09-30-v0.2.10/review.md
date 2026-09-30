# Sub2API 0.2.10 固定提交合并审核

## 合并边界

| 项目 | 结果 |
| --- | --- |
| 日期 | 2026-09-30 |
| 工作分支 | `feature/hy/10213_merge_sub2api_210` |
| 第一父 / 本地 main | `bdf31e87223058200c614a807e24169e12bb31ea` |
| 上游分支 / 目标 | `Wei-Shaw/sub2api main@a60a29549f488a854966aaec9541abbe006cac22` |
| 共同祖先 | `9a62841fd124d026cf3694fcf9b79e98addcdbdc` |
| 版本 / 增量 | `0.2.10`；33 commits、118 paths、+3759/-332 |
| 合并提交 | 尚未创建；MERGE_HEAD 固定目标，等待人工审核 |
| 范围 | 仅解决文本/语义冲突和必要适配，保留本地独有功能；不修复上游自身 bug |

fetch 时 upstream/main 已前进到 `a0f41f95a`；本轮严格使用指定目标，没有把后续提交混入候选。

## 15 个文本冲突裁决

| 文件（仓库相对路径） | 处理 |
| --- | --- |
| `backend/cmd/server/wire_gen.go` | 从合并后的 provider 源图重新生成；保留所有本地 provider 和 plugin account directory，增加 Claude reset credit 与 WS composite resolver；两次生成一致 |
| `backend/internal/handler/admin/dashboard_handler.go` | 本地 user_ids 验证/limit=0 与上游 metric 规范化合并 |
| `backend/internal/handler/admin/dashboard_handler_cache_test.go` | 保留双方用例，增加选中用户时 metric 缓存隔离/同集合 canonicalization 交叉验证 |
| `backend/internal/handler/admin/dashboard_query_cache.go` | 独立用户缓存键同时包含 user_ids 与 metric，保留 slice 拷贝隔离 |
| `backend/internal/handler/admin/dashboard_snapshot_v2_handler.go` | 显式 nil user_ids 和 tokens，保持两侧默认行为 |
| `backend/internal/repository/content_moderation_repo.go` | 同时保留 prompt_risk action 排除和上游两个 log-only mode 排除 |
| `backend/internal/repository/usage_log_repo_integration_test.go` | 保留本地选人/上海日小时桶测试与上游按指标 Top 排名测试，适配联合签名 |
| `backend/internal/repository/usage_log_repo_trend.go` | 保留本地选人 SQL，未选人时采用上游 metric 排名表达式 |
| `backend/internal/server/api_contract_test.go` | stub 适配 userIDs + metric 签名，不改 golden |
| `backend/internal/service/account_usage_service.go` | 仓储接口联合签名保留双方能力 |
| `backend/internal/service/account_usage_service_batch_test.go` | stub 同步联合签名 |
| `backend/internal/service/content_moderation.go` | 保留独立 Prompt Risk 前置阶段、judge header、runtime config/hash，并合入上游 allowlist/log-only；本地热更新保留白名单快照 |
| `backend/internal/service/dashboard_service.go` | 向仓储完整传递 userIDs + metric |
| `frontend/src/api/admin/dashboard.ts` | 同时保留 user_ids 与 metric |
| `frontend/src/views/admin/DashboardView.vue` | 保留本地 safeNumber 与消费榜/请求序号，接受上游消费切换和格式化 |

新增 `backend/internal/service/content_moderation_merge_contract_test.go` 仅验证本地 Prompt Risk/总开关热更新不会清除上游白名单，不修改上游测试或生产行为。

## 自动合并与功能保留

- 37 个双方修改路径全部核对；除上述 15 个外，涉及 domain constants、settings DTO/service/API/locale、handler Wire、service Wire、admin routes、OpenAI handler 及测试、apicompat types、moderation SQL 测试、CreateAccountModal 和 SettingsView。上游增量和本地独有增量均保留。
- 81 个仅上游修改路径的工作区内容与目标一致（忽略 Git 工作树换行转换）。656 个仅本地修改路径在文档更新前与第一父一致，含第一父已有删除；没有恢复已删除文件。36 个既有 `docs/features` 文件/资源在台账追加前全部一致。
- 本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk/LLM judge、Prompt Audit、Token Analysis、组织/部门用量与授权、子管理员、订阅自助日重置、GPT 额度展示、并发预设、quota flusher、兼容参数能力保持独立。
- Claude reset credit 是管理端按需查询，与本地 GPT 额度快照页不重叠。趋势指标切换也不替代选人趋势。真正重叠的模型路由归属、协议/用量处理直接使用上游实现。
- **白名单边界：** 本地 Prompt Risk 原有独立白名单/前置检查继续生效；上游风控白名单适用于其内容审计/cyber 阶段，不自动越过本地独立 Prompt Risk、Prompt Audit 或请求拦截。此为保留本地能力的冲突裁决，不额外扩大豁免权限。
- 无 schema、migration、依赖变更；不生成 Ent。Wire 工具附加的 subcommands checksum 已还原，`go mod tidy -diff` 无漂移。

## 验证

原始日志：本机 `backend/.gocache/merge210-*.log`（不入库）。Go 使用 repo-local review cache/GOPATH、每组 fresh GOTMPDIR、`-p 1 -count=1`，完整套件 PATH 加入 `F:/an/Git/usr/bin`。前端使用既有 pnpm 依赖；无生产服务/业务库/真实模型调用。

| 命令 / 范围 | 结果 |
| --- | --- |
| handler/admin、handler、service、repository 的 Dashboard/UserUsageTrend/PromptRisk/Allowlist/ClaudeResetCredit 专项 | 4 包通过 |
| 合并交叉合同专项 | 2 包通过；新增测试首轮指针夹具类型错误已修正，最终日志 `merge210-contract-final.log` |
| `go test -p 1 -count=1 -v ./...` | 通过，54 包返回 ok、14 个显式 skip；新增交叉合同以随后专项补充验证 |
| `go test -tags=unit -p 1 -count=1 -v ./...` | 退出 1：service 包被第一父已有 ptrFloat 重复声明阻塞；其余 59 个包返回 ok，14 个显式 skip |
| `go test -tags=integration -p 1 -count=1 -v ./...` | 退出 1：cyber snapshot 用例一次 1 秒超时；53 包返回 ok，18 个显式 skip，repository 另因 Docker 不可用整包跳过 |
| Wire 连续生成两次 | 通过，生成物 hash 一致 |
| `go mod tidy -diff` | 通过，无漂移 |
| Go server normal / embed build | 通过 |
| golangci-lint 2.13 `run --new-from-rev=HEAD ./...` | 通过，最终 0 issues（含新增测试） |
| `pnpm.cmd run lint:check` / `typecheck` | 通过 |
| `pnpm.cmd run test:run` | 374 files / 2828 tests 全部通过 |
| `pnpm.cmd run build` | 通过，保留上游 Browserslist/大 chunk 提示 |
| Wiki 图谱刷新 / 状态检查 | 35 nodes / 76 edges / 59 wikilinks / 0 unresolved；AllowDirtyWiki 返回 READY |

## 已确认的本地基线阻塞

unit 的 `internal/service` 包不能编译：`payment_config_plans_validation_test.go:137` 与 `gpt_quota_display_test.go:287` 同时定义 `ptrFloat`。前者带 unit build tag，后者无 tag。两文件与第一父 blob 一致，第一父已有相同声明，故不是此次合并引入；本轮不改 helper、不删除或绕过测试。该归属基于第一父源码核对，未另建第一父 checkout 重跑。

## 集成测试失败与覆盖边界

- 唯一失败为 `TestRecordCyberPolicyEvent_RuntimeSnapshotRefreshFailureKeepsStaleScope`，在 `content_moderation_cyber_test.go:281` 等待后台刷新计数 1 秒超时；同一测试文件在第一父和指定上游一致，本轮 default 中通过。
- 使用 fresh GOTMPDIR，执行 `go test -tags=integration -p 1 -count=3 -v ./internal/service -run '^TestRecordCyberPolicyEvent_RuntimeSnapshotRefreshFailureKeepsStaleScope$'`，3/3 通过。首次失败仍保留，不能将复跑通过改写为全量 integration 通过。未修改实现或测试超时；此前同类波动见 ops 中 0.2.4/PR #6924 历史记录，此处不宣称已修复根因。
- 18 个显式 skip 涉及 Docker 限流、Prompt Audit 专用 Redis/PG、Windows symlink 权限、TLS capture、TypeSafe/OpenAI live、插件包及 DingTalk sentinel；repository TestMain 另打印 Docker 不可用并跳过整包。用户趋势真实数据库/上海时区/Top 指标用例仅完成编译，未获得真实 PostgreSQL 运行覆盖。
- 未配置业务库或调用真实模型补齐环境；不把通过编译或命令结束等同集成验收。

## 提交门禁

- 分支 `feature/hy/10213_merge_sub2api_210`；HEAD 与 main 仍为原第一父，MERGE_HEAD 精确等于目标。
- 最终 129 个暂存路径；未解决冲突、未暂存修改、未跟踪文件均为 0，`git diff --cached --check` 通过。
- 81 个仅上游路径的 index blob 全等于目标；仅本地路径只增加本轮 wiki/图谱和台账变化，业务源码保持第一父。36 个既有 features 文件/资源全部保留，唯一修改为台账追加，历史字节前缀不变。
- 六个修改的 wiki 正文保持 LF；图谱已刷新，AllowDirtyWiki 的 READY 仅描述待提交 wiki 状态。
- 当前可进入人工合并提交审核；unit/integration 边界仍在，不是全绿或生产上线 GO。尚未 commit/push/PR/部署。
