# Sub2API 0.1.151 上游同步记录

合并日期 2026-07-13 ｜ 工作分支 `feature/hy/10151_同步sub2api主线`（第一父 `89711be2324d`）｜ 上游 `Wei-Shaw/sub2api main@42f3c22830b8b15650b12faeb38bbadb1641e6b1`（VERSION 0.1.151，merge base `6dd3274aafbc`）｜ 合并提交 `5655815f283e6e208352cffb4f3acbb7b2f4e0a1` ｜ 状态：随 `feature/hy/10153_同步sub2api主线` 经 PR #16（`177503f60`，2026-07-13）合入 main

本文是该轮同步的唯一记录。执行计划 `docs/superpowers/plans/2026-07-13-sub2api-v0.1.151-sync.md` 中仍有价值的决策与验证结果已并入本文，计划已移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-07-13 main sync (v0.1.151)` 节。

## 1. 范围

- 上游 47 commits、87 files（`+7043 / -361`）；合并结果相对第一父 88 files（`+7056 / -379`）。
- 5 个文本冲突文件；冲突后另做高风险自动合并文件审查（`git diff --cc`、`git diff HEAD^1..HEAD`、关键能力 `rg` 扫描），目标是排除静默删除、重复符号、路由中间件丢失、i18n 缺键和 cache usage 覆盖回归。
- 合并前先跑基线（后端 focused、前端 typecheck + Vitest），基线失败则停止合并。

## 2. 冲突与处理

| 文件 | 处理 |
| --- | --- |
| `backend/internal/handler/endpoint.go` | 同时保留本地裸 `/chat/completions` 归一化和上游 alpha search 归一化/直达 endpoint。 |
| `backend/internal/server/routes/gateway.go` | 保留 RequestArchive/RequestIntercept 中间件链，同时加入 `/v1/alpha/search`、`/alpha/search` 与 Codex direct alpha search。 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | 先执行本地 official Chat sanitizer，再执行上游 Grok Responses-only prompt cache key 清理；上游 cache identity / endpoint tracking 保留。 |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 删除与本地 options adapter 重复的上游入口，保留上游 custom/namespace/tool_search 全套 helper 和回程生命周期。 |
| `backend/internal/pkg/apicompat/types.go` | 同时保留本地 streaming output DTO 与上游 `tool_search_call` 自定义 JSON 语义。 |
| 自动合并后续适配 | 把上游 tool conversion / declared-tool choice 接入本地 `ResponsesToChatCompletionsRequestWithOptions`；修复 alpha route 测试 helper 参数和 legacy tool-choice fixture。 |

## 3. 保留的本地能力与语义适配

| 能力 | 状态 |
| --- | --- |
| Prompt Risk / 敏感词过滤与 LLM judge | 保留，管理路由、service、前端 risk-control 入口 |
| RequestArchive / RequestIntercept | 保留，`/v1`、Gemini、根级 Responses/Chat/images/videos、Codex direct 及新增裸 alpha search 均挂中间件 |
| Token Analysis | 保留，admin route、handler/service/repository、前端路由/API |
| 图片生成 | 保留，`/image-gen` 与 `/batch-image` 在 router/sidebar 并存 |
| 用户并发方案 | 保留，preset route/service/runner 与前端 dialog |
| OpenAI compatible cache usage | 保留，raw/buffered/streaming normalization 与 cache write "累加/补缺" 约束 |
| 第三方 Responses→Chat options 过滤 | 保留，上游工具转换接入本地 options 入口而非另起一套 |

## 4. 采用的上游行为

| 领域 | 内容 |
| --- | --- |
| Responses/Chat 工具桥 | custom/freeform、namespace、tool_search 支持 Chat fallback 与流式/非流式回程还原；拒绝工具名歧义和无效 tool_choice |
| Codex | 三路 alpha search；修复 OAuth Messages identity、originator/User-Agent 配对、续链 `item_*` ID |
| Fast/Flex 策略 | 规则新增 `user_ids`，设置页可搜索用户选择器与中英文 key coverage |
| Grok | Free OAuth prompt cache identity、可缓存 Chat→Responses bridge、quota exhausted 恢复、usage snapshot 加固 |
| 可靠性 | compact keepalive nil/lifecycle 守卫；`remote_compaction_v2` 原生 Responses 保留；ops capture writer 释放后 nil 安全 |
| 用量兼容 | Responses/Anthropic 流式与非流式补齐 `cache_creation_input_tokens` |

## 5. 验证

| 命令（`backend` 目录） | 结果 |
| --- | --- |
| 合并前后端 focused 基线（config/routes/handler/service） | 通过 |
| 合并前 `pnpm --dir frontend run typecheck` + `exec vitest run` | 通过，151 files / 956 tests |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/apicompat` | 通过 |
| `go test -tags=unit -p 1 -count=1 ./internal/handler ./internal/server/routes` | 通过 |
| `go test -tags=unit -p 1 -count=1 ./internal/service` | 通过（97.6s） |
| 前端本地/上游功能专项 | 9 files / 46 tests 通过 |
| 未解决文件、冲突标记、`git diff --cached --check` | 提交前通过 |

Windows 定向测试遇到 `.test.exe` `Access is denied` / file-in-use，换全新仓库内 `GOTMPDIR` 后通过，未改业务代码。

## 6. 遗留问题

- 计划中的全量 `go test -tags=unit ./...`、`-tags=integration ./...`、前端 `lint:check` 与全量 Vitest，以及收尾前再次刷新 upstream，在计划中未勾选，台账也只记录了上表的 focused 结果。上游随后前进的部分由同日 0.1.153 固定提交同步接续（`docs/upstream-sync/2026-07-13-v0.1.153/review.md`）。
