# Token Analysis 项目归因(成员 × 项目 token 统计)设计

## 背景

公司需要看到每个成员在每个项目(代码仓库)上消耗了多少 token。网关收到的 LLM 请求本身没有标准字段承载"项目/分支",但主流编码客户端的请求体里携带了可提取的环境信息(工作目录、文件路径等)。

本设计基于对 2026-06-05 全天真实归档流量(10,856 个请求, 9.7GB)的三轮离线扫描验证, 提取规则的真实命中率已实测确认。

## 实测结论(设计依据)

| 客户端 | 请求占比 | 流量占比 | 直接提取来源 | repo 命中率(字节加权) |
| --- | --- | --- | --- | --- |
| Codex 系(Desktop/vscode/tui) | 47% | 62% | `<environment_context>` 内 `<cwd>` | 99.8%(100%) |
| GitHub Copilot Chat | 43% | 26% | `copilot-instructions.md` 路径 / 附件文件路径 × 已知仓库根前缀 | 70.3%(67.1%) |
| Claude Code | 4% | 8% | system prompt 的 `Working directory` + gitStatus | 99.6%(100%) |
| opencode / trae 等 | 4% | 2.5% | 同类 cwd 标记 | 72-87% |
| OpenAI SDK 直调 / 内部服务 | 2% | 0.7% | 无环境信息 | 0% |
| **全口径** | | | | **84.4%(请求)/ 90.0%(字节)** |

branch 仅 Claude Code 可提取(99.6%), Codex/Copilot 原理上不携带, 全口径仅 4.8%。
**结论: branch 不做统计维度, 仅留存字段; repo 维度可支撑统计。**

## 方案选型

| 方案 | 结论 |
| --- | --- |
| A. 客户端自定义 Header 上报 | 准确但需要全员配置 wrapper, 且 Copilot BYOK 不支持自定义 header(43% 请求无解); 作为后续增强, 本期不做 |
| B. 网关热路径在线解析请求体 | 解析成本高、依赖客户端 prompt 格式, 不应进热路径; 不做 |
| **C. 离线扫描归档 JSONL(本期)** | 复用 token_analysis 既有索引管道, 零热路径成本, 规则可随时迭代重扫 |

选 C 的关键依据: `token_analysis` 已具备完整基建——增量读取归档文件(断点续传)、请求体解析、`FindNearestUsageLog` 把归档事件匹配到 `usage_logs` 真实 token/费用、管理端 API 和前端页面。项目归因只是在这条管道上增加一个提取维度。

## 实现设计

### 1. 提取器(纯函数, 无外部依赖)

新文件 `backend/internal/service/project_attribution.go`:

```go
type ProjectAttribution struct {
    Workdir   string   // 归一化工作目录, 如 E:/code/lag-killer
    Project   string   // 仓库名(Workdir basename 或字典命中), 如 lag-killer
    Branch    string   // 仅 Claude Code 可得, 留存不统计
    Source    string   // 提取来源: system / env-context / instructions-md / known-root / 空=未归因
    PathHints []string // Copilot 等无直接 cwd 时收集的绝对路径, 供已知根前缀匹配
}

func ExtractProjectAttribution(endpoint, userAgent, body string) ProjectAttribution
```

提取规则(全部经真实流量验证):

| 客户端识别(UA + endpoint) | 规则 |
| --- | --- |
| Anthropic `/v1/messages*` | 解析 body JSON 的 `system` 字段文本, 取 `(Primary )Working director(y\|ies): X` 与 `Current branch: X`; body 非完整 JSON 时降级对前 300KB 做同样正则 |
| OpenAI `/v1/responses`(Codex) | 前 300KB 内 `<cwd>X</cwd>`(env context 固定在头部, 大请求可能尾部截断, 不影响) |
| Copilot(UA 含 copilot) | 先把嵌套 JSON 的 `\\` 还原为 `\`; ① `<root>/.github/copilot-instructions.md` 路径反推仓库根 ② `workspace with the following folders` 列表 ③ 无直接命中时收集附件 `filePath=` 与盘符绝对路径作为 PathHints |
| 其他 chat completions(opencode/trae 等) | 前 200KB 内 cwd 标记正则 |

清洗与防噪(对应 v1→v3 扫描迭代修掉的真实脏数据):

- 路径归一化: 反斜杠转 `/`、去引号、多值 `Working directories:` 在字面量 `\n` 处截断。
- cwd 合法性校验: 必须以 `/`、`~` 或 `盘符:/` 开头, basename ≤80 字符且不含 `:`。
- 黑名单: Copilot system prompt 示例路径(`Users/someone`、`pygorithm`)、`.copilot/skills`、`AppData`、`Program Files` 等。
- 盘符路径正则要求左边界非字母数字(防 `https://` 被截成 `s://`)。

### 2. 已知仓库根(known roots)与 Copilot 解析

Copilot 命中率的主力是"附件路径 × 已知仓库根前缀"(实测贡献 3148/4616)。已知根来自 Codex/Claude Code 流量直接提取的 cwd, 需要跨请求、跨天累积:

- 新表 `token_analysis_project_roots(root TEXT PK, project TEXT, first_seen, last_seen)`。
- 索引开始时加载全量 roots 到内存; 索引过程中每次直接命中 cwd 即学习(内存去重, 批量落库)。
- Copilot 行的 PathHints 与内存 roots 做最长前缀匹配; 命中则 `Source=known-root`。
- 冷启动顺序性: 某仓库的 Copilot 流量先于 Codex/CC 流量出现时, 当次无法归因(留空), roots 跨天持久化后稳态收敛; 后续可加"对未归因行重扫"或 GitLab 仓库清单导入, 本期不做。

### 3. Schema 变更(migration 145)

`token_analysis_request_summaries` 追加列(幂等 `ADD COLUMN IF NOT EXISTS`):

```sql
client_workdir TEXT NOT NULL DEFAULT '',
client_project TEXT NOT NULL DEFAULT '',
client_branch TEXT NOT NULL DEFAULT '',
attribution_source TEXT NOT NULL DEFAULT ''
```

索引: `(client_project, event_time DESC)`、`(user_id, client_project, event_time DESC)`。
新表 `token_analysis_project_roots` 见上。

### 4. 索引管道接入

`token_analysis_indexer.go` 的 `indexArchiveLine`:

- `tokenAnalysisArchiveEvent` 增加 `UserAgent string`(归档记录本就携带, 旧行向后兼容)。
- 调用 `ExtractProjectAttribution`, 结果写入 summary 新字段; PathHints 仅在内存中用于 roots 匹配, 不落库。

### 5. 聚合查询与管理端 API

- `TokenAnalysisFilters` 增加 `Project string`。
- `TokenAnalysisRepository` 增加 `ListProjectUsage(ctx, filters, pagination)`: 按 `client_project`(空值归入 `unattributed`)聚合 request_count / 各 token 字段 / actual_cost / 用户数, 支持按 user 过滤; 复用 usage 匹配结果(`usage_log_id` 关联行)。
- 同时给 `ListUserUsage`、`ListRequests` 透传 Project 过滤。
- 路由: `GET /api/v1/admin/token-analysis/projects`(handler `TokenAnalysis.Projects`)。
- 前端 `TokenAnalysisView.vue` 增加"项目"维度表格与筛选, i18n 同步。

### 6. 非目标

- 不做 branch 统计维度(数据留存在 `client_branch`)。
- 不做 user 级回填猜测归因(避免误归因, 未归因显式呈现为 unattributed)。
- 不做在线 Header 上报与 usage_logs 表改动(后续增强项)。
- 不改变归档写入端与轮转策略; 归档保留期治理另行处理。

## 影响范围

| 文件 | 改动 |
| --- | --- |
| `backend/internal/service/project_attribution.go` | 新增提取器 |
| `backend/internal/service/project_attribution_test.go` | 提取器单测(各客户端 fixture + 清洗规则) |
| `backend/internal/service/token_analysis_types.go` | summary/filters/repo 接口扩展 |
| `backend/internal/service/token_analysis_indexer.go` | 接入提取器与 roots 学习 |
| `backend/internal/repository/token_analysis_repo.go` | 新列读写、roots 表、ListProjectUsage |
| `backend/migrations/145_token_analysis_project_attribution.sql` | 新列、索引、roots 表 |
| `backend/internal/handler/admin/token_analysis_handler.go` | Projects 接口 |
| `backend/internal/server/routes/admin.go` | 路由注册 |
| `frontend/src/views/admin/TokenAnalysisView.vue` + api/types/i18n | 项目维度展示 |
| `llm-wiki/wiki/backend.md` | 行为说明 |

## 验证计划

| 验证 | 方式 |
| --- | --- |
| 提取器单测 | `cd backend && go test ./internal/service/ -run ProjectAttribution` |
| repo/索引回归 | `go test ./internal/repository/ ./internal/service/ -run TokenAnalysis` |
| 提取器移植一致性 | 用临时 Go runner 对 2026-06-05 真实归档跑一遍, 命中率与 Python v3 扫描(84.4%/90.0%)对照, 偏差应 <1pp |
| 全量回归 | `go test ./...` + 前端 `vue-tsc --noEmit` |

### 移植一致性实测结果(2026-06-07, 对 2026-06-05 全天 10,856 请求)

| 客户端 | Python v3 | Go 提取器(按时序) | 结论 |
| --- | --- | --- | --- |
| codex-cli | 99.8% | 99.8% | 一致 |
| claude-code | 99.6% | 99.6% | 一致 |
| copilot | 70.3%(全天根集合) | 68.5%(时序口径); 对未归因行用全天根集合二次解析可再补 203 条(≈72.9%) | 差异来自冷启动顺序, roots 跨天持久化后稳态收敛 |
| 全口径(字节) | 90.0% | 89.9% | 一致 |

Go 版 instructions-md 层(多重反斜杠完全折叠)优于 Python 版, Copilot 直接命中 48.0% 高于 v3 的同层表现。

## 运行前提(运维侧)

- `gateway.request_archive.enabled=true` 需常态开启(请求体是归因数据源); 响应正文已不落盘(瘦身改造), 磁盘大头是请求体, 需配套保留期清理。
- token_analysis 索引按现有触发方式运行(管理端 `POST /token-analysis/index`), 建议配合日常排班每日执行前一天文件。
