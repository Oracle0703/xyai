# 代码审查报告: Token Analysis 用户输入留存分析

审查时间: 2026-06-08

审查对象: 当前本地分支 `feature/hy/0607_用户输入留存分析` 相对 `origin/main` 的差异。

> 说明: 本轮最初按“缓存区”检查时 `git diff --cached` 为空。用户补充“已经 git push 了”后, 改按本地已提交分支 diff (`origin/main...HEAD`) 审查。当前本地分支没有 upstream; 远端存在性确认结果见“远端确认”。

## 远端确认

| 目标 | 命令 | 结果 |
| --- | --- | --- |
| 本地分支 | `git status --short --branch` | `## feature/hy/0607_用户输入留存分析`; 另有本报告文件未暂存。 |
| 本地提交链 | `git log --oneline --reverse origin/main..HEAD` | 8 个提交, 从 `cbb781c0 feat(gateway): async request_archive...` 到 `e3ad2fa7 feat(token-analysis): auto-index...`。 |
| `origin` 同名分支 | `git ls-remote --heads origin "feature/hy/0607_用户输入留存分析"` | 命令成功但无输出, 未发现同名分支引用。 |
| `github` 同名分支 | `git ls-remote --heads github "feature/hy/0607_用户输入留存分析"` | 当前机器 DNS 无法解析 `github.com`, 未能验证。 |

## 总体结论

不建议直接合并。功能主链路能编译并通过目标测试: request archive 异步写入、响应 usage 瘦身、归档目录热切换、项目归因、用户净输入留存、Token Analysis 页面明细和自动索引都有实现和测试。但有 3 个需要合并前处理的风险:

1. `content_sha256` 对脱敏前原文计算, 会把 secret/token 的可验证指纹长期入库。
2. 用户输入全文留存和自动索引默认开启, 示例配置没有对应 `token_analysis` 配置段, 运维不容易发现和关闭。
3. 自动索引停机不可取消正在执行的索引轮次, 可能拖慢优雅退出。

## 问题清单

| 严重级别 | 问题 | 证据 | 改进建议 |
| --- | --- | --- | --- |
| 重要 | `content_sha256` 对脱敏前原文计算, 敏感输入会留下可验证哈希指纹。用户输入 `sk-...`、Bearer token、内部路径或私密文本虽然 `content` 会脱敏/截断, 但 hash 是原始文本的 SHA256, 拿到候选 secret 的人可离线比对确认是否出现过。 | `backend/internal/service/token_analysis_indexer.go:324-333` 先调用 `SanitizeTokenAnalysisInputText` 得到 `content`, 但随后 `rawSum := sha256.Sum256([]byte(strings.TrimSpace(bodySummary.LastUserText)))`; 设计文档 `docs/features/token-analysis-user-input-store-design-cn.md:29` 明确写“content_sha256 对脱敏前原文计算”; migration `backend/migrations/146_token_analysis_user_inputs.sql:11` 长期保存并建索引。 | 改成对脱敏后的 `content` 或规范化后的安全摘要计算 hash。若确实需要跨截断稳定去重, 应使用服务端密钥 HMAC 或只对脱敏后未截断规范文本计算, 并在文档说明 hash 不是原文可验证指纹。补测试: 输入含 secret 时, `content_sha256` 不等于原文 SHA256。 |
| 重要 | 用户输入全文留存与自动索引默认开启, 但示例配置没有 `token_analysis` 配置段, 运维很难发现默认会每 5 分钟自动索引并默认保留 8000 字符净输入。 | `backend/internal/config/config.go:1830-1835` 默认 `token_analysis.index_enabled=true`, `auto_index_interval_seconds=300`, `input_store_max_chars=8000`; `backend/internal/service/wire.go:152-158` `ProvideTokenAnalysisService` 创建后立即 `StartAutoIndex`; `deploy/config.example.yaml` 中未出现 `token_analysis` / `input_store_max_chars` / `auto_index_interval_seconds`。 | 在 `deploy/config.example.yaml` 增加完整 `token_analysis` 段, 明确隐私影响、默认值和关闭方式。考虑把 `input_store_max_chars` 默认改为 0, 或至少要求显式配置才留存全文。llm-wiki 和设计文档也要明确这是长期敏感数据存储。 |
| 中等 | 自动索引停止只关闭 stop channel, 不能取消正在进行的 `IndexRange`; cleanup 超时上下文对并行步骤没有约束, 服务退出可能被正在扫描的大文件拖到最长 10 分钟。 | `backend/internal/service/token_analysis_auto_index.go:56-66` 每轮使用独立 `context.WithTimeout(context.Background(), 10*time.Minute)`; `StopAutoIndex` 只 `close(s.autoIndexStop)` 后 `Wait()` (`token_analysis_auto_index.go:45-53`); cleanup 中直接调用 `tokenAnalysis.StopAutoIndex()` (`backend/cmd/server/wire.go:180-182`), 外层 `ctx` 没有传入 stop 逻辑。 | 在 `TokenAnalysisService` 保存 `autoIndexCancel context.CancelFunc`; `StopAutoIndex` 先 cancel 当前轮再等待, 或给 `StopAutoIndex(ctx)` 设置等待上限。补测试: fake repo 阻塞时 Stop 能在短超时内返回。 |
| 中等 | Token Analysis 明细抽屉的 `requestInputLoading` 是全局布尔, 连续点击两条记录时旧请求 finally 会把新请求的 loading 提前置 false, 短暂显示新记录预览而非 loading。 | `frontend/src/views/admin/TokenAnalysisView.vue:540-557` 每次点击都把 `requestInputLoading=true`, 但任何旧请求的 `.finally()` 都会执行 `requestInputLoading.value = false`; `.then()` 里检查了 `archive_id`, `finally` 没有检查。 | 在 finally 中也检查当前 `selectedRequest.archive_id`, 或使用递增 request token / AbortController。补前端测试: A 请求未返回时点击 B, A finally 不应关闭 B 的 loading。 |
| 中等 | token_analysis 目录热切换后索引只读当前目录, 旧目录历史文件不会被自动纳入后续索引。 | `backend/internal/service/token_analysis_indexer.go:35-41` 每次 `indexRange` 只取一次 `archiveDir` 并逐日拼当前目录文件; `docs/features/request-archive-dir-runtime-config-design-cn.md:55-56` 已说明旧文件需手动移动, 但 TokenAnalysis 页面和接口没有参数可选择旧目录。 | 在 TokenAnalysis 页面提示“仅索引当前归档目录”; 或为手动索引增加 source dir / include legacy dirs 选项。更完整方案是记录目录变更历史, 按日期范围读取多个目录。 |
| 轻微 | `Project` 过滤只支持精确匹配且区分大小写, UI 的项目输入框没有说明; 用户从项目排名点击能过滤, 但手输时容易以为支持模糊搜索。 | `backend/internal/repository/token_analysis_repo.go:839-844` 对非 `unattributed` 项目使用 `client_project = $n`; `frontend/src/views/admin/TokenAnalysisView.vue:23-26` 是普通输入框。 | 要么 UI 文案写“精确项目名”, 要么改为下拉/点击筛选, 或后端支持 `ILIKE` 模糊搜索并加合理索引/限制。 |

## 已验证

| 命令 | 结果 |
| --- | --- |
| `go test -p 1 -count=1 ./internal/service -run "TokenAnalysis|ProjectAttribution"` | 首次被 Windows Go build 缓存锁拦截; 使用 fresh `GOTMPDIR` 后通过。 |
| `go test -p 1 -count=1 ./internal/repository -run TokenAnalysis` | 首次被 Windows 临时 `.test.exe` 锁拦截; 使用 fresh `GOTMPDIR` 后通过。 |
| `go test -p 1 -count=1 ./internal/handler/admin -run TokenAnalysis` | 通过。 |
| `go test -p 1 -count=1 ./cmd/server -run TestProvideCleanup_WithMinimalDependencies_NoPanic` | 通过。 |
| `cmd.exe /c npm run typecheck` | 通过。 |
| `cmd.exe /c npx vitest run src/api/__tests__/admin.tokenAnalysis.spec.ts src/views/admin/__tests__/TokenAnalysisView.spec.ts` | 通过, 2 个文件 7 个测试。 |
| `git diff --check origin/main...HEAD` | 通过。 |

## 变更面概览

| 模块 | 主要变更 |
| --- | --- |
| RequestArchive | 异步队列写 JSONL; 响应只保存 size/hash/stream/usage; 后台设置可热切换 enabled/capture_response/dir。 |
| Token Analysis 索引 | 增加项目归因、用户净输入全文留存、自动索引循环。 |
| 数据库 | 新增 migration 145 项目归因字段和 project_roots; migration 146 用户输入留存表。 |
| Admin API | 新增 projects 聚合、request input 懒加载接口; request list 增加 input/quality/project 字段。 |
| 前端 | TokenAnalysis 页面改为请求明细, 加项目排行、全文抽屉、质量占位; Settings 增加 request archive 目录配置。 |
| 文档 | 新增多个 design / technical notes / review 文档, 并更新 llm-wiki。 |

## 合并前建议

| 优先级 | 建议 |
| --- | --- |
| P0 | 修改 `content_sha256` 策略, 避免原文可验证哈希长期入库。 |
| P0 | 在 `deploy/config.example.yaml` 补齐 `token_analysis` 配置段, 明确关闭自动索引和全文留存的方法。 |
| P1 | 让自动索引支持停止取消当前轮次, 避免停机卡住。 |
| P1 | 修复前端抽屉 loading 竞态。 |
| P2 | 在 TokenAnalysis 页面补充当前归档目录/历史目录切换说明, 或支持旧目录索引。 |

