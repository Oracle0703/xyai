# Sub2API 0.1.173 上游同步记录（固定提交）

合并日期 2026-08-09 ｜ 工作分支 `feature/hy/10173_merge_sub2api_173`（自本地 `main@ddbb0426bfaa5623e31d588977004a9f62bb4772` 创建，与 `github/main` 一致）｜ 上游 `Wei-Shaw/sub2api main@48eb3766d2da817b171b45bb3036d42575e42b8f`（VERSION 0.1.173，覆盖 0.1.172/0.1.173，merge base `aac53afe0ef1`）｜ 合并提交 `8be093e9a775dcb1f0d3086d4939944ad140e194`（父 `ddbb0426b` + `48eb3766d`）｜ 状态：经 PR #35（`1558cd9f1`，2026-08-09）合入 main

本文是该轮同步的唯一记录。原始材料 `requirements.md`、`specs.md`、`plan.md`、`test-review.md`、`delivery-status.md`、`delivery-report.md`（本目录）已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-08-09 main sync (v0.1.173; awaiting review)` 节（"待用户审核，尚未创建"是提交前快照，实际合并提交见上）。

## 1. 范围

- merge base 到固定上游共 174 commits、493 files（`+39486 / -3055`）；提交前 staged 508 个路径。
- 三方路径集合：only-local 389、only-upstream 414、both 79（9 个文本冲突 + 70 个自动合并路径）。
- "我们自己的 GitHub"指 remote `github`（`Oracle0703/xyai`），`origin` 为内网 Git 服务。
- 约束：只解决冲突及合并必需的接口适配，不修上游既有缺陷（migration 206/220 等）；Wire/Ent 从源定义重新生成；migration 不重命名、不改已发布文件；根级 Grok 别名不得绕过 RequestArchive/RequestIntercept、API Key、group gate。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `.gitignore` | 双方规则并集 |
| `backend/cmd/server/wire.go`、`wire_gen.go`、`wire_gen_test.go` | 同时保留本地 Prompt Metrics、Token Analysis、并发 preset、quota flusher 与上游 Channel Monitor V2 的注入与 cleanup；`wire_gen.go` 重新生成无漂移 |
| `backend/internal/config/config.go`、`backend/internal/service/setting_service.go` | 保留本地 archive/intercept 配置与风险控制回调 round-trip，加入上游 Grok、Channel Monitor V2、邮箱主域额度与 monitor listeners |
| `backend/internal/service/domain_constants.go` | 双方常量并集 |
| `backend/internal/server/routes/gateway.go` | 根级 video / TTS / STT / custom voices / realtime / web_search 别名显式挂 `RequestArchive -> RequestIntercept`，并继续执行 API Key、composite/group gate；Responses guard 仍位于 Intercept 前 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | 先执行上游 response-model observer，再执行本地 compatible cache usage 规范化 |

自动合并适配（不改生产逻辑）：
- 两个 `GroupsView` 测试中自动合并出重复声明的同名 `getLiveCapability` mock，按双方相同语义去重（focused 2 files / 10 tests 通过）。
- 本地测试 stub 对齐上游新增接口签名（`go test ./internal/service` 首轮失配后通过）。

独立冲突语义审查与最终代码审查：0 Critical / 0 Important / 0 Minor。

## 3. 保留的本地能力与语义适配

合并前 23 个 tracked `docs/features/` 文件全部保留（上游未修改或删除）。RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、子管理员、OpenAI-compatible cache usage、默认 reasoning effort、用户并发 preset 和 quota flusher 均保留；真正重叠的能力采用上游实现。

数据口径：本地 `upstream_model`（发给上游的映射模型）与上游新增 `upstream_response_model`（上游实际返回模型）并存，不混为同一字段；mismatch 为 `NULL/false/true` 三态，`false` 筛选不包含 `NULL`；与本地 cache usage 互斥桶并存。

## 4. 采用与未采用的上游行为

| 领域 | 内容 |
| --- | --- |
| Channel Monitor V2 | 被动聚合与隐私视图；admin/user API、repository、service、前端视图与 feature gate；默认仍为 V1，V2 显式启用 |
| Grok | Voice / Realtime / Video / Web Search 路由与根别名；Redis 单次消费 OAuth session（先校验 state/redirect/client 再消费，远端 miss 不复活本地旧 session）；分组视频/音频/搜索独立定价字段，不复用图片定价 |
| 计费与审计 | 上游响应模型审计；Gemini 生图按实际张数计费（实际图片张数观察器仅覆盖 `GeminiMessagesCompatService`）；非白名单邮箱主域单账户额度 |
| migrations | 双 `194_`、双 `195_` 及 196–206、217–220；runner 按完整文件名排序与去重，不为数字前缀重复而重命名 |
| 未修（上游原样） | migration 206/220 数据风险、上游文档 trailing whitespace、`go.sum` 非 tidy，见第 6 节 |

## 5. 验证

| 命令 / 检查（`backend` / `frontend`） | 结果 |
| --- | --- |
| 合并前基线：`go test -p 1 -count=1 ./...`、前端 lint/typecheck/build | 通过；全量 Vitest 1624/1626，2 条 rollback timeout 断言为既有失败 |
| `go generate ./ent`、`go generate ./cmd/server` | 通过，无生成物漂移 |
| config / routes / cmd server / service 定向 | 通过 |
| `go test -p 1 -count=1 ./...`（默认 tag） | 通过 |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 仅第一父既有 `/auth/me` fixture（`admin_permissions:null`）失败 |
| `go build ./...`、`go build -tags embed` | 通过；embed 产物 160,223,744 bytes |
| 全量 / 增量 golangci-lint | 全量 28 项均为第一父既有；`--new-from-rev=ddbb0426...` 0 issues |
| `go mod tidy -diff` | 退出 1，只建议删除固定上游的 5 组 CLI 传递 checksum，未改 `go.sum` |
| 前端 lint、typecheck | 通过 |
| 完整 Vitest | 246/246 files、1694/1694 tests 通过 |
| 前端 production build | 通过，1052 modules |
| Git 审计 | 0 unstaged / 0 untracked / 0 unmerged；23/23 features；排除 `docs/channel-monitor-v2-safe-defaults.md` 后 `git diff --cached --check` 通过 |
| Wiki 图谱 | 33 nodes / 67 edges、49 wikilinks、0 unresolved；未提交时状态为预期 `PARTIAL` |

## 6. 遗留问题

| 级别 | 问题 | 处置 |
| --- | --- | --- |
| P1 上游 | `backend/migrations/206_channel_monitor_v2_privacy_defaults.sql` 无法区分默认 false 与管理员主动 false，会把部分显式 false-like 隐私设置迁为 true | 不在本分支修；部署前核验生产数据 |
| P1 上游 | `backend/migrations/220_clear_non_grok_video_generation_config.sql` 会清空非 Grok、非 composite 的视频价格；旧 checksum 库不会自动恢复早期 composite 数据 | 同上 |
| 配置 | YesCaptcha key 未进入部署示例，开启 Grok 密码登录时可能缺 key | 启用前显式配置并做端到端验证 |
| P2 上游 | `docs/channel-monitor-v2-safe-defaults.md` 第 3、4、50 行 trailing whitespace，全量 diff check 会非零 | 与固定上游 blob 一致，原样保留 |
| 基线 | `/auth/me` fixture、全量 lint 28 项 | 保持不改；lint 见 `docs/features/technical-debt-board-cn.md` TD-004 |
