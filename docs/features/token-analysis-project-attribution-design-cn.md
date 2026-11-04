# Token Analysis 项目归因（成员 × 项目 token 统计）设计

更新：2026-11-04。状态：**现行实现（cwd basename 归因，migration 145）已上线；2026-11-03 用户给出的新分类口径已确定但尚未实现，也未做历史回填（§7、§9、§13）。**

本文是 Token Analysis 项目归因功能的唯一文档，包含现行实现、设计依据、用户确定的新分类规则、2026-11-03 未归因线索采集结论、落地方案、代码位置、易错点和验证方式。原分类规则文档 `docs/features/token-analysis-project-classification-rules-cn.md` 与采集目录 `docs/delivery/2026-11-03-project-attribution-survey/`（README、candidates/evidence/verification 等 JSON、collect.py/summarize.py）已并入本文并移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:<路径>` 查看）。凡是对后续修改有约束的结论都已写入正文。关联功能：[用户净输入留存与请求明细页](token-analysis-user-input-store-design-cn.md)（同一索引管道、同一自动索引开关）。

阅读顺序建议：
- 先分清"现行实现"和"新规则"：读 §1；
- 修改提取器或索引：读 §4、§11；
- 实现新分类规则：读 §7、§8、§9、§11，待确认项见 §7.6；
- 定位代码：读 §10；跑测试：读 §12。

## 1. 背景与现状边界

公司需要看到每个成员在每个项目（代码仓库）上消耗了多少 token。网关收到的 LLM 请求没有标准字段承载"项目/分支"，但主流编码客户端的请求体里携带工作目录、文件路径等环境信息，可以离线提取。

| 口径 | 状态 | 归因值 | 正文 |
| --- | --- | --- | --- |
| 现行实现 | 已上线（2026-06） | 工作目录 basename；Copilot 靠已知根前缀匹配；未命中为空，聚合时显示 `unattributed` | §2–§6 |
| 新分类规则 | 2026-11-03 用户确定，**未实现** | 后端 `服务/项目`、明确项目根、orig-web 仓库名、已确认别名、其余有效路径按完整路径并备注"文件" | §7 |
| 历史回填 | **未执行** | — | §9 |

不能把 §7 的规则当作已生效，也不能把现行命中率当作"业务项目归一化正确"的证明：现行实现直接取 basename，日期目录、任务名、构建子目录都可能成为项目名（§8.6）。

## 2. 设计依据：2026-06-05 实测

基于 2026-06-05 全天真实归档流量（10,856 个请求，9.7GB）三轮离线扫描（Python v1→v3）确定提取规则：

| 客户端 | 请求占比 | 流量占比 | 直接提取来源 | repo 命中率（字节加权） |
| --- | --- | --- | --- | --- |
| Codex 系（Desktop/vscode/tui） | 47% | 62% | `<environment_context>` 内 `<cwd>` | 99.8%（100%） |
| GitHub Copilot Chat | 43% | 26% | `copilot-instructions.md` 路径 / 附件文件路径 × 已知仓库根前缀 | 70.3%（67.1%） |
| Claude Code | 4% | 8% | system prompt 的 `Working directory` + gitStatus | 99.6%（100%） |
| opencode / trae 等 | 4% | 2.5% | 同类 cwd 标记 | 72–87% |
| OpenAI SDK 直调 / 内部服务 | 2% | 0.7% | 无环境信息 | 0% |
| **全口径** | | | | **84.4%（请求）/ 90.0%（字节）** |

branch 实测仅 Claude Code 稳定携带（99.6%），Codex/Copilot 原理上不携带，全口径仅 4.8%。**结论：branch 不做统计维度，仅留存字段；repo 维度可支撑统计。**

Copilot 命中的主力是"附件路径 × 已知仓库根前缀"（实测贡献 3148/4616）。

## 3. 方案选型

| 方案 | 结论 |
| --- | --- |
| A. 客户端自定义 Header 上报 | 准确但需要全员配置 wrapper，且 Copilot BYOK 不支持自定义 header（43% 请求无解）；作为后续增强，本期不做 |
| B. 网关热路径在线解析请求体 | 解析成本高、依赖客户端 prompt 格式，不应进热路径；不做 |
| **C. 离线扫描归档 JSONL（采用）** | 复用 token_analysis 既有索引管道，零热路径成本，规则可随时迭代重扫 |

选 C 的依据：`token_analysis` 已有增量读取归档文件（offset 续读）、请求体解析、账单关联（现为 `linkUsageRange`）、管理端 API 和前端页面，项目归因只是在这条管道上增加一个提取维度。

## 4. 现行实现

### 4.1 提取器

`backend/internal/service/project_attribution.go`，纯函数、无外部依赖：

```go
type ProjectAttribution struct {
    Workdir   string   // 归一化工作目录, 如 E:/code/lag-killer
    Project   string   // Workdir basename 或已知根命中的项目名
    Branch    string   // 留存, 不做统计维度
    Source    string   // system / env-context / instructions-md / workspace-folders / raw-head / known-root; 空=未归因
    PathHints []string // 无直接 cwd 时收集的绝对路径(小写归一, 最多 60 条), 只在内存中用于已知根匹配, 不落库
}

func ExtractProjectAttribution(endpoint, userAgent, body string) ProjectAttribution
```

按 UA 与 endpoint 分派（顺序即优先级）：

| 判定 | 函数 | 规则 | Source |
| --- | --- | --- | --- |
| UA 含 `copilot` | `extractCopilotAttribution` | 先把 `\\` 循环折叠为单 `\`（嵌套 JSON 每层翻倍）；① `<root>/.github/copilot-instructions.md` 反推仓库根 ② `workspace with the following folders` 列表 ③ 都没有时收集 `filePath=` 属性与盘符绝对路径作为 PathHints | `instructions-md` / `workspace-folders` |
| UA 含 `claude-cli`/`claude-code`，或 endpoint 前缀 `/v1/messages` | `extractAnthropicAttribution` | body 是完整 JSON 时**只看顶层 `system`**（字符串或文本块数组拼接），取 `(Primary )Working director(y\|ies): X` 与 `Current branch: X`；body 非完整 JSON（截断）时降级对前 300 KiB 做同样正则 | `system` / `raw-head` |
| UA 含 `codex` 或 endpoint 为 `/v1/responses` | `extractCodexAttribution` | 前 300 KiB 内 `<cwd>X</cwd>`，同窗口内也尝试 `Current branch:` | `env-context` |
| 其他（opencode/trae 等） | `extractGenericAttribution` | 前 300 KiB 内先找 `Working directory` 类标记，再找 `<cwd>` | `raw-head` |

清洗与防噪（对应 v1→v3 扫描修掉的真实脏数据）：

- `normalizeAttributionPath`：去首尾空白与引号/反引号，反斜杠转 `/`，去尾部 `/`。
- `cutAttributionLiteralNewline`：多值 `Working directories:` 在字面量 `\n`（及 `/n- `、`/n  `）处截断。
- `validAttributionCwd`：非空、≤250 字节、不含换行与 NUL；以 `/`、`~` 或 `盘符:/` 开头；basename 非空、≤80 字节且不含 `:`。
- `projectAttributionBlacklist`（子串、忽略大小写）：`users/someone`、`pygorithm`（Copilot system prompt 示例）、`.copilot`、`appdata`、`code/user/prompts`、`%3a`、`program files`、`/windows/` 等，只用于 Copilot 分支。
- 盘符路径正则要求左边界非字母数字（防 `https://` 被截成 `s://`）。
- `AttributionProjectOf`：项目名 = 归一化工作目录最后一段。

### 4.2 已知仓库根与 Copilot 匹配

已知根来自直接命中的工作目录，跨请求、跨天累积：

- 表 `token_analysis_project_roots(root PK, project, first_seen, last_seen)`。
- `indexRange` 开始时 `newTokenAnalysisProjectRoots` 经 `LoadProjectRoots` 全量载入内存；每条直接归因的请求调用 `learn`（键为小写工作目录，内存去重）；`indexRange` 结束或出错时 `flush` 经 `UpsertProjectRoots` 落库（已存在只刷新 `project`/`last_seen`）。
- Copilot 行无直接命中且有 PathHints 时，`match` 用 `MatchAttributionKnownRoot` 做最长前缀匹配（根按长度降序，要求等于根或以 `根/` 开头），命中则 `Source=known-root`。
- 冷启动顺序性：某仓库的 Copilot 流量先于 Codex/Claude Code 流量出现时当次无法归因，根跨天持久化后稳态收敛。对未归因行重扫、GitLab 仓库清单导入均未做。

### 4.3 索引管道接入

`token_analysis_indexer.go` 的 `indexArchiveLine`：对每条 `request` 事件调用 `ExtractProjectAttribution(endpoint, event.UserAgent, event.Body)`（`tokenAnalysisArchiveEvent.UserAgent` 来自归档记录），结果写入 `TokenAnalysisRequestSummary` 的 `ClientWorkdir/ClientProject/ClientBranch/AttributionSource`。`UpsertRequestSummary` 冲突时覆盖这四列，因此对同一行重新索引会按当前提取器重算归因。

### 4.4 聚合查询与接口

- `TokenAnalysisFilters.Project`：`buildTokenAnalysisWhere` 中 `unattributed` 映射为 `client_project = ''`，其他值**精确匹配、区分大小写**；同时作用于用户、项目、请求明细等列表。
- `ListProjectUsage`（`token_analysis_repo.go`）：按 `(client_project, user_id)` 即"项目 × 成员"分组，空项目显示为 `unattributed`；输出 `request_count`、`matched_request_count`（已关联账单的行数）、各 token 字段与 `actual_cost`（`usage_log_id` LEFT JOIN `usage_logs`）、`last_event_time`、`human_input_count`（`tokenAnalysisHumanInputSQL`，估算值）。排序由 `tokenAnalysisProjectOrderBy` 映射。
- 路由：`GET /api/v1/admin/token-analysis/projects` → `TokenAnalysisHandler.Projects`（`routes/admin.go` 的 `registerTokenAnalysisRoutes`）。
- 前端：`frontend/src/views/admin/TokenAnalysisView.vue` 的项目筛选与 `frontend/src/components/admin/token-analysis/TokenAnalysisProjectTable.vue` 项目排行；`TokenAnalysisRequestTable.vue` 显示项目+分支（空项目显示"未归因"），`TokenAnalysisRequestDrawer.vue` 显示项目/分支/来源/工作目录。

### 4.5 非目标

- 不做 branch 统计维度（数据留在 `client_branch`）。
- 不做用户级回填猜测（避免误归因，未归因显式呈现为 `unattributed`）。
- 不做在线 Header 上报，不改 `usage_logs`。
- 不改变归档写入端与轮转策略；归档保留期治理另行处理。

## 5. 数据模型与迁移（migration 145）

`backend/migrations/145_token_analysis_project_attribution.sql`（受 checksum 校验，不可修改；文件头引用本文路径）：

- `token_analysis_request_summaries` 追加 `client_workdir`、`client_project`、`client_branch`、`attribution_source`，均为 `TEXT NOT NULL DEFAULT ''`，`ADD COLUMN IF NOT EXISTS` 幂等。
- 索引：`idx_token_analysis_project_time (client_project, event_time DESC)`、`idx_token_analysis_user_project_time (user_id, client_project, event_time DESC)`。
- 新表 `token_analysis_project_roots (root TEXT PRIMARY KEY, project TEXT NOT NULL DEFAULT '', first_seen, last_seen)` 与索引 `idx_token_analysis_project_roots_project`。

新分类规则需要的类型/规则版本/证据字段尚未建模，见 §9 阶段 2。

## 6. 运行前提与配置

- `gateway.request_archive.enabled=true` 需常态开启（请求体是归因数据源）。响应正文已不落盘，磁盘大头是请求体，需配套保留期清理。
- 索引默认随服务启动自动运行：`StartAutoIndex` 启动后先跑一轮，再每 `token_analysis.auto_index_interval_seconds`（默认 300）秒对 [昨天, 今天] 做增量索引（offset 续读、幂等），**无需外部 cron**。`auto_index_interval_seconds: 0` 或 `token_analysis.index_enabled: false` 时只能手动；管理端 `POST /api/v1/admin/token-analysis/index` 用于补索引历史日期。默认值见 `backend/internal/config/config.go` 与 `deploy/config.example.yaml`，上线时须核对生产实际配置。
- 自动索引意味着开启 request_archive 后归因与净输入留存都会自动落库，敏感数据影响见[用户净输入留存](token-analysis-user-input-store-design-cn.md)。
- 索引只读当前生效的归档目录（后台可热切换），切换后旧目录文件需手动迁移。

## 7. 新分类规则（2026-11-03 用户确定，尚未实现）

来源：用户看过 §8 的线索清单后给出的分类说明。本节只是业务口径，不指定数据库字段；提取器、存储、聚合、页面均未修改，也未重索引或回填。

### 7.1 分类原则

识别到明确的项目边界，就统一归到该项目，忽略项目内部的目录、模块和文件。没有足够依据确定项目边界时，使用提取到的完整文件或目录路径作为归因值，备注为"文件"。完全没有有效路径或项目证据时，保留未归因。

| 分类结果 | 归因值 | 备注 |
| --- | --- | --- |
| 后端项目 | `服务/项目`，例如 `micro-manager/activity-server` | 可留空；仓库地址单独保留为来源证据 |
| 其他明确项目 | 项目名或仓库名，例如 `WelcomeScreen`、`compass`、`xyai` | 可留空 |
| 文件或普通目录 | 规范化后的完整路径 | `文件` |
| 无有效证据 | 未归因 | 沿用现有未归因语义 |

"文件"也包含无法确定项目根的目录；不能把目录末段随意提升为项目名。

### 7.2 用户已明确的规则

| 规则 | 匹配依据 | 归因结果 | 边界 |
| --- | --- | --- | --- |
| R1 后端服务/项目 | 已知后端仓库结构中的 `micro-*/<项目>/…` 或 `microservice/<项目>/…` | `服务/项目` | 截止到项目这一层，后面的 modules、services、models、src、文件名均不继续细分 |
| R2 已明确的项目根 | 路径属于确认的项目根及其子目录 | 统一项目名 | O02 → `WelcomeScreen`；O03 同类 compass 子目录 → `compass`；O18 → `xyai` |
| R3 orig-web 仓库 | 仓库地址或本地路径含完整目录段 `orig-web/<仓库名>/…` | `<仓库名>` | 取 orig-web 后第一段；去仓库后缀 `.git`，忽略其后文件、分支/提交页面等路径 |
| R4 已确认别名/副本 | 已知 worktree、临时副本或仓库别名与正式项目对应 | 正式项目名 | O19 → `client-web-page`；O18 已列出的审核副本 → `xyai` |
| R5 普通文件/目录兜底 | 有有效路径，但 R1–R4 均未确定项目边界 | 完整文件或目录路径 | 备注固定为 `文件`；不猜测业务名称、不按用户合并 |

- R1 依据用户此前说明"后端仓库都基于这个理念"（后端结构 `192.168.1.27 / 服务 / 项目 / 文件`），本次以 `micro-manager/activity-server` 明确截止层级。同名项目在不同服务下分开，例如 `micro-activity/modules` 与 `micro-perm/modules`；项目内的 `battlepass/modules/…` 仍归 `micro-activity/battlepass`。
- R3 与 R4 的关系：先提取原仓库名，再应用明确的别名。例如 `orig-web/orig.client-web-page.git` 最终为 `client-web-page`。用户没有要求全局删除仓库名的 `orig.` 前缀，其他仓库默认保留原名；后续有明确别名再合并。

### 7.3 例子

省略号表示已有路径的父级或子级，不是需要匹配的字面字符。

| 输入线索 | 归因值 | 备注 | 依据 |
| --- | --- | --- | --- |
| `…/192.168.1.27/micro-activity/battlepass/README.md` | `micro-activity/battlepass` | — | R1 |
| `…/micro-manager/activity-server/modules/activitypush/parse.go` | `micro-manager/activity-server` | — | R1，不再拆到 activitypush |
| `…/micro-perm/modules/application/application.update.go` | `micro-perm/modules` | — | R1，modules 是项目这一层 |
| `…/micro-activity/battlepass/modules/battlepass/claim.go` | `micro-activity/battlepass` | — | R1，modules 是项目内部目录 |
| `…/PycharmProjects/WelcomeScreen/vanna_data/chroma.sqlite3` | `WelcomeScreen` | — | O02 / R2 |
| `D:/AI_project/compass/desktop/framework/src-tauri/…` | `compass` | — | O03 / R2 |
| `…/compass/component/process.git` | `compass` | — | 用户说明的 compass 同类结构 / R2 |
| `E:/allsite/xyai/frontend/src/…` | `xyai` | — | O18 / R2 |
| `E:/tmp/imagegen-review-fixes/…` | `xyai` | — | O18 已采集的特定副本 / R4 |
| `…/.codex/worktrees/3b88/client-web-page/src/…` | `client-web-page` | — | O19 / R4 |
| `http://192.168.1.23/orig-web/orig.client-web-page.git` | `client-web-page` | — | R3 提取仓库名，R4 应用 O19 别名 |
| `…/orig-web/orig.xyweb-lib/src/…` | `orig.xyweb-lib` | — | R3，当前没有额外删前缀的别名规则 |
| `…/orig-web/orig.cddada7d.xunyou.com.git` | `orig.cddada7d.xunyou.com` | — | R3 |
| `E:/10.13` | `E:/10.13` | 文件 | R5，不猜测日期目录的业务名 |
| `F:/police/dataset` | `F:/police/dataset` | 文件 | R5，不与其他目录自动合并 |
| `F:/police/model/train.py` | `F:/police/model/train.py` | 文件 | R5，保留文件路径，不只取 train.py |
| `e:/tmp` | `E:/tmp` | 文件 | R5，O18 的特定副本别名不扩展到所有 tmp 目录 |

### 7.4 同类推广口径

以下是根据用户范例整理的推广办法，**不声称用户已经逐项命名了 §8.3 全部 43 组候选**。

| 情况 | 推广办法 |
| --- | --- |
| 同一已确定项目根下的不同子目录 | 全部回到同一个项目名；例如确认 `atlas` 为项目根后，deploy、manager-service、resource-service、service_sdk 均可归 `atlas` |
| 存放多个项目的容器目录 | 不把磁盘根、用户名、code、AI_project、PycharmProjects、allsite、workshop、tools、worktrees 哈希直接当项目；需要确定其下的实际项目根 |
| 尚未确定根的其他候选 | 当前按完整路径归因并备注"文件"；不能只凭某个目录名像项目就自动套用 compass 的层级 |
| 不同机器上同一已确认项目 | 可按同一项目规则归并，来源路径分别保留；普通文件兜底不自动跨机器合并 |
| 同名仓库或跨地址仓库 | 保留仓库主机与完整根作为证据；没有别名或同源依据时，不仅因显示名相同就断定同一仓库 |

### 7.5 处理顺序与证据选择

1. 从当前工作区、工作目录、仓库地址、附件（含 `tagged_files`）提取有效路径，去掉列表符号、行号和 `(85 lines)` 等外围说明。
2. 先判断该路径是否属于当前工作对象，再应用分类。依赖缓存、工具自带示例、普通文档引用、历史工作区只是辅助证据，不自动替代当前项目。
3. 匹配明确的根/别名与后端结构；`orig-web` 先取仓库名，再应用明确别名；命中后停止向项目内部细分。全部未命中才使用文件路径兜底。
4. 路径分隔符统一为 `/`；Windows 盘符统一大写、比较时忽略大小写，展示保留目录原名；POSIX 路径保留大小写。`.git` 去除仅用于仓库名，不能改变普通文件名。
5. 目录规则按完整路径段匹配：`compass-old` 不匹配 `compass`，`xyai-backup` 不因前缀相同归到 `xyai`；临时副本只使用已确认的映射。

多线索选择（实施默认口径，**非用户已确认的费用拆分规则**）：优先使用当前明确工作区；附件与仓库证据归一到同一个项目时合并。确实涉及多个项目且不能确定主项目时，保留候选并继续未归因；不取文本中第一条路径、不因频次较高就归入某项目，也不把一条请求的完整 token 重复计入多个项目。

Go `pkg/mod` 等依赖缓存即使命中 `micro-*/<项目>`，也应先经过"当前工作对象"判断：单纯引用 `micro-activity/modules` 依赖不能把当前 `battlepass` 请求改归 modules。文件兜底同样不能把无关示例或缓存路径变成当前归因。

### 7.6 待确认项

用户规则覆盖了范例与可按 R1/R3 机械判定的候选；以下仍需用户确认（编号见 §8.3）。确认前实现应按 R5 处理（完整路径 + "文件"），不得自行命名项目。

| 候选 | 待确认问题 |
| --- | --- |
| O07–O10（atlas 系） | `atlas` 是否为项目根（确认后 deploy/manager-service/resource-service/service_sdk 统一归 `atlas`） |
| O11–O15（workshop/interactive-map、game-guide、game-manager、mod-manager，tools/repo-deck） | workshop、tools 是容器目录；其下各目录是否独立项目、是否属于 compass 产品；repo-deck 工具自身是否计项目 |
| O16 gitlab_tool | 单列还是归属某业务项目 |
| O17 xy-data-router/xunyou-datarouter | 是否确认 `xunyou-datarouter` 为项目根（src、cgo、构建子目录统一归入） |
| O21 orig-lagz/orig.www.lagzapper.com | 不在 `orig-web/` 下，R3 不适用；是否统一为某名称 |
| O22 远程 `orig-web/orig.cddada7d.xunyou.com` 与本地 `E:/orig-web/cddada7d.xunyou.com` | R3 分别得到两个不同仓库名；是否同一项目需别名确认 |
| O23 marketing-client / marketing-client-component | 替代关系还是独立项目；是否与 B09 marketing-agency 合并 |
| O24 192.168.1.23/beyond/g | 样本主要是规范文档引用，是否只作规则背景、不参与归因 |
| O25 Documents/Codex/2026-10-15/wom、O26 Documents/海外 等 | 是否业务项目；日期父目录是否忽略 |
| O27 e:/tmp | 临时工作区需要什么明确仓库证据才能并入真实项目（目前除 O18 指定副本外一律"文件"） |
| B06 本地 `E:/micro/perm-server` | `micro/` 不符合 R1 的 `micro-*` 结构；是否与远程 `micro-manager/perm-server` 合并 |
| B11–B14（version-release、version-deploy、kol-oversea、marketing-v3） | 现行已按 basename 归因；新规则会改成 `服务/项目`，历史行是否回填改名 |
| 依赖缓存 | 当前请求确实在修改依赖源码时是否另计 |
| 多项目请求 | 单主项目、拆分还是多项目统计（§7.5 的默认口径未经用户确认） |

## 8. 2026-11-03 未归因线索采集（证据）

只读采集，没有部署、重索引、生产数据库写入、业务代码修改或提交，没有浏览器验证。目的：给用户提供待确认清单，**不是**归因决策。

### 8.1 范围与方法

| 项目 | 结果 |
| --- | --- |
| 时间窗口 | `event_time` ∈ [2026-11-02 00:00, 2026-11-03 00:00) Asia/Shanghai |
| 原始文件 | `2026-11-02.jsonl`，231,243,156,719 bytes（215.36 GiB）；扫描前后 size/mtime 不变；该文件索引水位 `last_offset` 等于文件大小，`processed_rows` 68,016、`failed_rows` 0（与摘要行数是不同计数器） |
| 已索引请求 | 摘要行 68,019，窗口内 94 个用户；未归因（`client_project = ''`）2,690（3.95%），涉及 46 个用户 |
| 读取范围 | 全部 2,690 条未归因请求，逐条校验 `archive_id` 与 `event == "request"`；不是全文件审计 |
| 读取成本 | 按行末 `source_offset` 以 1 MiB 块倒读到上一个换行（单行上限 128 MiB）；I/O 4,420,796,416 bytes（约 4.12 GiB），目标请求行合计 2,791,372,658 bytes（约 2.60 GiB），135 秒 |
| 原文形态 | 289 条含 `tagged_files`；32 条正文不是可解析 JSON，改用原始文本搜索；`body_truncated` 均为 false |
| 路径初筛 | 2,556 条出现宽泛路径/URL 线索（含示例、工具与运行时路径），不能解释为 2,556 条可准确归因 |
| 候选整理 | 43 组候选，2,196 条请求至少命中一组；内部 Go 模块缓存（`/pkg/mod/192.168.1.27/`）出现于 282 条请求，单独标注 |
| 提取器一致性 | 线上与本地 `project_attribution.go` SHA-256 均为 `b7c83dd65e277639ebbe3955d2e71a5651cd5cbe9e8078e079013f731e19808b`；HEAD `26f6363af` 的该文件哈希仍相同（最后修改 2026-06-10），即 §4 描述的就是当时线上行为 |
| 本地校验 | 倒读首行、多块大行、末行 3 项边界检查通过；请求身份唯一、候选成员完整、每请求见证 ≤20 条 |

采集器（服务器上经 stdin 执行，只向 stdout 输出，低 CPU/IO 优先级）口径：

- 数据库只读：每条查询包在 `BEGIN READ ONLY; SET LOCAL statement_timeout='30s'; …; ROLLBACK;`。读三组数据：窗口统计；未归因行的 `archive_id, source_file, source_offset, user_id, event_time, user_agent, endpoint`；已归因行按 `(client_project, client_workdir)` 计数（用于"当前已归因"列）。
- 每行记录 `body_sha256`（归档自带）和 `line_sha256`（整行 SHA-256），不导出 prompt 原文。
- 遍历 body 的 `system/instructions/input/messages/content/text/arguments/output/result` 字段并记录 `角色/字段` 位置，提取四类线索：`tagged_files`（`<tagged_files>` 内路径）、`cwd_marker`（`<cwd>` 或 `(Primary )Working director(y|ies):`）、`path_mention`（盘符路径或 `/Users|home|root|workspace(s)|work|app|opt|mnt|data|code|projects|tmp/` 开头路径）、`repository_url_mention`（http/ssh/git URL 且含 github/gitlab/gitee/`git.`/svn/`192.168.`/`.git`）。
- 清洗：去行号 `:N(:M)`、尾部标点和外围说明；`/Users/<名>/`、`/home/<名>/`、`X:/Users/<名>/` 遮蔽为 `<user>`；URL 去掉用户信息与 query；路径或 URL 含 `sk-`、`ghp_`、`github_pat_`、`token`、`password`、`secret` 的直接丢弃；超过 300 字符丢弃。

整理器（本地）口径：

- 43 组候选为人工列出的路径片段；匹配要求**完整路径段**（`(^|/)片段(/|\.git(/|$)|$)`，忽略大小写），`compass-old` 不会命中 `compass`。
- 匹配前排除依赖路径（`/pkg/mod/`、`/node_modules/`、`/site-packages/`）；内部 Go 模块缓存另计，只保留模块身份。
- 每组按 `archive_id` 去重计数，并分别统计含目录标记、附件、仓库地址的请求数；"当前已归因"= 已归因工作目录命中同一片段的请求数之和。
- 每条请求每类证据最多保留 4 条见证路径；整理后的证据不能当完整采集输入重跑聚合（整理器会拒绝）。
- O16 修正本地分组前缀后，对该用户 319 条请求定向复核，得到 70 条请求的路径见证（69 条含目录标记、69 条含 `tagged_files`），全部位于 `/Users/<user>/code/python/gitlab_tool` 下。

### 8.2 计数口径

- 每行"未归因线索请求"按 `archive_id` 去重；同一请求可提及多个候选，**各行不能相加**。
- 对话历史会重复携带路径，出现次数不等于独立会话、人工操作数、项目用量或可回填条数。
- "目录标记"可能来自当前环境、历史环境或引用；"附件"也可能是依赖文件。必须结合位置、角色、主工作区与多仓库冲突规则后才能自动归因。
- **线索覆盖数不能当作采用 §7 规则后的准确归因率。**

### 8.3 候选清单

列：未归因线索请求数 / 涉及用户数 / 其中含目录标记、附件、仓库地址的请求数 / 当日已归因请求数（现行 basename 结果）/ 采集时提出的问题 / §7 规则下的处理。"规则下的处理"中，"用户明确"指 §7.2–§7.3 直接点名；"按 R1/R3"指可按结构机械判定；"待确认"见 §7.6。

后端：

| 编号 | 路径/仓库线索 | 未归因 | 用户 | 标记/附件/地址 | 已归因 | 采集时问题 | §7 规则下的处理 |
| --- | --- | ---: | ---: | --- | ---: | --- | --- |
| B01 | `192.168.1.27 / micro-activity / battlepass` | 217 | 1 | 212 / 183 / 0 | 0 | 是否用服务/项目作为唯一标识 | 用户明确：`micro-activity/battlepass` |
| B02 | `micro-activity / condition` | 10 | 1 | 8 / 8 / 0 | 388 | condition 是否独立项目 | 按 R1：`micro-activity/condition` |
| B03 | `micro-activity / modules` | 16 | 1 | 15 / 0 / 0 | 0 | 公共模块是否单列（依赖缓存另列） | 按 R1：`micro-activity/modules`；仅依赖缓存引用不算 |
| B04 | `micro-perm / modules` | 202 | 1 | 0 / 0 / 117 | 0 | 是否单列公共权限模块 | 用户明确：`micro-perm/modules` |
| B05 | `micro-perm / login-server` | 161 | 1 | 0 / 0 / 0 | 0 | 是否单列登录服务 | 按 R1：`micro-perm/login-server` |
| B06 | `micro-manager / perm-server；E:/micro/perm-server` | 207 | 1 | 207 / 0 / 74 | 0 | 本地短路径是否与远程合并 | 远程按 R1：`micro-manager/perm-server`；本地短路径待确认 |
| B07 | `microservice / gateway` | 201 | 1 | 0 / 0 / 75 | 0 | microservice 是否也是两层 | 按 R1：`microservice/gateway` |
| B08 | `micro-manager / activity-server` | 68 | 1 | 0 / 0 / 11 | 0 | 是否独立活动服务 | 用户明确：`micro-manager/activity-server` |
| B09 | `micro-manager / marketing-agency` | 68 | 1 | 0 / 0 / 0 | 41 | 是否与客户端目录分开 | 按 R1：`micro-manager/marketing-agency`；与 O23 关系待确认 |
| B10 | `micro-user / personal-center` | 15 | 1 | 0 / 0 / 0 | 0 | 正文提及，正式名 | 按 R1：`micro-user/personal-center`（仍需通过"当前工作对象"判断） |
| B11 | `micro-manager / version-release` | 0 | 0 | 0 / 0 / 0 | 167 | 是否保留服务层 | 按 R1：`micro-manager/version-release`；历史改名待确认 |
| B12 | `micro-manager / version-deploy` | 0 | 0 | 0 / 0 / 0 | 62 | 同上 | 同上 |
| B13 | `micro-business / kol-oversea` | 0 | 0 | 0 / 0 / 0 | 77 | 同上 | 同上 |
| B14 | `micro-activity / marketing-v3` | 0 | 0 | 0 / 0 / 0 | 60 | 同上 | 同上 |
| B15 | `micro-user / authtype-phone` | 1 | 1 | 0 / 0 / 0 | 0 | 低频正文提及 | 按 R1（需"当前工作对象"判断） |
| B16 | `micro-user / internalserver` | 1 | 1 | 0 / 0 / 0 | 0 | 低频正文提及 | 同上 |

其他目录：

| 编号 | 路径/仓库线索 | 未归因 | 用户 | 标记/附件/地址 | 已归因 | 采集时问题 | §7 规则下的处理 |
| --- | --- | ---: | ---: | --- | ---: | --- | --- |
| O01 | `E:/10.13；F:/police/dataset；F:/police/model` | 417 | 1 | 417 / 0 / 0 | 0 | 正式项目名及合并关系 | 用户明确：R5 完整路径 + "文件" |
| O02 | `PycharmProjects/WelcomeScreen`（Windows / macOS） | 228 | 1 | 227 / 0 / 0 | 0 | 真实项目还是 IDE 默认目录 | 用户明确：`WelcomeScreen` |
| O03 | `D:/AI_project/compass/desktop/framework` | 260 | 1 | 215 / 0 / 0 | 0 | compass/desktop/framework 取哪层 | 用户明确：`compass` |
| O04 | `compass/component/process` | 315 | 2 | 0 / 0 / 187 | 0 | 单列还是归 compass | 用户例子：`compass` |
| O05 | `compass/component/screen-perception` | 187 | 2 | 0 / 0 / 187 | 0 | 同上 | 用户明确（R2：已确认根 `compass` 的同类子目录）：`compass` |
| O06 | `compass/runtime/llm-core` | 325 | 2 | 0 / 0 / 187 | 0 | 同上 | 用户明确（R2：已确认根 `compass` 的同类子目录）：`compass` |
| O07 | `D:/AI_project/atlas/deploy` | 401 | 1 | 215 / 0 / 0 | 0 | 是否 atlas 子仓库 | 待确认 atlas 根 |
| O08 | `D:/AI_project/atlas/manager-service` | 260 | 1 | 0 / 0 / 0 | 0 | 汇总还是单列 | 同上 |
| O09 | `D:/AI_project/atlas/resource-service` | 101 | 1 | 0 / 0 / 0 | 0 | 同上 | 同上 |
| O10 | `atlas/service_sdk` | 138 | 2 | 0 / 0 / 34 | 0 | SDK 单列还是归 atlas | 同上 |
| O11 | `D:/AI_project/workshop/interactive-map` | 468 | 1 | 155 / 0 / 0 | 0 | editor_service/web/plugin 一个还是多个 | 待确认 |
| O12 | `D:/AI_project/workshop/game-guide` | 313 | 1 | 0 / 0 / 0 | 0 | 是否独立、是否属 compass | 待确认 |
| O13 | `D:/AI_project/workshop/game-manager` | 313 | 1 | 0 / 0 / 0 | 0 | 同上 | 待确认 |
| O14 | `D:/AI_project/workshop/mod-manager` | 186 | 1 | 0 / 0 / 0 | 0 | 同上 | 待确认 |
| O15 | `D:/AI_project/tools/repo-deck` | 239 | 1 | 0 / 0 / 0 | 0 | 工具自身是否计项目 | 待确认 |
| O16 | `/Users/<user>/code/python/gitlab_tool` | 70 | 1 | 69 / 69 / 0 | 0 | 单列还是归业务项目 | 待确认 |
| O17 | `xy-data-router/xunyou-datarouter` | 86 | 1 | 86 / 0 / 0 | 571 | 子目录是否统一归入 | 待确认项目根 |
| O18 | `E:/allsite/xyai；E:/tmp/imagegen-review-fixes` | 198 | 1 | 197 / 0 / 0 | 138 | 审核副本是否并入；多项目取主项目 | 用户明确：`xyai`（副本别名仅限该目录） |
| O19 | `.codex/worktrees/3b88/client-web-page；orig-web/orig.client-web-page` | 36 | 1 | 36 / 0 / 36 | 0 | worktree 与 `orig.` 前缀是否统一 | 用户明确：`client-web-page` |
| O20 | `orig-web/orig.xyweb-lib` | 36 | 1 | 0 / 0 / 36 | 0 | 是否单列 xyweb-lib | 用户例子：R3 → `orig.xyweb-lib` |
| O21 | `C:/Work/Web/orig-lagz/orig.www.lagzapper.com` | 35 | 1 | 35 / 0 / 0 | 232 | 是否统一为 www.lagzapper.com | 待确认 |
| O22 | `orig-web/orig.cddada7d.xunyou.com；E:/orig-web/cddada7d.xunyou.com` | 207 | 1 | 0 / 0 / 40 | 0 | 是否同一项目 | 远程按 R3：`orig.cddada7d.xunyou.com`；与本地合并待确认 |
| O23 | `D:/Go/src/marketing-client；marketing-client-component` | 68 | 1 | 0 / 0 / 0 | 0 | 替代还是独立；是否并 marketing-agency | 待确认 |
| O24 | `192.168.1.23/beyond/g` | 207 | 1 | 0 / 0 / 207 | 0 | 规范文档引用是否参与归因 | 待确认（采集时未当作当前项目） |
| O25 | `C:/Users/<user>/Documents/Codex/2026-10-15/wom` | 8 | 1 | 8 / 0 / 0 | 0 | 正式名；日期父目录是否忽略 | 待确认 |
| O26 | `Documents/海外；Documents/ChatGPT/阿卡贝拉Game；Documents/testcase` | 8 | 3 | 7 / 0 / 0 | 164 | 业务项目还是一次性工作 | 待确认 |
| O27 | `e:/tmp` | 198 | 1 | 197 / 0 / 0 | 0 | 需要什么仓库证据才能合并 | 用户明确：R5 `E:/tmp` + "文件" |

### 8.4 实际出现的仓库地址

10 个地址经核验（9 个来自整理后的见证，`orig.cddada7d.xunyou.com.git` 另由归档 `8c16cecfeb177ae68b1e0709d3f7d542` 见证）。出现不证明它就是该请求的主项目；SSH 用户部分已省略。

| 关联编号 | 地址 |
| --- | --- |
| B04 | `http://192.168.1.27/micro-perm/modules.git` |
| B06 | `http://192.168.1.27/micro-manager/perm-server.git` |
| B07 | `http://192.168.1.27/microservice/gateway.git` |
| O19 | `http://192.168.1.23/orig-web/orig.client-web-page.git` |
| O20 | `http://192.168.1.23/orig-web/orig.xyweb-lib.git` |
| O22 | `http://192.168.1.23/orig-web/orig.cddada7d.xunyou.com.git` |
| O04 | `ssh://192.168.8.6:30102/compass/component/process.git` |
| O05 | `ssh://192.168.8.6:30102/compass/component/screen-perception.git` |
| O06 | `ssh://192.168.8.6:30102/compass/runtime/llm-core.git` |
| O10 | `ssh://192.168.8.6:30102/atlas/service_sdk.git` |

### 8.5 已识别的误归因风险

| 形态 | 证据 | 需要的规则 |
| --- | --- | --- |
| Go 依赖缓存 | `.../go/pkg/mod/192.168.1.27/micro-activity/modules/v2@v2.44.0/...` 与 battlepass 同时出现 | 保留为依赖证据；不能仅因路径出现就把主项目改成 modules |
| 同会话多仓库 | compass / atlas / workshop，以及 perm-server / modules / gateway 同时出现 | 定义单主项目、拆分或多项目；不能默认取第一条或最高频路径 |
| worktree / 临时副本 | `.codex/worktrees/.../client-web-page`、`E:/tmp/imagegen-review-fixes` | 用可验证的仓库映射合并；不把哈希、tmp 或临时目录名作为正式项目 |
| 项目内子目录 | framework/src-tauri、editor_service/web、xunyou-datarouter/src | 回到确认的项目根；不能直接以当前 basename 作项目名 |
| 通用规范/基础设施 | beyond/g 文档、包仓库、登录页、API/部署地址 | 普通提及不当作当前项目 |

### 8.6 现行提取器的实证限制

逐条读取 6 个未归因见证请求的结构（只记录字段名、长度和标记字节位置）：

| 客户端 / endpoint | body 大小 | 发现 | 结论 |
| --- | --- | --- | --- |
| Codex Desktop `/v1/responses` × 2 | 1,036,424 / 2,227,673 bytes | 首个 `<cwd>` 位于第 393,193 / 532,258 字节 | 超出 300 KiB 头部窗口，`extractCodexAttribution` 漏判 |
| Claude Code（claude-cli vscode / cli）`/v1/messages` × 2 | 69,029 / 1,668,672 bytes | `Primary working directory:` 位于第 9,017 / 39,318 字节，但顶层 `system` 各文本块（94/9,254 字符；70/57/6,656 字符）都不含该标记 | body 是完整 JSON 时只看顶层 `system`，标记在消息里，漏判；不能把修复简化为只扩展顶层 system 正则 |
| GitHubCopilotChat/0.72.0 `/v1/chat/completions` | 173,929 bytes | 无 cwd 标记，第 37,138 字节起有 `<tagged_files>` | 没有 `tagged_files` 专用解析；POSIX 路径兜底只靠 `filePath=` 属性，普通 macOS 附件路径漏判 |
| ai-sdk openai-compatible `/v1/chat/completions` | 13,357 bytes | 无任何目录标记 | 无线索，属正常未归因 |

另外：已归因项目直接取 cwd basename，因此日期、任务名、构建子目录也可能成为项目名，识别命中不等于业务归因正确。

## 9. 新规则落地方案（建议，未实施）

当前归因发生在 `token_analysis_indexer.go` 读取原始 JSONL 时；规则文档不会被线上进程读取。目标流程：请求写入原始归档 → 后台增量索引提取有效线索 → 应用版本化分类规则 → 保存归因结果与简要依据 → `/admin/token-analysis` 展示项目/文件统计。

| 阶段 | 具体工作 | 完成标志 |
| --- | --- | --- |
| 1. 实现规则与提取 | 把 §7 实现为独立归一化模块；补 `tagged_files`、POSIX 路径、超过 300 KiB 的工作区标记及 Claude 消息内环境提取，按协议位置判断当前工作对象；规则与别名集中保存、带版本 | 范例和反例均有测试：后端项目边界、compass/xyai、orig-web、文件兜底、依赖缓存与多项目冲突 |
| 2. 保存可追溯结果 | 保存归因类型（项目/文件/未知）、归因值、有效路径/匹配来源、规则版本；规则版本与风险分类的 `cls_rule_version` 分开；类型参与分组/筛选，避免项目名与文件路径混桶 | 一条请求能解释"为什么归到这里"；旧记录明确为旧版本或待重算，不伪装成新版结果 |
| 3. 处理旧根目录 | `token_analysis_project_roots` 的旧记录不能绕过新规则；已学习根命中后仍须归一化；文件兜底路径不得作为可信项目根继续学习 | 旧 basename 映射不覆盖服务/项目、项目根或明确别名；普通文件不会被学成仓库 |
| 4. 改管理页 | 列表显示归因名称及"项目/文件"类型；文件类备注"文件"；详情保留来源路径，未归因显示缺线索或冲突原因 | 名称、筛选和汇总口径一致 |
| 5. 上线前预演 | 只读比较 11-02 的 2,690 条未归因记录及已归因对照样本，输出旧值→新值、规则、证据；验证大请求性能和数据库/前端合同 | 已归因样本没有被无关附件、依赖或普通目录错误覆盖；请求数、账单关联及 token/费用总额守恒，只允许分类分布变化 |
| 6. 发布并验证增量 | 兼容迁移 + 经过审核的后端、前端和同版本规则集；核对归档开关、实际索引配置和服务时区；验证新请求到页面的整条链路 | 新索引请求按新规则落库，旧数据可读；所有索引实例使用相同规则版本 |
| 7. 单独回填历史 | 新增项目归因专用的预演/批量更新入口，按日期与旧版本处理；先小范围核对再扩展 | 可暂停、续跑、复查及按前像撤回；只更新归因字段及规则元数据 |

新归档如何自动生效：

- `token_analysis_auto_index.go` 的自动任务启动后执行一次，再按间隔处理昨天和今天的新增行。开启归档、`index_enabled=true` 且 `auto_index_interval_seconds>0` 时，部署后新增索引行即走新规则，不需要新增每日全量 cron。轮询间隔不是可见延迟上限，大文件积压和失败会延后。
- "新增索引行"按索引水位定义：升级后首次处理的旧请求也会应用新规则；已越过水位的历史行不会因重启或普通"索引当前范围"而重新归因。
- 第一版采用随发布交付的版本化规则集。若要后台改规则立即生效，需另做持久化配置、版本发布和实例刷新；目前没有可直接粘贴规则的配置入口。

历史处理与性能边界：

- 已有可信工作目录/路径足够判断时直接重新分类；线索不足的再按 `source_file` + `source_line_start/source_line_length`（或行末 `source_offset`）读回原文，并核对 `archive_id` 与正文哈希。不需要默认重读全部 215 GiB。
- 回填前输出候选差异与数量，保存受影响归因字段的前像；写入以读取时的版本和源记录身份作条件，避免覆盖并发更新。原始归档保持原样。
- 不删除全局索引水位来触发全量重灌，也不调用普通摘要 upsert（会连带重建净输入、风险结果或账单关联）；归因专用更新不创建账单、不重新扣费。
- 采集证明存在有效路径，不代表精简后的候选证据足够对每条请求定案；需要协议上下文的记录仍须定向读原文。原文缺失或损坏且证据不足时保留未知，不补造项目。
- 规则调整后应能优先基于已持久化的线索重算派生结果；失败重试、故障恢复和归档容量治理遵循各自机制，项目分类升级不能当作归档 writer/索引水位问题已修复。

## 10. 代码地图

| 位置 | 内容 |
| --- | --- |
| `backend/internal/service/project_attribution.go` | `ProjectAttribution`、`ExtractProjectAttribution`、`extractAnthropicAttribution`/`extractCodexAttribution`/`extractCopilotAttribution`/`extractGenericAttribution`、`setWorkdir`、`AttributionProjectOf`、`validAttributionCwd`、`projectAttributionBlacklist`、`MatchAttributionKnownRoot`、常量 `projectAttributionHeadWindow`（300 KiB）、`ProjectAttributionSourceKnownRoot` |
| `backend/internal/service/project_attribution_test.go` | `TestExtractProjectAttribution*`（各客户端 fixture、截断降级、Copilot 已知根、多值行、脏 cwd、NUL） |
| `backend/internal/service/token_analysis_indexer.go` | `indexRange`（载入/落库已知根）、`tokenAnalysisProjectRoots`（`learn`/`match`/`flush`）、`indexArchiveLine`（调用提取器、写摘要字段） |
| `backend/internal/service/token_analysis_types.go` | `TokenAnalysisRequestSummary.Client*`、`TokenAnalysisFilters.Project`、`TokenAnalysisProjectUsage`、仓储接口 `ListProjectUsage`/`LoadProjectRoots`/`UpsertProjectRoots` |
| `backend/internal/service/token_analysis_service.go` | `ListProjectUsage` 透传 |
| `backend/internal/service/token_analysis_auto_index.go` | `StartAutoIndex`、`StopAutoIndex`、`autoIndexOnce` |
| `backend/internal/repository/token_analysis_repo.go` | `ListProjectUsage`、`tokenAnalysisProjectOrderBy`、`buildTokenAnalysisWhere`（Project 过滤）、`LoadProjectRoots`、`UpsertProjectRoots` |
| `backend/internal/handler/admin/token_analysis_handler.go` | `Projects` |
| `backend/internal/server/routes/admin.go` | `registerTokenAnalysisRoutes` |
| `backend/migrations/145_token_analysis_project_attribution.sql` | §5 |
| `frontend/src/views/admin/TokenAnalysisView.vue`、`frontend/src/components/admin/token-analysis/`（`TokenAnalysisProjectTable.vue`、`TokenAnalysisRequestTable.vue`、`TokenAnalysisRequestDrawer.vue`） | 项目筛选、项目排行、明细中的归因字段 |

## 11. 关键约束与易错点

- **规则：现行实现与新规则必须分开表述。** 依据：§7 尚未实现；文档、wiki 或页面若把新口径写成现状，会误导后续实现和数据解读。
- **规则：Codex/通用分支只扫描 body 前 300 KiB；Anthropic 完整 JSON 只看顶层 `system`。** 依据：曾复现的漏判——Codex 首个 `<cwd>` 在第 393,193/532,258 字节；Claude Code 的 `Primary working directory` 在消息里而不在顶层 system（§8.6）。扩窗口或扩搜索范围时须按协议位置区分"当前环境"和"引用的历史文本"，否则会把会话正文里的路径误当工作目录（`TestExtractProjectAttributionAnthropicIgnoresConversationQuote` 就是防这个）。
- **规则：Copilot 分支先折叠多重反斜杠再匹配，盘符路径要求左边界非字母数字，并过滤 `projectAttributionBlacklist`。** 依据：v1→v3 扫描曾把嵌套 JSON 里的 `\\\\` 路径、`https://` 截出的 `s://`、Copilot system prompt 示例路径（`Users/someone`、`pygorithm`）和用户配置目录误识为工作目录。
- **规则：含 NUL、换行或超长的 cwd 直接拒绝。** 依据：NUL 无法写入 Postgres text 列，曾导致整行永久失败（`TestExtractProjectAttributionRejectsNULBytes`）。
- **规则：`UpsertRequestSummary` 会覆盖归因四列。** 依据：§9 明确禁止借普通摘要 upsert 做归因回填——它会连带重算净输入、风险和账单关联；归因回填要走专用入口。
- **规则：Project 过滤精确匹配、区分大小写，`unattributed` 是保留值。** 依据：审核时发现手输项目名容易以为支持模糊搜索，已在筛选框占位文案写明"精确匹配"。已否决：后端 `ILIKE` 模糊搜索，原因是常用路径是从项目排行点击下钻，手输为辅，不值得为此加索引。
- **规则：采集计数各行不能相加，线索覆盖数不是准确率。** 依据：同一请求可命中多个候选，对话历史会重复携带路径（§8.2）。
- **规则：依赖缓存（`pkg/mod` 等）、工具示例、规范文档引用、历史工作区不能直接当主项目。** 依据：采集中 282 条请求带内部 Go 模块缓存，与 battlepass 同时出现（§8.5）。
- **规则：新规则下文件兜底路径不能写入 `token_analysis_project_roots`。** 依据：已知根会被 Copilot 前缀匹配复用，普通目录一旦被学成"仓库根"会把后续无关请求归错（§9 阶段 3）。
- **规则：目录匹配按完整路径段。** 依据：`compass-old`、`xyai-backup` 不应因前缀相同被归并（§7.5）；采集整理器也按此口径匹配。

## 12. 验证

### 12.1 命令

```bash
cd backend
# 提取器与索引
go test -tags=unit -count=1 ./internal/service -run "ProjectAttribution|TokenAnalysis"
# 仓储（项目聚合、Project 过滤、已知根）
go test -tags=unit -count=1 ./internal/repository -run TokenAnalysis
go test -tags=unit -count=1 ./internal/handler/admin -run TokenAnalysis
# 前端
cd ../frontend && npm run typecheck && npx vitest run src/views/admin/__tests__/TokenAnalysisView.spec.ts
```

Windows 上 Go 测试偶发 build 缓存或临时 `.test.exe` 被占用，换一个新的 `GOTMPDIR` 后重跑即可。

实现新规则（§9）时需新增：范例与反例单测（§7.3 全表、`compass-old`/`xyai-backup` 反例、依赖缓存、多项目冲突、大小写）；对 11-02 未归因样本的只读预演对比；token/费用总额守恒检查。

### 12.2 最近结果

提取器移植一致性（2026-06-07，2026-06-05 全天 10,856 请求，Go 提取器对照 Python v3）：

| 客户端 | Python v3 | Go 提取器（按时序） | 结论 |
| --- | --- | --- | --- |
| codex-cli | 99.8% | 99.8% | 一致 |
| claude-code | 99.6% | 99.6% | 一致 |
| copilot | 70.3%（全天根集合） | 68.5%（时序口径）；对未归因行用全天根集合二次解析可再补 203 条（≈72.9%） | 差异来自冷启动顺序，根跨天持久化后收敛 |
| 全口径（字节） | 90.0% | 89.9% | 一致 |

Go 版 instructions-md 层（多重反斜杠完全折叠）优于 Python 版，Copilot 直接命中 48.0%，高于 v3 同层。

线上（2026-11-02 全天）：68,019 条摘要中未归因 2,690（3.95%），见 §8.1；该数字反映"有无识别"，不反映归因名称是否符合 §7。

## 13. 状态与待办

- [x] 现行提取器、已知根、migration 145、项目聚合接口与页面（2026-06）。
- [x] 2026-11-03 未归因线索只读采集（§8）；用户给出新分类口径（§7）。
- [ ] 用户确认 §7.6 待确认项（尤其 atlas、workshop/tools、O22 合并、多项目请求口径、B11–B14 历史改名）。
- [ ] 按 §9 阶段 1–4 实现新规则：归一化模块、类型/规则版本/证据字段（需新 migration）、旧根处理、页面"项目/文件"类型。
- [ ] §9 阶段 5 只读预演并审核差异。
- [ ] 发布后验证增量链路；单独设计并执行历史回填（专用入口，可撤回）。
- [ ] 后续增强（未排期）：未归因行重扫、GitLab 仓库清单导入、客户端 Header 上报。
