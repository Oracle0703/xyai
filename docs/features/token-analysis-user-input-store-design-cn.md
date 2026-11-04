# Token Analysis 用户净输入留存与请求明细页设计

更新：2026-11-04。状态：**已实现并合入 main（2026-06-08，migration 146）；默认开启留存是负责人确认的有意决策（§8）。质量字段仅占位，评估任务未实现（§11、§15）。**

本文是 Token Analysis 用户净输入留存与请求明细页的唯一文档，包含需求、数据模型、提取/脱敏/哈希规则、接口、页面、配置、代码位置、易错点和验证方式。两份代码审查报告（`docs/reviews/token-analysis-user-input-retention-staged-code-review-cn.md`、`docs/reviews/token-analysis-user-input-retention-main-merge-review-cn.md`）已并入本文并移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:<路径>` 查看）；审核中被采纳的结论写在 §13，未采纳的建议以"已否决"写明原因。关联功能：[项目归因](token-analysis-project-attribution-design-cn.md)（同一索引管道与自动索引开关）。

阅读顺序建议：
- 修改提取、脱敏或哈希：读 §4、§5、§13；
- 修改接口或页面：读 §6、§7；
- 评估隐私影响或关闭留存：读 §8；
- 定位代码：读 §12；跑测试：读 §14。

## 1. 背景与目标

公司后续要分析"用户输入是否符合标准"，但评价维度尚未确定。摘要表只存 300 字符脱敏预览（`last_user_preview`），完整请求正文只在归档 JSONL 里——一旦按保留期删除原始文件，输入分析就无法回溯。本功能先把原料和占位落好：

1. 索引时把每次请求的用户净输入全文（脱敏 + 截断）单独入库；
2. 质量字段留空占位，评价标准确定后由评估任务回填；
3. Token 分析页的"可疑请求"表改造为通用**请求明细**表：时间、成员、模型、项目/分支、用量、质量、输入预览一处可看，点行抽屉懒加载全文。

## 2. 维度清单

| 维度 | 来源 | 状态 |
| --- | --- | --- |
| 请求时间 | `token_analysis_request_summaries.event_time` | 既有 |
| 成员 | `user_id` → `users.email` JOIN | 既有 |
| 模型 / 端点 | `model` / `endpoint` | 既有 |
| 仓库 / 分支 | `client_project` / `client_branch`（离线归因） | 既有 |
| 用量 / 费用 | `usage_log_id` → `usage_logs` JOIN | 既有 |
| 输入字符数 | `user_chars` | 既有 |
| 客户端来源 | `attribution_source` | 既有 |
| 风险分 | `risk_score` / `risk_reasons` | 既有 |
| 归档截断标记 | `request_body_truncated`（归档时请求体被截断，归因/全文可能不全） | 既有字段，本功能展示 |
| **用户输入全文** | `token_analysis_user_inputs.content` | 本功能新增 |
| **内容哈希** | `content_sha256`（同一人类输入跨 agent 轮次去重） | 本功能新增 |
| 重复计数 | 当前筛选范围内 `content_sha256` 相同的请求数（窗口函数） | 派生展示 |
| **质量占位** | `quality_score` / `quality_findings` / `quality_version` / `evaluated_at` | 本功能新增（空） |

## 3. 数据模型与迁移（migration 146）

`backend/migrations/146_token_analysis_user_inputs.sql`（受 checksum 校验，不可修改；文件头引用本文路径）新建 `token_analysis_user_inputs`：

| 列 | 说明 |
| --- | --- |
| `id BIGSERIAL PK` | |
| `archive_id TEXT NOT NULL UNIQUE` | 与摘要表同键，幂等 upsert |
| `event_time`、`user_id` | 冗余自摘要，便于独立清理与按人查询 |
| `content TEXT NOT NULL` | 脱敏 + 截断后的最后一条用户净输入 |
| `content_sha256` | 对**脱敏后、未截断**文本计算（§4） |
| `chars`、`truncated` | 留存文本的 rune 数；`truncated` 防止把截断的长输入误判为短输入 |
| `quality_score SMALLINT`、`quality_findings JSONB`、`quality_version TEXT`、`evaluated_at` | 评估占位，索引不写 |
| `created_at`、`updated_at` | |

索引：`(event_time DESC)`、`(user_id, event_time DESC)`、`(content_sha256)`。

独立建表而不是给摘要表加列：大文本不进聚合热表；质量字段与输入同生命周期；后续可独立清理。

## 4. 提取、脱敏与哈希

- 来源：`TokenAnalysisBodySummary.LastUserText`，在生成预览前保留最后一条用户输入的原始全文（messages / responses / gemini / image 各 shape 与 multipart 表单两条解析路径共用 `finalizeTokenAnalysisBodySummary`：在预览脱敏前把原文复制到 `LastUserText`），`json:"-"` 不外发。`InputComplete` 也在该函数中置 true；截断降级摘要不经过它，因此为 false。
- 净输入口径（`tokenAnalysisUserInputText`，与预览同源）：取最后一条含用户本人文本的用户消息；跳过 `tool_result`/`functionResponse` 等非文本块和 assistant/agent 文本；剥离文本开头完整闭合的客户端注入包装（`tokenAnalysisInjectedContextTags`，位于 `token_analysis_origin.go`：`environment_context`、`in-app-browser-context`、`system-reminder`、`skill-context`、`system-update`），剥离后去掉 Codex app 的 `## My request:` 标题；正文中引用、未闭合或位于正文之后的包装保留。没有用户文本时保留更早的用户输入，因此同一 agent 轮次的工具续执行请求共享发起该轮的输入。`UserChars` 另按"用户角色的非工具文本"累计（含注入包装），与净输入口径不同。
- `RedactTokenAnalysisInputText`：去首尾空白、剥 NUL（`sanitizeTokenAnalysisText`），再用与预览相同的 `tokenAnalysisRedactions` 正则替换为 `[redacted]`；**保留换行和缩进**（评估时排版本身是信号）。只脱敏不截断。
- `truncateTokenAnalysisRunes`：按 rune 截断到 `token_analysis.input_store_max_chars`，返回是否截断。`SanitizeTokenAnalysisInputText` = 脱敏 + 截断，供单测与其他调用方使用。
- 哈希：`content_sha256 = hex(SHA-256(脱敏后未截断文本))`。同一输入 → 同一脱敏文本 → 同一哈希，跨 agent 轮次、跨截断配置稳定；且不是 secret 原文的可验证指纹（§13 第 1 条）。

## 5. 索引写入规则

`indexArchiveLine` 在写完请求摘要后，若 `input_store_max_chars > 0`，调用 `storeTokenAnalysisUserInput`：

| 情况 | 行为 |
| --- | --- |
| 有净输入 | `UpsertUserInput`：冲突时更新 `event_time/user_id/content/content_sha256/chars/truncated` |
| 质量字段 | **仅当旧行 `content_sha256` 等于新值时保留**四个 `quality_*`；哈希变化时清空（`quality_version` 置空串），避免旧评分挂到新正文 |
| 净输入为空，且 body 完整解析、未截断 | `DeleteUserInput` 删除旧行（清掉旧版本误把工具输出存成净输入的历史行） |
| 净输入为空，但归档截断或解析降级 | 不动旧行（不能证明"没有正文"） |
| `input_store_max_chars = 0` | 不写不删，已有行保留 |

标准改版重评时，用 `quality_version` 区分是哪一版标准打的分。

## 6. 接口合同

- `GET /api/v1/admin/token-analysis/requests`：`ListRequests` LEFT JOIN `token_analysis_user_inputs`，每项增加 `has_input`、`input_truncated`、`quality_score`、`duplicate_count`；**全文不进列表 payload**。`duplicate_count = COUNT(*) OVER (PARTITION BY COALESCE(ui.content_sha256, s.archive_id))`，在当前筛选集内计算，无留存输入的行恒为 1。
- `GET /api/v1/admin/token-analysis/requests/input?archive_id=`：`TokenAnalysisHandler.RequestInput` → `GetUserInput`，返回 `TokenAnalysisUserInput`（含 `content`、`content_sha256`、`chars`、`truncated`、`quality_*`）；无记录返回 404。
- 两个接口与其他 Token Analysis 接口同在 `registerTokenAnalysisRoutes` 下，受管理端权限控制。

## 7. 页面

`frontend/src/views/admin/TokenAnalysisView.vue`，明细拆分在 `frontend/src/components/admin/token-analysis/`：

- "可疑请求"改为"请求明细"（`TokenAnalysisRequestTable.vue`）：列为时间 / 成员 / 模型 / 项目+分支 / 用量 / 质量 / 预览，风险徽标并入预览列；排序可切换按时间（默认）/ 按风险；默认 `risk_min` 由 20 改为 0（明细默认展示全部请求，排查风险时手动调高）。
- 预览旁 `×N` 重复计数徽标（N>1 才显示）。选计数徽标而非折叠分组：不改行结构、零风险；折叠为后续可选项。
- `request_body_truncated` 为真时主表与抽屉显示"归档截断"标记（数据可信度信号）。
- 抽屉（`TokenAnalysisRequestDrawer.vue`）：点行时 `selectRequest` 懒加载全文（`has_input=false` 不请求）；"用户输入全文"区块 pre-wrap 展示并给出截断提示，加载失败回退显示预览；"质量"区块未评估时显示占位；另展示 Cache creation、内容哈希、字符数。内容哈希沿用脱敏后指纹语义，不暴露原文。
- 概览卡片：总请求数 / 输入 Token / 输出 Token / 总 Token。筛选：增加 Account ID / Group ID（后端早已支持）。
- 归档文件卡片提示"仅列出当前生效归档目录，切换后旧目录文件需手动迁移"；项目筛选框占位写明"项目名（精确匹配） / unattributed"（zh/en 同步）。

## 8. 配置与默认留存决策

| 配置 | 默认 | 说明 |
| --- | --- | --- |
| `token_analysis.input_store_max_chars` | 8000 | 0 = 不留存全文；负数启动校验失败 |
| `token_analysis.index_enabled` | true | false 时不启动自动索引 |
| `token_analysis.auto_index_interval_seconds` | 300 | 0 = 仅手动索引；负数校验失败 |
| `gateway.request_archive.enabled` | — | 归档是数据源；开启后默认自动索引会把净输入自动落库 |

默认值在 `backend/internal/config/config.go`，`deploy/config.example.yaml` 有完整 `token_analysis` 段，写明长期留存敏感数据的影响和关闭方式；改默认值时两处同步。

**默认开启是有意决策**（负责人确认）：本 fork 是公司内部部署，"标准未定先存原料"是功能需求本身，默认 0 等于默认关闭功能。面向通用部署的 fork 应自行评估改 0。已否决：把 `input_store_max_chars` 默认改为 0 或新增显式 opt-in 开关（两次审核均建议），原因同上；隐私边界改为在示例配置、本文和 llm-wiki 三处标注。

开启 `gateway.request_archive.enabled` 前运维需确认：

1. **保留期**：JSONL 清理周期已排定（全文已入库，原文窗口仅供排障）；
2. **DB 访问权限**：`token_analysis_user_inputs` 含员工输入全文（已脱敏），管理端与 DB 直连权限按敏感表对待；
3. **备份与清理**：DB 备份覆盖该表；历史行清理（如按年龄删除）与质量评估回填周期不冲突；
4. **关闭方式**：`auto_index_interval_seconds: 0` 停自动索引，`input_store_max_chars: 0` 停全文留存；已入库行不受影响，需手动清理。

## 9. 与既有 prompt metrics 的关系

仓库另有在线采集链路 `user_prompt_events` + `user_prompt_analysis`（migration 143，`backend/internal/service/promptmetrics/`，管理页"Prompt 指标"）：网关请求时落库 prompt 摘要/哈希，本地规则打 clarity/context/quality 分。

| | `token_analysis_user_inputs`（本功能） | `user_prompt_events`（prompt metrics） |
| --- | --- | --- |
| 采集 | 离线索引归档 JSONL，`archive_id` 幂等可重放 | 在线网关路径 |
| 全文 | 默认存（脱敏 + 8000 截断） | `store_full_text` 默认关，只存摘要 |
| 用量 | `usage_logs` JOIN 真实 tokens/cost | 估算 tokens |
| 项目/分支 | 离线归因 | 在线提取 |
| 质量 | 占位待标准 | 本地规则版已有 |

后续若统一，可经 `content_sha256` / `prompt_hash` 对齐两表，把 prompt metrics 的规则分回填进 `quality_*`，或反向把净输入全文供给 prompt metrics 的 LLM 分析；本功能不合并两者。

## 10. 与 JSONL 保留策略的关系

净输入全文入库后，原始 JSONL 可按保留期（建议 7–14 天）gzip + 删除，输入合规分析不再依赖原文窗口。体量预估（2026-06 流量）：1.1 万请求/天 × 平均几 KB ≈ 30–50 MB/天；agent 轮次重发的同一输入可经 `content_sha256` 去重统计。

## 11. 质量评估回填约定（未实现）

评估任务（规则或 LLM）读取 `quality_score IS NULL`（或 `quality_version` 低于当前标准版本）的行，写回 `quality_score`（SMALLINT）、`quality_findings`（jsonb，维度明细）、`quality_version`、`evaluated_at`。列表与抽屉已渲染这些字段，回填即显示，前端无需再改。评估结果绑定 `content_sha256`：内容变化的重索引会清空评分（§5），评估任务应能重新拾取这些行。

## 12. 代码地图

| 位置 | 内容 |
| --- | --- |
| `backend/internal/service/token_analysis_summary.go` | `finalizeTokenAnalysisBodySummary`、`tokenAnalysisUserInputText`、`tokenAnalysisUserAuthoredText`、`RedactTokenAnalysisInputText`、`SanitizeTokenAnalysisInputText`、`truncateTokenAnalysisRunes` |
| `backend/internal/service/token_analysis_origin.go` | `tokenAnalysisInjectedContextTags`（净输入剥离的注入包装标签） |
| `backend/internal/service/token_analysis_types.go` | `TokenAnalysisBodySummary.LastUserText/InputComplete`、`TokenAnalysisUserInput`、`TokenAnalysisRequestItem.HasInput/InputTruncated/QualityScore/DuplicateCount`、仓储接口 `UpsertUserInput`/`DeleteUserInput`/`GetUserInput` |
| `backend/internal/service/token_analysis_indexer.go` | `indexArchiveFile`（循环顶部 `ctx.Err()` 检查）、`indexArchiveLine`、`storeTokenAnalysisUserInput`、`tokenAnalysisInputStoreMaxChars`、`archiveDir` |
| `backend/internal/service/token_analysis_auto_index.go` | `StartAutoIndex`、`startAutoIndexWithInterval`、`StopAutoIndex`（`lifecycleCancel` 后 `backgroundWG.Wait`）、`autoIndexOnce` |
| `backend/internal/service/token_analysis_service.go` | `GetUserInput`（无记录转 not found） |
| `backend/internal/repository/token_analysis_repo.go` | `UpsertUserInput`（质量字段按哈希保留）、`DeleteUserInput`、`GetUserInput`、`ListRequests`（LEFT JOIN + `duplicate_count`） |
| `backend/internal/handler/admin/token_analysis_handler.go` | `Requests`、`RequestInput` |
| `backend/internal/server/routes/admin.go` | `registerTokenAnalysisRoutes` |
| `backend/internal/config/config.go`、`deploy/config.example.yaml` | `TokenAnalysis.InputStoreMaxChars` 等默认值与校验；示例配置段 |
| `backend/migrations/146_token_analysis_user_inputs.sql` | §3 |
| `frontend/src/api/admin/tokenAnalysis.ts` | `getRequestInput` |
| `frontend/src/views/admin/TokenAnalysisView.vue` | `selectRequest`、`requestInputLoading` |
| `frontend/src/components/admin/token-analysis/TokenAnalysisRequestTable.vue`、`TokenAnalysisRequestDrawer.vue` | 明细表与抽屉 |

## 13. 关键约束与易错点

1. **规则：`content_sha256` 必须对 `RedactTokenAnalysisInputText` 的输出（脱敏后、未截断）计算，不能对 `LastUserText` 原文计算。** 依据：最初实现对原文做 SHA-256，用户输入里的 `sk-...`、Bearer token 虽然在 `content` 中被脱敏，但拿到候选 secret 的人可以离线比对哈希确认它是否出现过；该列长期保存且有索引。回归测试 `TestTokenAnalysisIndexerUserInputHashNotRawFingerprint` 断言含 secret 输入的哈希 ≠ 原文 SHA-256 且 = 脱敏文本 SHA-256。已否决：服务端密钥 HMAC，原因是密钥管理成本高于收益，脱敏后哈希已消除指纹风险。
2. **规则：`StopAutoIndex` 先取消 `lifecycleCtx` 再等待；`indexArchiveFile` 的逐行循环顶部必须检查 `ctx.Err()`。** 依据：最初 Stop 只关 stop channel，每轮用独立的 10 分钟超时 context，停机可被大文件拖住；第一次修复改为可取消的 base context 后仍不彻底——空行、无效 JSON、非 request 事件不经过任何 repo 调用，取消后照样扫完整个文件并返回成功。回归测试：`TestTokenAnalysisAutoIndexStopCancelsRunningRound`（repo 阻塞时 Stop 3 秒内返回）、`TestTokenAnalysisIndexerStopsScanningSkippedLinesAfterCancel`（1 条 request + 5000 条跳过行，首条落库后取消，`IndexRange` 返回 `context.Canceled` 且只有 1 次 upsert）。取消导致的报错不告警。
3. **现状（历史实现，待整改）：索引器对 `UpdateIndexState` 的返回值用 `_ =` 忽略，水位保存失败不会中断本轮。** 2026-06-08 合并前复核时曾按"状态是辅助进度信息"维持该语义；但 2026-11-02 磁盘故障证明水位与实际落库不一致会让失败行无法重放，已在 [Token Analysis 基础设计](token-analysis-design-cn.md) §14 定为 P1 改造："水位保存失败停止本轮，可靠 checkpoint 不越过未持久化行"（尚未实施）。修改索引器时以 §14 为准，不要把忽略返回值当作需要保持的约束。
4. **规则：抽屉懒加载的 `.then()` 和 `.finally()` 都要校验 `selectedRequest.archive_id === item.archive_id`。** 依据：`requestInputLoading` 是全局布尔，曾复现连续点击 A→B 时 A 的旧请求 `finally` 提前关掉 B 的 loading、短暂显示错误内容。回归测试在 `TokenAnalysisView.spec.ts`（"keeps loading state for the newly opened request when an older input request resolves late"）。
5. **规则：质量字段只在 `content_sha256` 不变时随重索引保留。** 依据：重索引可能因提取口径修正而改变净输入，旧评分若保留会挂到新正文上（`TestTokenAnalysisRepositoryUpsertUserInputPreservesQuality`、集成测试 `TestTokenAnalysisUserInputQualityFollowsContentHash`）。
6. **规则：只有完整解析、未截断且确认无用户正文时才删除旧净输入。** 依据：旧版本曾把只有 `tool_result` 的请求的工具输出存成净输入，需要重索引清理；但截断或降级解析不能证明"没有正文"，删了会丢真实输入（`TestTokenAnalysisIndexerReindexClearsStaleUserInputOnlyWhenConfirmedEmpty`）。
7. **规则：列表接口不返回全文，只返回 `has_input`/`input_truncated`/`quality_score`/`duplicate_count`。** 依据：全文最长 8000 字符，列表分页一次几十行，放进 payload 会显著放大响应并扩大敏感数据暴露面。
8. **规则：索引只读当前生效的归档目录。** 依据：归档目录可在后台热切换，`indexRange` 每轮只取一次 `archiveDir`；切换后旧目录的历史文件不会被自动纳入。页面已提示需手动迁移。已否决：为手动索引增加 source dir 参数或按目录变更历史读多个目录，原因是从简，产品口径为手动迁移。
9. **规则：示例配置必须包含 `token_analysis` 段并写明隐私影响与关闭方式。** 依据：最初示例配置没有该段，运维无从发现默认会每 5 分钟自动索引并留存 8000 字符净输入。
10. **规则：一个功能只保留一份文档。** 依据：合入前曾因两份审查报告并存、旧报告结论过期（还触发 `git diff --check` 的 EOF 空行失败）而误导审核；现已全部并入本文。

## 14. 验证

### 14.1 命令

```bash
cd backend
# 提取、脱敏、留存、哈希、取消
go test -tags=unit -count=1 ./internal/service -run "TokenAnalysis|SanitizeTokenAnalysisInputText"
go test -tags=unit -count=1 ./internal/repository -run TokenAnalysis
go test -tags=unit -count=1 ./internal/handler/admin -run TokenAnalysis
go test -tags=unit -count=1 ./cmd/server -run TestProvideCleanup_WithMinimalDependencies_NoPanic
# 真实 PostgreSQL（integration 标签）：质量字段随哈希保留/清空
go test -tags=integration -count=1 ./internal/repository -run TestTokenAnalysisUserInputQualityFollowsContentHash
# 前端
cd ../frontend
npm run typecheck
npx vitest run src/api/__tests__/admin.tokenAnalysis.spec.ts src/views/admin/__tests__/TokenAnalysisView.spec.ts
```

Windows 上 Go 测试偶发 build 缓存或临时 `.test.exe` 被占用，换一个新的 `GOTMPDIR` 后重跑即可。

端到端手工验证：触发索引 → `token_analysis_user_inputs` 出现行 → 页面明细表新列 → 点行抽屉显示全文；手工设置质量字段 → 同哈希重索引保留、换哈希重索引清空；完整解析确认无正文时旧净输入被删除。

### 14.2 最近结果

合入 main 前复核（2026-06-08）：service、repository、handler/admin、`cmd/server` cleanup、`internal/pkg/diskspace` 的 unit 测试通过；`npm run typecheck` 通过；上述 vitest 2 个文件通过（当时 8 个用例，之后补了第 4 条的竞态用例）。`./internal/server/middleware -run RequestArchive` 因 Windows 临时 `.test.exe` 文件锁连续两次未跑完，未取得结果。`git diff --check` 的 EOF 空行问题已修复。此后未单独针对本功能重跑。

## 15. 状态与待办

- [x] 净输入留存、哈希去重、质量占位、请求明细页与抽屉（2026-06-08 合入 main）。
- [x] 审核问题全部处理：哈希改脱敏后计算、停机可取消、抽屉竞态、示例配置段、目录与精确匹配提示（§13）。
- [ ] 确定输入质量评价标准并实现评估回填任务（§11）。
- [ ] 历史行清理策略（按年龄删除等）与 DB 访问权限落实（§8 checklist 第 2、3 条），需运维与负责人确认。
- [ ] 可选：明细表按 `content_sha256` 折叠分组；与 prompt metrics 对齐（§9）。
