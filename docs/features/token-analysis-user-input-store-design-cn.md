# Token Analysis 用户净输入留存与请求明细页设计

## 背景

公司后续要分析"用户输入是否符合标准", 但评价维度尚未确定。当前 DB 只存 300 字符脱敏预览(`last_user_preview`), 完整请求正文只在归档 JSONL 里——一旦按保留期删除原始文件, 输入分析就无法回溯。本特性先把**原料和占位**落好:

1. 索引时把每次请求的用户净输入全文(脱敏+截断)单独入库;
2. 质量字段留空占位, 评价标准确定后由评估任务回填;
3. Token 分析页的"可疑请求"表改造为通用**请求明细**表, 一处可看: 时间、成员、模型、项目/分支、用量、质量、输入预览, 点行抽屉懒加载全文。

## 维度清单

| 维度 | 来源 | 状态 |
| --- | --- | --- |
| 请求时间 | `token_analysis_request_summaries.event_time` | 已有 |
| 成员 | `user_id` → users.email JOIN | 已有 |
| 模型 / 端点 | `model` / `endpoint` | 已有 |
| 仓库 / 分支 | `client_project` / `client_branch`(离线归因) | 已有 |
| 用量 / 费用 | `usage_log_id` → usage_logs JOIN | 已有 |
| 输入字符数 | `user_chars` | 已有 |
| 客户端来源 | `attribution_source` | 已有 |
| 风险分 | `risk_score` / `risk_reasons` | 已有 |
| **用户输入全文** | `token_analysis_user_inputs.content` | **本次新增** |
| **内容哈希** | `content_sha256`(同一人类输入跨 agent 轮次去重) | **本次新增** |
| **质量占位** | `quality_score/quality_findings/quality_version/evaluated_at` | **本次新增(空)** |

## 存储(migration 146)

`token_analysis_user_inputs`: `archive_id` UNIQUE 与 summaries 同键幂等; `content` 为脱敏+截断后的最后一条用户输入全文; `content_sha256` 对脱敏前原文(TrimSpace)计算, 跨截断稳定; `truncated` 标记防止误判短输入。独立表而非加列到 summaries: 大文本不进聚合热表, 质量字段与输入同生命周期, 后续可独立清理。

Upsert 冲突时只更新内容字段, **不触碰 `quality_*` 四列**——重建索引不会冲掉评估结果; 标准改版重评时按 `quality_version` 区分是哪一版标准打的分。

## 提取与脱敏

- `TokenAnalysisBodySummary.LastUserText` 在 sanitize 预览前保留原始全文(messages/responses/gemini/image 四 shape 同源)。
- `SanitizeTokenAnalysisInputText`(`token_analysis_summary.go`): 复用预览的 secret 脱敏正则, 但**保留换行/缩进**(评估时排版本身是信号), rune 截断返回 truncated。
- 配置 `token_analysis.input_store_max_chars`(默认 8000, 0=不留存全文)。

## 接口与页面

- 列表 `GET /admin/token-analysis/requests` LEFT JOIN user_inputs, item 增加 `has_input/input_truncated/quality_score`(全文不进列表 payload)。
- 全文懒加载 `GET /admin/token-analysis/requests/input?archive_id=`, 404 当无记录。
- TokenAnalysisView "可疑请求" → "请求明细": 列为 时间/成员/模型/项目+分支/用量/质量/预览(风险徽标并入预览列), 排序切换 按时间(默认)/按风险; 默认 `risk_min` 由 20 改为 0(明细定位展示全部请求, 风险排查时手动调高)。抽屉新增"用户输入全文"(pre-wrap, 截断提示)与"质量"区块(未评估占位)。

## 质量评估回填约定(后续)

评估任务(规则或 LLM)读取 `quality_score IS NULL`(或 `quality_version < 当前标准版本`)的行, 写回 `quality_score`(SMALLINT)、`quality_findings`(jsonb, 维度明细)、`quality_version`、`evaluated_at`。列表与抽屉已渲染这些字段, 回填即显示, 前端无需再改。

## 与既有 prompt metrics 的关系

仓库另有在线采集链路 `user_prompt_events` + `user_prompt_analysis`(migration 143, `internal/service/promptmetrics/`, 管理页"Prompt 指标"): 网关请求时落库 prompt 摘要/哈希, 本地规则打 clarity/context/quality 分。两者定位不同:

| | token_analysis_user_inputs(本特性) | user_prompt_events(prompt metrics) |
| --- | --- | --- |
| 采集 | 离线索引归档 JSONL, archive_id 幂等可重放 | 在线网关路径 |
| 全文 | 默认存(脱敏+8000 截断) | `store_full_text` 默认关, 只存摘要 |
| 用量 | usage_logs JOIN 真实 tokens/cost | 估算 tokens |
| 项目/分支 | 离线归因(实测 84% 请求级命中) | 在线提取 |
| 质量 | 占位待标准 | 本地规则版已有 |

后续若统一, 可经 `content_sha256` / `prompt_hash` 对齐两表, 把 prompt metrics 的规则分回填进 `quality_*`, 或反向把净输入全文供给 prompt metrics 的 LLM 分析——届时再做取舍, 本次不合并。

## 与 JSONL 保留策略的关系

净输入全文入库后, 原始 JSONL 可按保留期(建议 7-14 天)gzip+删除, 输入合规分析不再依赖原文窗口。体量预估: 1.1 万请求/天 × 平均几 KB ≈ 30-50MB/天(agent 轮次重发的同一输入可经 sha256 去重统计)。

## 验证

| 验证 | 方式 |
| --- | --- |
| 提取 | `go test ./internal/service/ -run "TokenAnalysisSummarize\|SanitizeTokenAnalysisInputText"`(原文保留/排版保留/脱敏/截断) |
| 索引留存 | `go test ./internal/service/ -run "TokenAnalysisIndexer"`(留存内容+哈希+截断; 0=不写; GetUserInput 404) |
| 前端 | `vitest run`(列表字段/点行懒加载全文)+ `vue-tsc --noEmit` |
| 端到端 | 触发索引 → user_inputs 出现行 → 页面明细表新列 → 点行抽屉全文; 手工 UPDATE quality_score → 重索引不被冲掉 |
