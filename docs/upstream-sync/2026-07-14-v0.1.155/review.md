# Sub2API 0.1.155 上游同步记录

合并日期 2026-07-14 ｜ 工作分支 `feature/hy/10155_同步sub2api主线`（自本地 `main@20e6379f9243f00aaf84b562af40c7b80793d4fc` 创建）｜ 上游 `Wei-Shaw/sub2api main@7c717365ef728e53cdcf6d639a4dd68226db03b2`（VERSION 0.1.155，merge base `55ed0ab0da36` = 上轮固定的 0.1.153）｜ 合并提交 `d294d493705a252a5038287a96643a43a38b330e`，后续修复 `07cd3195c`、文档 `9004a6650` ｜ 状态：已经 PR #19（`59f36654f`）与 PR #20（`c78859c5c`）于 2026-07-14 合入 main

本文是该轮同步的唯一记录。原始材料 `requirements.md`、`specs.md`、`plan.md`、`test-review.md`、`delivery-status.md`、`delivery-report.md`（本目录）已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-14 main sync (v0.1.155)` 节。

## 1. 范围

- 上游 71 commits、238 files（`+15089 / -880`）；合并结果相对第一父 238 files（`+15087 / -879`）。
- 合并前用 `git merge-tree --write-tree --messages main upstream/main` 预演：3 个文本冲突、23 个双方修改文件。
- 执行决策：开工前把原分支 `feature/hy/0710_组织用量报表` 的未完成进度存为 WIP 提交 `1ed588fc3`，再从本地 `main` 建新分支（不使用 worktree）；"保留远端修改"解释为保留上游新增行为，而不是冲突时整文件取 `theirs`；固定 SHA 防止远端继续移动。
- 不在范围：推送、建 PR、部署、继续原分支需求、清理 lint 债务。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `backend/internal/repository/redis.go` | 保留本地 `InitRedis(...)(*redis.Client, error)`、启动时 Redis 7+/Memurai 版本校验和失败时关闭客户端；`server.enable_server_timing` 开启时再注册上游 `serverTimingRedisHook`。 |
| `backend/internal/server/router.go` | 同时挂载上游 `ServerTiming` 与本地 Prompt Metrics `CaptureMiddleware`；保留 Prompt Metrics 管理路由与合规确认门。 |
| `backend/internal/service/content_moderation.go` | 保留本地可选 `config.Config`、Prompt Risk judge semaphore 与 LLM 语义审核；共享 HTTP client 改为上游 `servertiming.InstrumentClient(nil)`。 |

自动合并审查：对 23 个双方修改文件逐个检查相对本地 `main` 的删除行与合并结果，Wire、config、admin routes、OpenAI gateway、usage、前端账号页 locale 均为上游增量叠加，未发现本地合同被静默删除。

生成一致性修复（`07cd3195c fix: 修复 Wire 生成依赖解析`）：`backend/cmd/server/main.go` 的 Wire 指令改为 `//go:generate go run -mod=mod github.com/google/wire/cmd/wire`，重新生成 `wire_gen.go` 并补 `go.sum`；`go generate ./ent` 输出稳定。当前源码仍是该指令。

## 3. 保留的本地能力与语义适配

| 能力 | 状态 |
| --- | --- |
| Prompt Risk / 内容审核 / LLM judge | 保留，配置注入、judge fail-open、管理路由与 Prompt Metrics |
| RequestArchive / RequestIntercept | 保留，网关路由、中间件链、运行时设置与管理入口 |
| Token Analysis / 图片生成 | 保留，admin service/repository、`/image-gen`、batch image、OpenAI/Grok images 并存 |
| 用户并发与缓存错误 | 保留，preset/runner、普通 slot、`ConcurrencyCacheError` 与 503 语义 |
| OpenAI-compatible options / cache usage | 保留，Responses→Chat options adapter、provider preset、DeepSeek `prompt_cache_hit_tokens`，cache write/read 互不覆盖 |
| Redis 启动约束 | 保留，Redis 7+/Memurai 检查与 Server-Timing hook 组合生效 |

## 4. 采用的上游行为

| 领域 | 内容 |
| --- | --- |
| 可观测性 | opt-in 管理端 Server-Timing（SQL、Redis、外部 HTTP、cache 与总耗时）；管理请求统一标记 `X-Admin-UI-Request`；Ops 系统日志 host 筛选 |
| OpenAI 兼容 | native Responses namespace 可逆摊平/回程恢复；Responses Lite 保留客户端 image tools；图片非流式 keepalive 与终态修正；Codex manifest API Key failover |
| 计费与数据 | OpenAI 长上下文费率改为账号级 opt-in（默认关闭），应用结果写入 usage log；migrations 174–176 及 Ent 生成代码 |
| Grok | Web SSO → Build OAuth 批量导入与导入后 probe；Channel Monitor provider；滚动 24h Free 配额估算；OAuth media 官方 API 路由；reasoning null 清理 |
| 可靠性 | scheduler 全量重建并发合并/事件延迟修复；代理到期与账号自动暂停触发重建；HTTP/2 keep-alive PING；quota reset credit 检测 |
| 前端 | 账号页 Grok SSO 与 OpenAI 长上下文计费开关；监控页 Grok/Free 状态；Ops 日志 host 条件 |

## 5. 验证

| 命令（`backend` / `frontend`） | 结果 |
| --- | --- |
| `git rev-list --parents -n 1 d294d4937` | 父为 `20e6379f9` + `7c717365e`；VERSION `0.1.155` |
| 未解决文件、冲突标记、`git diff --check` | 通过 |
| `go generate ./ent`、`go generate ./cmd/server` | Wire 指令修复后通过，生成结果稳定 |
| `go test -tags=unit -p 1 -count=1 ./...` | 通过；首轮 `cmd/server`、`ent/schema`、`internal/service` 遇 Windows 文件锁，换全新 `GOTMPDIR` 重跑通过（`internal/service` 100.3s） |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过；`Access is denied` 中断的包逐包用全新 `GOTMPDIR` 重跑通过，`responseheaders` 第 2 次重试通过 |
| 上游前端专项 Vitest | 12 files / 129 tests 通过 |
| `pnpm --dir frontend exec vitest run` | 163 files / 1055 tests 通过 |
| `pnpm --dir frontend run typecheck` | 通过 |
| `pnpm --dir frontend run lint:check` | 单独运行通过（并行首跑只撞上 Vite 临时时间戳文件） |
| 全仓 `golangci-lint run ./...` | 非门禁：29 个既有问题；用户把本阶段验收限定为测试用例通过 |

## 6. 遗留问题

- 全仓 lint 基线债务仍在，见 `docs/features/technical-debt-board-cn.md` TD-004。
