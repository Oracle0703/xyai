# Token Analysis 请求来源实现审核

日期：2026-10-24。对象：当前未提交工作区中的请求来源分类、净输入提取、索引接入和关联前端展示。两名 subagent 分别只读审核后端持久化与前端合同，主审核执行最小复现。未修改业务代码、未访问或写入生产。

最新状态：Claude 修复后的独立复核已关闭 F1–F3，未发现新的阻塞问题；保留一项索引完成自动提交筛选草稿的 P3。下文原始发现保留作为历史证据，最新验证见末尾“修复后独立复核”。

## 结论

来源分类主链路与声明的规则推断合同基本一致：使用原始结构、不依赖预览/重复数，机器元数据值域分别匹配，system/developer 尾注跳过，截断/探测/旧行可降级，前后端 reason/version 合同一致。

但净输入提取口径已经改变，历史回填的数据同步尚不完整，不能直接按现有“重置水位后重索引”方案验收。发现两项 P2 回填缺口；另复现一项页面既有筛选行为与新增 appliedFilters 展示不一致的问题。

## F1：P2，提取为空时旧净输入残留（已运行复现）

- 位置：`backend/internal/service/token_analysis_indexer.go:383`；触发本轮变化的是 `token_analysis_summary.go` 的纯用户文本抽取。
- 最小请求：assistant `tool_use` → user 仅含 `tool_result.content="tool output"`，没有真正的用户正文。旧版本会将工具内容写入 user_inputs；新版正确得到空 `LastUserText`。
- 重索引时仅在 `LastUserText` 非空时调用 `UpsertUserInput`，没有清除/失效旧输入的分支。摘要预览虽然变空，旧 `token_analysis_user_inputs` 行仍存在，列表 `has_input` 仍为 true，抽屉全文与重复哈希仍来自工具输出。
- 复现：在既有 repository stub 中预存该 archive_id 的旧工具文本，再运行真实 `indexArchiveLine`；确认新摘要预览为空后，`GetUserInput` 仍返回 `Content:"tool output"`。期望旧误提取输入不再可见的断言失败。
- 建议：为“启用留存、完整解析成功、确认没有用户正文”的回填结果同步失效旧输入。不要把采集关闭、归档截断或解析降级当成明确的空输入而误删历史数据。

## F2：P2，正文改变后旧评分仍绑定新正文（条件性缺陷）

- 位置：`backend/internal/repository/token_analysis_repo.go:701`；本轮 `summaryResponsesRequest` 改变了实际写入内容。
- 最小请求：user `fix the login bug` → assistant `I will read the file`。旧版保存后者，新版回填会保存前者。
- `ON CONFLICT` 更新 `content/content_sha256`，但无条件保留 `quality_score/quality_findings/quality_version/evaluated_at`。如果旧行已有评估，原本针对 assistant 文本的评分就会显示在新的用户正文上，且按未评估条件选取的任务可能不再处理它。
- 证据等级：SQL 与调用链静态确认；本轮没有查询生产是否已有非空质量字段，也没有声称生产已发生评分错配。若当前评分全为空，该风险尚未兑现。
- 建议：内容身份不变时保留评估；身份变化时明确失效或版本化关联旧评估。既有“不覆盖质量字段”合同应限定为同一份输入，而非无条件保留。

## F3：P2，排序/分页使用未提交筛选草稿（已运行复现，存量行为）

- 位置：`frontend/src/views/admin/TokenAnalysisView.vue:573`（同类 loader 也读取 `cleanFilters`），而 `appliedFilters` 仅在 `reloadAll()` 中更新。
- 复现：查询 model-a → 将输入框改为 model-b，但不提交 → 点击“按风险”排序。列表 API 实际传 `model-b`，已应用筛选标签仍为 `model-a`，概览也仍是旧查询。
- Vitest 中真实挂载当前 View 与子组件，查询与点击后检查：标签保留 A 的断言通过，API 应继续使用 A 的断言失败，实际收到 B。
- 归属：HEAD 原有 loader 已使用草稿，不能将该底层行为归为来源分类新增回归；当前 UI 重构新增 appliedFilters 快照/标签后，显示与实际列表查询不一致更加明确。这是同页待修项，不是分类算法缺陷。
- 建议：排序/分页续查使用已提交快照，只有显式提交筛选时才更新快照并统一刷新数据。

## 验证与边界

| 验证 | 本轮结果 |
| --- | --- |
| `go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin -run TokenAnalysis` | 3 个包全部通过 |
| View、API、双语完整性/编译 Vitest 专项 | 4 文件 / 46 测试通过 |
| F1 最小回填状态转换复现 | 失败，确认旧净输入残留 |
| F3 筛选快照复现 | 失败，确认实际请求 model-b 而标签 model-a |
| `git diff --check` | 通过 |

两份复现测试只临时加入本机，运行后已移除；未更改既有测试断言或实现。没有重复全仓构建，也没有把此前报告的完整门禁当成本轮重新执行的结果。

已知的普通 user 脚本请求、无标记内部任务、包装内排队用户消息等规则局限不重复列为新缺陷。272 条脱敏结构回放不是独立真值或原始请求全文执行；1,246 是含 response 的归档行数，两批非随机样本不能支持全天客户端份额或“影响很小”的推断。

文档还需收口：设计中“上线前全量门禁 normal/embed/production build 通过”和末尾“未进行全仓构建”并存，应按最终实际执行情况统一；影响评估中的“少量”也应改成“总体影响尚未量化”。

## 处理结果（2026-10-24）

- F1：已修复。新增 `DeleteUserInput`，仅在启用留存、未截断、未降级且完整解析后确认没有用户正文时删除旧净输入；回归用例覆盖删除与三种保留情形。
- F2：已修复。`quality_*` 仅在 `content_sha256` 不变时保留，换哈希即清空；嵌入式 PostgreSQL 18 新建空库集成测试通过。
- F3：已修复。所有加载、排序/翻页续查、趋势与索引触发统一读取 `appliedFilters`；View 回归用例按本审核的复现步骤断言仍请求 model-a。
- 文档收口：设计文档的构建陈述已按实际执行情况统一；影响评估改为“总体影响尚未量化”，并注明 1,246 行含 response 行、两批样本均非随机抽样。
- 详见设计文档“实现审核修订”一节。

## 修复后独立复核（2026-10-24）

两名 subagent 分别复核后端持久化与前端筛选合同，主审核重新执行专项测试。F1–F3 均可关闭，未发现本次修复引入的新阻塞；本轮只修正文档，没有更改业务代码。

| 项目 | 独立复核结果 |
| --- | --- |
| F1 旧净输入残留 | 删除限定为留存开启、未截断、未降级且抽取为空；回归覆盖删除、截断完整 JSON、截断损坏 JSON、关闭留存 |
| F2 评分错绑 | SQL 按更新前哈希判断；相同哈希保留全部质量字段，变化时清空，`quality_version` 回空字符串；独立 PostgreSQL 18 新空库实测通过 |
| F3 排序/翻页草稿混用 | 概览、各列表、趋势及索引触发读取已提交快照；新增 A → 草稿 B → 排序 → 翻页回归通过 |
| Go 专项 | `go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin -run TokenAnalysis` 三包通过 |
| PostgreSQL 集成 | `go test -tags integration -p 1 -count=1 ./internal/repository -run '^TestTokenAnalysisUserInputQualityFollowsContentHash$' -v` 明确输出 PASS，非 Docker 不可用导致的跳过；仅连接本轮新建本机独立集群，完成后已停止 |
| 前端专项 | View、API、双语完整性与编译共 4 文件 / 47 测试通过；`pnpm run typecheck` 通过 |
| 静态与文档检查 | 相关 View、组件、测试、API、locale 的 ESLint 通过；`git diff --check` 通过；本轮修改的 wiki 保持 LF |

### P3：索引完成会自动提交筛选草稿（存量行为，非阻塞）

- 位置：`frontend/src/views/admin/TokenAnalysisView.vue` 的 `pollIndexStatus()` 在索引结束后调用 `reloadAll()`；后者第一步把 `cleanFilters` 复制到 `appliedFilters`。
- 实际复现：提交 model-a → 触发索引 → 输入 model-b 但不提交 → 模拟 3 秒后的完成轮询。临时 Vitest 真实挂载页面后，期望继续请求 model-a 的断言失败，实际请求 model-b。
- 影响：自动刷新会让尚未确认的筛选生效；概览、列表和标签一起切换，不再是 F3 的混合口径。该路径此前已存在，不影响来源分类与 F1/F2 的回填修复。
- 建议：将“提交草稿”和“按现有快照刷新”分开，索引完成仅执行后者。临时复现文件运行后已删除。

本轮同步修正 backend wiki 中“当前未修复”和净输入设计中“无条件保留评分”的过期表述，并在 frontend wiki 标明上述 P3 例外。未执行生产部署、历史回填或提交；普通 user 脚本/无标记内部任务仍只能推断，不能视为已获得真实人工身份。
