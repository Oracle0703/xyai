# Sub2API 0.2.9 固定提交合并审核

## 合并边界

| 项目 | 结果 |
| --- | --- |
| 日期 | 2026-09-29 |
| 工作分支 | `feature/hy/10211_merge_sub2api_209` |
| 第一父 / 本地 main | `7373dc2674be8266cd048b7a79e0c184650a3f8b` |
| 上游分支 / 目标 | `Wei-Shaw/sub2api main@9a62841fd124d026cf3694fcf9b79e98addcdbdc` |
| 共同祖先 | `a3eb7ef302961cba716dc78b39b93b60c467db0e` |
| 版本 / 上游增量 | `0.2.9`；70 commits、117 paths、+3133/-378 |
| 合并提交 | 尚未创建；`MERGE_HEAD` 固定目标，等待用户审核 |
| 范围 | 仅冲突解决和必要调用适配；不修复上游自身 bug，不 commit/push/PR/部署 |

## 冲突裁决

| 文件 | 处理 |
| --- | --- |
| `backend/internal/handler/concurrency_error_response_test.go` | 同时保留本地 service 和上游 gin import，保留本地缓存故障与上游 Google 格式测试 |
| `backend/internal/handler/gemini_v1beta_handler.go` | 重叠并发错误处理采用上游 `googleConcurrencyError(err, slotType)` 统一入口；共享映射仍保留本地缓存故障脱敏 503，保留本地 `ParseGeminiModelActionPath` |
| `backend/internal/handler/gemini_v1beta_handler_test.go` | 无文本冲突，但本地旧调用补 `"user"` 参数适配上游签名，原断言不变 |

## 自动合并与本地功能核对

12 个双方修改文件逐项对比共同祖先、第一父和上游增量：上述前两个冲突文件，以及 `gateway_handler_error_fallback_test.go`、`chatcompletions_responses_test.go`、`chatcompletions_to_responses.go`、`types.go`、`openai_gateway_forward.go`、`openai_gateway_responses_chat_fallback.go`、`openai_gateway_service_test.go`、`openai_gateway_usage.go`、中英文 `admin/overview.ts`。

- 105 个仅上游修改路径的合并 blob 全部等于固定目标。
- 680 个仅本地修改路径中，业务源码不变；仅 Gemini 测试调用适配上游签名（后续 wiki/台账为本轮文档变更）。
- 36 个已跟踪 `docs/features` 文件/资源全部保留；台账只追加记录，设计/审核材料不会被误认为已实现功能。
- 本地归档/拦截、Prompt Metrics/Risk、Token Analysis、组织/部门用量、子管理员、订阅自助重置、GPT 额度展示、并发预设和 quota flusher 的 provider/路由继续保留。
- Chat → Responses 保留 cache key/previous response 与流式生命周期字段，采用上游 `type=message`；OpenAI 转发保留本地 schema format 清理和 thinking 字段清理，接入上游 beta 处理。
- Responses → Chat 保留本地第三方参数过滤与 reasoning 回注，接入上游 OpenCode DeepSeek placeholder；用量保留本地图片缺价逻辑，并接受上游 Free Fast 缺价记录和账号长上下文成本 gate。
- 无依赖、schema/migration、Wire 变化，未做无关生成或重构。上游新增修复按目标原样合入，不对其额外修 bug。

## 验证

原始日志保留在本机 `backend/.gocache/merge209-*.log`，不入库。Go 命令在 `backend/` 执行，复用仓库 cache、每组独立 GOTMPDIR，PATH 加入 `F:/an/Git/usr/bin`（只作用于测试进程）；前端命令在 `frontend/` 执行。

| 验证命令/范围 | 结果 |
| --- | --- |
| Go handler/apicompat/service 专项，`-run 'Concurrency\|Gemini\|DeepSeek\|CompatibleCache\|RecordUsage\|LongContext\|GroupModelAllowlist\|Beta\|ContextRollover'` | 通过，3 包 |
| `go test -p 1 -count=1 ./...` | 通过，54 个有测试包返回 ok |
| `go test -tags=unit -p 1 -count=1 -v ./...` | 失败：service 包第一父已有 `ptrFloat` 重复声明；其余 59 包返回 ok，14 个显式 skip |
| `go test -tags=integration -p 1 -count=1 -v ./...` | 命令退出 0；18 个显式 skip，repository 包另因 Docker 不可用整包跳过，不等于完整集成覆盖 |
| `go build -o .gocache/merge209-server.exe ./cmd/server` | 通过 |
| `go build -tags=embed -o .gocache/merge209-server-embed.exe ./cmd/server` | 通过 |
| `go mod tidy -diff` | 通过，无依赖漂移 |
| golangci-lint 2.13.0 / Go 1.27.0：`run --new-from-rev=HEAD ./...` | 通过，0 issues |
| `pnpm.cmd run lint:check` / `typecheck` | 通过 |
| `pnpm.cmd run test:run` | 373 files / 2810 tests 通过 |
| `pnpm.cmd run build` | 通过；保留上游 Browserslist 数据过期和大 chunk 提示，未额外优化 |
| wiki 图谱刷新 / `tools/check-understand-status.cmd -AllowDirtyWiki` | 35 nodes / 76 edges / 58 wikilinks / 0 unresolved；READY 仅表示允许待提交 wiki 的图谱状态 |

integration 未执行范围包含：Docker 仓储和限流集成、Prompt Audit 专用 Redis/PG、Windows symlink 权限、外部 TLS capture、TypeSafe live、OpenAI key、本地插件包，以及一个既有 DingTalk sentinel。未使用真实业务库或真实模型调用来填补前提。

结论：冲突处理与代码保留检查已完成，可进入人工提交审核；不是全测试全绿或生产上线 GO。未扩大范围修复第一父/上游问题。

## 已确认的基线阻塞（本轮不修）

`go test -tags=unit -p 1 -count=1 -v ./...` 中 `internal/service` 编译失败：

- `backend/internal/service/payment_config_plans_validation_test.go:137`（`//go:build unit`）定义 `ptrFloat`。
- `backend/internal/service/gpt_quota_display_test.go:287`（无 build tag）再次定义 `ptrFloat`。
- 两文件均与第一父 `7373dc2674be8266cd048b7a79e0c184650a3f8b` blob 一致；第一父已有相同声明和 tag，故为本地 main 既有 unit-tag 编译冲突，而非此次上游合并引入。该归属依据是第一父源码/构建条件对比，未另建基线 checkout 重跑。
- 未改测试 helper、未删除测试、未绕过 unit tag；该包 unit 专属用例未能运行，不能宣称全量测试全绿。default 全量已通过。

## 提交前状态

- 分支 `feature/hy/10211_merge_sub2api_209`；HEAD/main 仍为原第一父，MERGE_HEAD 精确等于指定目标。
- 128 个暂存路径；未解决冲突、未暂存修改、未跟踪文件均为 0。`git diff --cached --check` 通过。
- 历史台账前缀与第一父逐字一致；除追加台账外，原 36 个 features 文件/资源全部保留且 blob 不变。
- 未创建合并提交，等待人工审核；不将此状态等同于全绿或上线验收。
