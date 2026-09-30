# Sub2API 0.2.11 固定提交合并审核

## 边界

| 项目 | 结果 |
| --- | --- |
| 日期 | 2026-09-30 |
| 工作分支 | `feature/hy/10214_merge_sub2api_211` |
| 第一父 / 本地 main | `ba53ec78d557ba2aa08efa5e28f2dde869b011da` |
| 上游分支 / 固定目标 | `Wei-Shaw/sub2api main@42bc7f6cffe24bcb471608e48e66b4a0afa1f882` |
| 共同祖先 | `a60a29549f488a854966aaec9541abbe006cac22` |
| 版本 / 上游增量 | `0.2.11`；23 commits / 90 paths / +5146 -296 |
| 合并提交 | 尚未创建；保留 MERGE_HEAD 等待人工审核 |
| 范围 | 仅冲突解决；本地独有功能保留，重叠行为采用上游，不修上游 bug |

开始时本地 main 工作区干净，未拉取或改写本地 main；fetch 后 upstream/main 与指定目标一致。没有跟随其他提交。

## 冲突裁决

| 文件 | 裁决 |
| --- | --- |
| `backend/cmd/server/wire_gen.go` | 由合并后的源图连续生成两次，hash 一致。幂等协调器提前初始化供上游 Claude reset provider 使用；保留本地 GPT 额度、订阅自助重置、组织/部门、Token Analysis、并发预设、Prompt Metrics/Risk、插件目录及 cleanup 接线 |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 保留本地提前解析模型、第三方 temperature/max-completion 参数过滤及 reasoning 缓存；加入上游映射后 GPT-6.1 Sol effort/Chat-only tools 校验，删除冲突块中重复的模型/effort 声明，不改上游校验行为 |

没有新增业务修复或替换上游测试断言。无 schema/migration 变化，不生成 Ent；Wire 运行产生的两个 subcommands checksum 已恢复，依赖文件无业务差异。

## 自动合并与本地功能保留

- 19 个双方修改路径逐项对照第一父、共同祖先与目标；新计费预留与本地并发错误/Retry-After 校验独立，默认 effort/大请求保护/compatible cache usage 与上游模型校验并存，管理路由未丢本地授权和订阅入口。
- 文档更新前，71 个仅上游修改路径的 index blob 全等于目标；695 个仅本地修改路径全等于第一父（含既有删除）。29 个现存 `docs/features` 文件逐 blob 保持第一父，没有删除或覆盖。
- RequestArchive/RequestIntercept、Prompt Metrics/Risk/LLM judge、Token Analysis、组织/部门报表和权限、子管理员、GPT 额度展示、订阅自助重置、用户并发预设、quota flusher、compatible 参数及缓存用量等本地独有能力保留。
- Claude 原生账号 reset grant 不等于本地订阅日限重置或 GPT 额度快照，三者均保留。OpenAI 当前订阅档位、GPT-6.1 Sol 元数据/计费、Codex 远程目录使用固定上游实现。
- 组件 README 和六份 Wiki 同步；发现 keys README 仍描述旧 `xunyou` provider，第一父源码已是 `OpenAI`，只修正文档，不回改源码。

双方修改路径：

- `backend/cmd/server/wire_gen.go`
- `backend/internal/config/config.go`
- `backend/internal/handler/gemini_v1beta_handler.go`
- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/pkg/apicompat/chatcompletions_to_responses.go`
- `backend/internal/server/api_contract_test.go`
- `backend/internal/server/routes/admin.go`
- `backend/internal/service/openai_codex_transform.go`
- `backend/internal/service/openai_gateway_chat_completions.go`
- `backend/internal/service/openai_gateway_chat_completions_raw.go`
- `backend/internal/service/openai_gateway_chat_completions_test.go`
- `backend/internal/service/openai_gateway_messages.go`
- `backend/internal/service/openai_gateway_request_body.go`
- `backend/internal/service/openai_gateway_responses_chat_fallback.go`
- `backend/internal/service/wire.go`
- `deploy/config.example.yaml`
- `frontend/src/components/account/__tests__/credentialsBuilder.spec.ts`
- `frontend/src/i18n/locales/en/admin/accounts.ts`
- `frontend/src/i18n/locales/zh/admin/accounts.ts`

## 验证

日志位于本机 `backend/.gocache/merge211-*.log`，不入库。Go 固定 `GOCACHE=backend/.gocache/review-cache`、repo-local GOPATH、每组独立 GOTMPDIR、`-p 1 -count=1`；测试进程 PATH 加入 `F:/an/Git/usr/bin`。本轮完整测试/Wire 的 GOMODCACHE 沿用宿主 `D:/project/pkg/mod`，tidy/build/lint 显式使用 repo-local `review-gopath/pkg/mod`。使用 backend/go.mod 的 Go 1.27.0，不按仓库根默认 Go 版本推断。

| 命令 | 结果 |
| --- | --- |
| `go test -p 1 -count=1 -v ./...` | 通过，54 包返回 ok，14 个显式 skip |
| `go test -tags=unit -p 1 -count=1 -v ./...` | 退出 1：service 包 ptrFloat 重复声明，59 包返回 ok，14 个显式 skip |
| `go test -tags=integration -p 1 -count=1 -v ./...` | 退出 0，54 包返回 ok；18 个显式 skip，repository 另因 Docker 缺失整包跳过 |
| Wire 连续两次生成 | 通过，生成物 hash 一致 |
| `go mod tidy -diff` | 通过，无漂移 |
| Go server normal / embed build | 通过 |
| golangci-lint 2.13 `run --new-from-rev=HEAD ./...` | 通过，0 issues |
| `pnpm.cmd run lint:check` / `typecheck` | 通过 |
| `pnpm.cmd run test:run` | 374 files / 2871 tests 全部通过 |
| `pnpm.cmd run build` | 通过；保留 Browserslist/大 chunk 提示 |
| Wiki 刷新 / 状态检查 | 35 nodes / 76 edges / 59 wikilinks / 0 unresolved；`-AllowDirtyWiki` 返回 READY |

default 包含上游 `TestGPT61SolOnlyChatFallbackRejectsToolsAndDisabledReasoning`，覆盖此次回退冲突中的拒绝行为。

## 已核实的第一父测试阻塞

`internal/service/payment_config_plans_validation_test.go:137`（unit tag）与 `internal/service/gpt_quota_display_test.go:287` 同时定义 `ptrFloat`。两文件当前内容的 Git blob 均等于第一父，重复声明不是本次合并引入；遵守仅冲突范围，不重命名 helper 或修改测试。归属基于第一父源码及 blob 对比，未另建第一父 checkout 重跑。


unit 的 service 包未获得运行覆盖，包含本地默认 effort/Responses→Chat unit 测试及上游 `billing_inflight_reservation_test.go`；不能用 default 通过替代这些 unit 用例。未移除、改名或绕过测试以制造全绿。

## 环境与验收边界

- default 的 14 个显式 skip 包含 DingTalk sentinel、Windows symlink 权限、Prompt Audit 专用 Redis/PostgreSQL、TypeSafe/OpenAI live 与外部插件包；未调用真实模型或改业务库来补齐前提。
- integration 日志已确认 Docker 不可用，repository 的 TestMain 跳过整包，真实数据库/事务覆盖未获得；不能把该包的 ok 输出当作数据库用例运行成功。Docker 限流与其他显式 skip 同样不计入覆盖。
- 未启动业务服务或做浏览器/真实上游重置验收，本轮结论限于合并候选与本机自动检查，不是生产发布批准。

## 提交审核状态

- HEAD 与本地 main 保持 `ba53ec78d557ba2aa08efa5e28f2dde869b011da`；MERGE_HEAD 为指定 `42bc7f6cffe24bcb471608e48e66b4a0afa1f882`。
- 本轮只增加冲突融合、上游改动及相应文档/导航；上游与第一父既有问题不修。合并台账仅尾部追加，29 个 features 文件保持第一父。
- 可进入人工 commit 前审核；unit 阻塞和集成覆盖缺口保留，不宣称全绿。未 commit、push、创建 PR、合回 main 或部署。

最终检查：103 个暂存路径；未解决冲突、未暂存修改、未跟踪文件均为 0；`git diff --cached --check` 通过。71 个仅上游路径的 index blob 等于目标；仅本地路径除本轮文档/图谱/台账外全部保持第一父，台账历史字节前缀不变，29 个 features 文件逐 blob 不变。

## 提交前复核补充

- 冲突复核：两处冲突及 19 个双方修改路径的结果均为“第一父 + 上游增量”；`wire diff` 无差异；default tag 下 service/handler/apicompat/openai/server/config/repository 通过。
- unit 补跑：用 `go test -overlay` 在临时文件中去掉 `payment_config_plans_validation_test.go` 的重复 `ptrFloat`（工作区不改），unit tag 下 server/handler/apicompat/openai/repository 通过；service 包仅 `TestApplyDefaultOpenAIReasoningEffort/config_none_normalizes_to_empty_->_disabled` 失败。
- 该失败在第一父 `ba53ec78d` 的临时 worktree 中同样复现，非本次合并引入：上游 0.2.7 起 `normalizeOpenAIReasoningEffort` 接受 `none`，本地测试预期过时；配置校验只允许 low/medium/high/xhigh，运行时不受影响。遵守仅冲突范围，未修改。
