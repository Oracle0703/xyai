# 合并前代码审查报告: Token Analysis 用户输入留存分析

审查时间: 2026-06-08

审查对象: 当前分支 `feature/hy/0607_用户输入留存分析` 相对本地 `main` 的差异, 范围为 `main...HEAD`。当前 HEAD 为 `ace14582 fix(token-analysis): address codex review of user-input retention branch`。

## 总体结论

不建议无处理直接合并到 `main`。上一轮审查中的多项问题已经被后续提交修复或补充说明, 例如 `content_sha256` 已改为对脱敏后未截断文本计算, `deploy/config.example.yaml` 已补齐 `token_analysis` 配置段, 前端抽屉 loading 竞态代码也已加 archive_id 校验。但本轮合并前仍有几类风险:

| 级别 | 结论 |
| --- | --- |
| 重要 | 自动索引的停机取消仍不完整: `StopAutoIndex` 虽然 cancel 了 context, 但文件扫描循环不检查 `ctx.Err()`, 大文件/大量无效行仍可能继续扫完整个文件。 |
| 重要 | 用户净输入全文长期留存默认开启仍是合规风险; 文档已提示, 但合并到 `main` 前需要明确这是有意默认, 或改成显式 opt-in。 |
| 中等 | 分支内历史审查文档已经过期且 `git diff --check` 失败, 直接合并会把误导性审查结论和空白行问题带进主干。 |
| 中等 | 项目归因设计文档的运行前提仍描述为手动排班索引, 与当前服务启动自动索引实现不一致。 |
| 轻微 | 前端已有竞态修复代码, 但缺少针对连续点击两条请求时旧请求 finally 不影响新 loading 状态的单测。 |

## 主要问题

| 严重级别 | 问题 | 证据 | 改进建议 |
| --- | --- | --- | --- |
| 重要 | `StopAutoIndex` 的“取消进行中轮次”修复不彻底。当前 `StopAutoIndex` 会调用 `autoIndexCancel()` 并等待 goroutine 退出, 但 `indexArchiveFile` 的逐行读取循环没有在循环顶部或读行后检查 `ctx.Err()`。如果归档文件很大、包含大量空行/无效 JSON/跳过行, 或 repo 调用很快返回 `context canceled` 但循环继续, 停机仍会继续扫完整个文件。 | `backend/internal/service/token_analysis_auto_index.go:51-61` cancel 后直接 `Wait()`; `backend/internal/service/token_analysis_indexer.go:171-214` 的 `for` 循环只处理 `readErr`, 没有 `ctx.Err()` 检查; `UpdateIndexState` 调用错误也被 `_ =` 忽略。现有测试 `TestTokenAnalysisAutoIndexStopCancelsRunningRound` 只覆盖 repo 阻塞在 `FindNearestUsageLog` 并尊重 ctx 的路径, 没覆盖大文件/大量跳过行。 | 在 `indexArchiveFile` 循环顶部和每次 `ReadBytes` 后加入 `if err := ctx.Err(); err != nil { return nil, err }`; 对 `UpdateIndexState` 是否应在 cancel 时忽略、其他错误时返回做明确分支。补一个测试: 构造很多无效/空行或让 repo 快速返回 ctx canceled, 调用 `StopAutoIndex` 后应在短时间内退出。 |
| 重要 | 用户净输入全文长期留存默认开启, 合并到 `main` 前仍需产品/合规确认。当前只要 `request_archive` 开启, 默认自动索引每 5 分钟运行, 并把脱敏后用户净输入最多 8000 字符长期写入 DB。虽然配置示例和 wiki 已提示, 但默认开启意味着一次排障开关可能把敏感输入从短期 JSONL 变成长期数据库资产。 | `backend/internal/config/config.go:1830-1835` 默认 `index_enabled=true`, `auto_index_interval_seconds=300`, `input_store_max_chars=8000`; `deploy/config.example.yaml:747-761` 已写明长期留存敏感数据; `llm-wiki/wiki/backend.md:104-105` 也记录默认 8000 和启动即自动索引。 | 若 `main` 面向通用部署, 建议把 `input_store_max_chars` 默认改为 `0`, 或增加 `token_analysis.input_store_enabled` 之类显式开关。若这是内部 fork 的强需求默认, 建议在合并说明中要求负责人确认, 并补运维 checklist: 开启 request_archive 前确认保留期、DB 访问权限、备份清理策略和关闭方式。 |
| 中等 | 历史审查报告会误导后续审核, 且当前 `git diff --check` 已失败。分支中 `docs/features/token-analysis-user-input-retention-staged-code-review-cn.md` 仍保留旧结论“审查对象相对 origin/main”“DNS 无法解析 GitHub”“不建议直接合并, 有 3 个风险”等内容, 但当前分支已新增修复提交且 GitHub 可访问验证结果已变化。文件末尾还触发 `new blank line at EOF`。 | `docs/features/token-analysis-user-input-retention-staged-code-review-cn.md:5-16` 是旧远端/旧基线说明; `:20-24` 仍保留旧的不建议合并主结论; `git diff --check main...HEAD` 输出 `docs/features/token-analysis-user-input-retention-staged-code-review-cn.md:80: new blank line at EOF`。 | 合并前要么用本报告替换旧报告, 要么把旧报告明确标注为“历史审查记录, 已被后续修复覆盖”, 并删除末尾多余空行。建议最终只保留一份当前合并前报告, 避免审核人读到互相冲突的结论。 |
| 中等 | 项目归因设计文档的运行前提与实现不一致。设计文档仍说 token_analysis 按管理端 `POST /token-analysis/index` 触发, 建议每日排班索引前一天文件; 当前实现已经在服务启动时自动每 5 分钟索引 `[昨天, 今天]`。 | `docs/features/token-analysis-project-attribution-design-cn.md:150` 描述手动触发/每日排班; `backend/internal/service/token_analysis_auto_index.go:15-19` 和 `backend/internal/service/wire.go` 当前实现启动自动索引。 | 更新设计文档“运行前提/验证计划”: 默认自动索引, `auto_index_interval_seconds=0` 才仅手动; 手动接口用于补历史日期。否则运维会误以为需要外部 cron, 或忽略自动索引的敏感数据落库行为。 |
| 轻微 | 前端 loading 竞态代码已修, 但缺少回归测试。当前实现的 `.finally()` 已按 `selectedRequest.archive_id` 校验, 逻辑方向正确; 但现有测试只覆盖单次点击懒加载全文, 不覆盖连续点击 A/B 时 A 的旧 promise finally 不关闭 B 的 loading。 | `frontend/src/views/admin/TokenAnalysisView.vue:581-602` 有竞态防护; `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts:153-166` 只断言单条请求点击和全文显示。 | 补一个 vitest: A 的 `getRequestInput` 延迟, 点击 A 后立即点击 B, resolve A 时 B 仍保持 loading 或正确显示 B 的状态; 再 resolve B 后显示 B 全文。 |

## 已修复或基本可接受项

| 项目 | 当前判断 | 证据 |
| --- | --- | --- |
| `content_sha256` 原文指纹 | 已修复。现在先 `RedactTokenAnalysisInputText`, 再对脱敏后未截断文本算 SHA256。 | `backend/internal/service/token_analysis_indexer.go:324-333`; `docs/features/token-analysis-user-input-store-design-cn.md:29` |
| 前端抽屉旧请求 finally 竞态 | 代码已修, 但建议补测试。 | `frontend/src/views/admin/TokenAnalysisView.vue:596-601` |
| 归档目录热切换后的旧目录边界 | 已在 Settings 和 TokenAnalysis 文件卡片文案提示“旧目录需手动迁移”, 属产品取舍。 | `frontend/src/i18n/locales/zh.ts:1497`; `llm-wiki/wiki/backend.md:102` |
| Project 精确匹配说明 | 已补占位说明。 | `frontend/src/i18n/locales/zh.ts:1464` |
| `deploy/config.example.yaml` 缺 token_analysis 段 | 已补完整配置段和敏感数据提示。 | `deploy/config.example.yaml:747-765` |

## 验证结果

| 命令 | 结果 |
| --- | --- |
| `go test -tags=unit -p 1 -count=1 ./internal/service -run "TokenAnalysis\|ProjectAttribution\|RequestArchive"` | 首次被 Windows Go build 缓存权限问题拦截; 使用 fresh `GOTMPDIR` 后通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/repository -run TokenAnalysis` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/handler/admin -run TokenAnalysis` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./cmd/server -run TestProvideCleanup_WithMinimalDependencies_NoPanic` | 通过。 |
| `go test -tags=unit -p 1 -count=1 ./internal/pkg/diskspace` | 通过。 |
| `cmd.exe /c npm run typecheck` | 通过。 |
| `cmd.exe /c npx vitest run src/api/__tests__/admin.tokenAnalysis.spec.ts src/views/admin/__tests__/TokenAnalysisView.spec.ts` | 通过, 2 个文件 8 个测试。 |
| `go test -tags=unit -p 1 -count=1 ./internal/server/middleware -run RequestArchive` | 未完成验证: 连续两次被 Windows 临时 `.test.exe` 文件锁拦截, 错误为 `The process cannot access the file because it is being used by another process`。 |
| `git diff --check main...HEAD` | 未通过: `docs/features/token-analysis-user-input-retention-staged-code-review-cn.md:80: new blank line at EOF`。 |

## 合并前建议清单

| 优先级 | 建议 |
| --- | --- |
| P0 | 在 `indexArchiveFile` 循环中尊重 `ctx.Err()`, 让 `StopAutoIndex` 真正能快速取消大文件扫描。 |
| P0 | 决策 `input_store_max_chars=8000` 是否允许作为 `main` 默认值; 若允许, 在合并说明里明确合规负责人和运维清理策略。 |
| P1 | 清理或更新旧审查报告, 并修复 `git diff --check` 空白行失败。 |
| P1 | 更新 `token-analysis-project-attribution-design-cn.md` 的自动索引运行前提。 |
| P2 | 补前端连续点击请求明细的 loading 竞态回归测试。 |

## 处理结果(2026-06-08, Claude)

| 级别 | 问题 | 处理 |
| --- | --- | --- |
| 重要1 | `indexArchiveFile` 循环不检查 `ctx.Err()`, 跳过行路径不经过 repo 调用, 停机仍扫完大文件 | **已采纳修复**: 逐行循环顶部加 `ctx.Err()` 检查(`token_analysis_indexer.go`)。新增回归测试 `TestTokenAnalysisIndexerStopsScanningSkippedLinesAfterCancel`: 1 条 request + 5000 条跳过行, 首条落库后取消, `IndexRange` 返回 `context.Canceled` 且仅 1 条 upsert(修复前会扫完全文件并返回成功)。`UpdateIndexState` 维持 `_ =` 尽力而为语义不改: 状态记录是辅助进度信息, 其失败不应中断索引、也不应覆盖真正的索引错误 |
| 重要2 | 默认留存 8000 字符需负责人确认 | **决策确认, 维持默认开启**: 上轮审查已明确不采纳"默认改 0"(本 fork 内部部署, 先存原料是特性需求本身), 本轮按建议把决策记录与运维 checklist(保留期/DB 权限/备份清理/关闭方式)落进 `token-analysis-user-input-store-design-cn.md` |
| 中等3 | 旧审查报告结论过期 + `git diff --check` EOF 空行 | **已修复**: 旧报告顶部加"历史审查记录"标注(指明 3 风险已全部修复、最新结论以本报告为准), 删除 EOF 多余空行, `git diff --check` 通过。两份报告并存但结论链路清晰, 不再互相冲突 |
| 中等4 | 归因设计文档运行前提仍写手动排班 | **已修复**: "运行前提"更新为默认自动索引(无需外部 cron), `auto_index_interval_seconds: 0` / `index_enabled: false` 退化为仅手动, 手动接口用于补历史日期, 并提示自动索引的敏感数据落库影响 |
| 轻微5 | 缺前端连续点击 loading 竞态回归测试 | **已补**: vitest 用例连续点击 A→B, A 的旧 promise 晚到时 B 抽屉保持 loading 且不显示 A 的全文, B 返回后正常展示 |
