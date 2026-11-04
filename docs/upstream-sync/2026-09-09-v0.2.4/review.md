# Sub2API 0.2.4 上游同步记录（固定提交 + PR #6924）

合并日期 2026-09-09（PR #6924 叠加 2026-09-10）｜ 工作分支 `feature/hy/10204_merge_sub2api_204`（自本地 `main@f6bce5db1cda145f838bc8fa67b7c8dd90f6b0bd` 创建，与 `github/main` 一致）｜ 上游 `Wei-Shaw/sub2api main@98d86915becae9fe9491a91ffc6defd5235c8d2b`（VERSION 0.2.4，merge base `270eac697304`）；叠加 PR #6924 head `4e5632c3e32f9a8a5150c44e8a7f46efe7fc2688` ｜ 合并提交 0.2.4 = `2398cc00df1eb15876e05504131775af13ba374e`（父 `f6bce5db1` + `98d86915b`）；PR #6924 = `6e9b68ce10ae0e34715c12c63383f0c06a9ed6df`（父 `2398cc00d` + `4e5632c3e`）｜ 状态：两次合并均在 main 第一父历史上

本文是该轮同步的唯一记录。原始材料 `requirements.md`、`specs.md`、`plan.md`、`test-review.md`、`delivery-status.md`、`delivery-report.md`（本目录）已并入本文并移出仓库，原文最后存在于提交 `26f6363af`（`git show 26f6363af:<路径>` 可查）。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-09-09 main sync (v0.2.4; awaiting review)` 与 `2026-09-10 PR #6924 supplement (awaiting review)` 两节（"待审核"是提交前快照，实际合并提交见上）。

## 1. 范围

**0.2.4**：上游 3 commits、19 paths（`+336 / -10`），聚焦 GPT Image 2.5 与 OAuth 生图主控模型。预演与实际合并均 0 文本冲突；2 个双方修改路径；17 个仅上游路径与固定提交逐 blob 一致。提交前 staged 34 files（`+755 / -27`）。

**PR #6924**（`fix: validate Codex User-Agent before identity parsing`）：用户指定在同一分支叠加。PR 当时 OPEN / MERGEABLE，GitHub 只有 CLA check SUCCESS，没有完整后端 CI；PR base 精确等于已合入的 `98d86915b`。只合入 PR head，不用可能夹带更新主线的临时 merge ref `df3a1b1f7f49`。三方路径集合 local 520、PR 4、both 0；0 文本冲突，4/4 结果 blob 与 PR head 一致。提交前 staged 17 files（`+176 / -24`）。

约束：只解决冲突与合并必需的适配，不修目标中的上游缺陷；24 个 tracked `docs/features/` 文件必须保留；重叠能力采用上游；提交前停下等待用户审核。

## 2. 冲突与处理

无文本冲突。双方修改路径：

| 文件 | 处理 |
| --- | --- |
| `README_CN.md` | 同时保留本地部署/赞助说明与上游图片模型章节 |
| `backend/internal/service/openai_codex_transform.go` | 同时保留本地 reasoning helper 与上游生图主控模型函数 |

本轮无 Ent schema、migration、Wire source 或生命周期变化，按 wiki 规则无需重新生成。

PR #6924 改动路径：`backend/internal/pkg/openai/request.go`、`backend/internal/pkg/openai/request_ua_validation_test.go`、`backend/internal/service/openai_codex_identity.go`、`backend/internal/service/openai_ua_validation_test.go`，均原样采用。

## 3. 保留的本地能力与语义适配

24/24 `docs/features/` 文件保留、零删除。本地网关、审计、用量、权限与后台能力（RequestArchive/RequestIntercept、Prompt Metrics/Risk、Token Analysis、组织用量、子管理员、并发 preset、compatible cache usage、默认 reasoning effort、quota flusher、reasoning-only failover）未被本轮 19 个路径或 PR 的 4 个路径触碰。

## 4. 采用的上游行为

| 项目 | 合同 |
| --- | --- |
| GPT Image 2.5 | 支持 `gpt-image-2.5-flare`、`gpt-image-2.5-sunburst` 及 `-2026-09-08` 日期快照；内置价格仅在显式价格缺失时回落；进入模型白名单候选，但显式账号映射与分组白名单不自动扩大权限 |
| OAuth/Setup Token 生图主控模型 | Responses 主控模型默认 `gpt-5.6-luna`；可选 `SUB2API_IMAGES_MAIN_MODEL`（trim 后非空才覆盖），Compose 变体同步透传；`reqBody.model` 用主控模型，`tools[image_generation].model` 保留客户端选的图片模型；已带合法文本主控模型的 Responses 请求不被覆盖 |
| 生图 usage | 保留文本输入、图片输入、图片输出 token 明细 |
| PR #6924 | `PairCodexClientIdentity` 在 trim/identity 解析前用 `httpguts.ValidHeaderFieldValue` 校验原始 User-Agent 并显式拒绝 CR/LF；NUL、DEL、控制字节或换行都不能贡献 originator/version/suffix；非法 canonical resolver 或 candidate 统一回退内置官方 UA/版本。transparent version policy、随机 UA、安装/session ID、配置、依赖及 HTTP/WS 共用 identity 结构不变 |

## 5. 验证

### 0.2.4

| 命令 / 检查（`backend` 用任务专用 `GOCACHE`/`GOTMPDIR`） | 结果 |
| --- | --- |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/openai ./internal/service -run 'GPTImage25|OpenAIImage|Images|ImageModel|Pricing'` | 通过 |
| `go test -p 1 -count=1 ./...` | 除 repository 3 个 `backup_pg_dumper` 用例（Windows PATH 无 `sh.exe`）外通过 |
| `go test -tags=unit -p 1 -count=1 ./...` | 除同 3 个 `sh.exe` 用例和第一父 `/auth/me` golden 差异外通过；排除这 4 个后 default repository、unit repository、unit server 均通过 |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过 |
| `go build ./...`、`go build -tags=embed ./...` | 通过 |
| `go mod tidy -diff` | 非零，仅第一父已有 `go.sum` 旧校验和清理与 `xxh3` 补齐差异，未改文件 |
| `golangci-lint run --new-from-rev=HEAD ./...` | v2.13.0 为 0 issues；PATH 中的 v2.9.0 用 Go 1.26 构建，无法分析 Go 1.27 代码，改用 Go 1.27 构建的版本 |
| 前端 lint、typecheck | 通过 |
| `useModelWhitelist.spec.ts` | 1 file / 16 tests 通过 |
| 完整 Vitest | 293/296 files、2161/2165 tests；4 个失败（Channel Monitor 旧数量断言 1 个，Groups/Subscriptions 测试未安装 Pinia 3 个）都在未被 0.2.4 修改的第一父测试文件中 |
| 前端 build | 通过，i18n 3/3，1079 modules |
| Wiki 图谱 | 33 nodes / 67 edges、49 wikilinks、0 unresolved，`READY` |
| Git 审计 | 0 unstaged/untracked/unmerged、0 冲突标记，cached diff check 通过 |

### PR #6924

| 检查 | 结果 |
| --- | --- |
| UA / identity 精确过滤测试（`internal/pkg/openai`、`internal/service`） | 通过 |
| 受影响包 default/unit | `internal/pkg/openai` 通过；一次未排除用例的 service 全包运行触发第一父既有 cyber-policy 1 秒时序波动，全新 `GOTMPDIR` 精确复跑通过，排除该用例后完整 service 通过 |
| `go build ./...`、`go build -tags=embed ./...`、golangci-lint v2.13.0 | 通过 / 0 issues |
| `go mod tidy -diff` | 只复现 0.2.4 基线 `go.sum` 差异；PR 不改 go.mod/go.sum |
| Wiki 图谱 | `READY` |

## 6. 遗留问题

未修复（均不属于本轮冲突范围）：3 个 Windows 缺 `sh.exe` 的 `backup_pg_dumper` 测试、第一父 `/api/v1/auth/me` golden 未同步 `admin_permissions:null`、4 个第一父前端测试问题、基线 `go.sum` tidy 差异、service 中 cyber-policy 1 秒时序用例的偶发波动。
