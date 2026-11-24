# GPT 降智测试设计

更新：2026-11-24。状态：**前后端已实现（`feature/hy/10239_gpt_iq_test`，未提交）；已按 2026-11-23 Codex 实现审核修订；未上线，未做浏览器验收，未用真实 OpenAI 账号验证（§16）。默认关闭。**

本文是"降智测试"功能的唯一文档，面向 Claude 阅读。账号集合、展示名、分组与排序复用 GPT 账号额度展示（`docs/features/gpt-account-quota-display-design-cn.md`，下称"额度文档"），本文只写增量。2026-11-23 Codex 设计评审的采纳结论已并入正文，未采纳项写在 §15；评审原文在仓库外 `E:\tmp\xyai-gpt-iq-test-review-20261123\`。

阅读顺序：改上游请求读 §5、§6；改排程/并发/预算读 §7；改判分/判定读 §4、§8；改权限读 §9；改接口读 §10；定位代码读 §12。

## 1. 目标与范围

| 项目 | 规则 |
| --- | --- |
| 目的 | 每天定时对额度展示中的账号发固定题目，判断上游是否"降智"（答错、思考量骤降、失败），结果全员可见 |
| 结果展示 | `/gpt-quota` 额度区下方"降智测试"区；左迅游、右速宝，卡片 `id` 即额度条目 ID，展示名与顺序与额度卡片一致 |
| 可见范围 | 与 `/gpt-quota` 相同：通过模式门禁的登录用户，不按组织过滤，不开放匿名 |
| 配置页 | 管理后台"降智测试配置" `/admin/gpt-iq-test`；完整管理员、`admin.gpt_iq_test`、组织负责人可用（§9） |
| 测试项 | 多个；每项有题面、模型、思考强度、次数、判分规则；内置"打招呼 hi"与"糖果题"两个模板 |
| 自动化 | 每日定时一次（默认 08:30 Asia/Shanghai）；配置页上手动跑（全部/指定账号/指定测试项），结果逐格刷新 |
| 用户操作 | 只读，没有触发测试的入口 |
| 不做 | 不在服务器跑远程脚本或 Codex CLI；不测 API Key 账号与其他平台；不改账号调度状态 |

## 2. 名词

- **测试项（case）**：一道题。`chat` 只看能否正常回答（如 hi）；`graded` 有判分规则（如糖果题）。
- **版本（revision）**：测试项中影响结果解释的字段（题面、模型、思考强度、次数、超时、判分）的不可变快照。
- **批次（batch）**：一次定时或手动触发。全站同一时刻只有一个运行中批次。
- **调用（result 行）**：某账号 × 某测试项 × 第 i 次；重试复用同一行并累加 `attempts`。
- **判定（verdict）**：读取时由结果计算，不落库（§8）。

## 3. 账号集合与名称

- 账号集合 = 额度展示中 `selected` 且仍有资格的条目（额度文档 §2.1）；名字、`c-`/`d-` 归类、自然排序都调用额度服务的同一函数（`sortGPTQuotaEntries`、`effectiveDisplayName`、`GPTQuotaClass`）。本功能不单独选账号；增删账号、改别名只在 `/admin/gpt-quota`（仅完整管理员）。
- 依据：用户要求"结果和账号额度展示一样，名字也用一样的"；第二份账号列表会让两区错位。
- 条目不再可见后，用户列表与详情都不再返回它的历史；管理页批次详情对全站范围仍以 `#条目ID` 显示。
- 额度展示开关与降智开关独立：`GET /gpt-quota/status` 返回两者，任一开启就显示菜单；一边读取失败按"未开启"处理并记日志，不影响另一边，两边都失败才返回错误。

## 4. 测试项

### 4.1 字段与版本

| 字段 | 规则 |
| --- | --- |
| `name` | 1–40 字符，无控制字符（可变） |
| `enabled`、`sort_order` | 可变；停用项不进定时批次，可被手动显式指定（试运行） |
| `public_prompt`、`public_answer` | 可变；用户侧**始终按当前开关**过滤，包括历史结果 |
| `kind` | `chat` / `graded` |
| `prompt` | 1–8000 字符，`\r\n` 归一为 `\n`；拒绝 `sk-`、Bearer 令牌、邮箱 |
| `model` | `[A-Za-z0-9._:/-]`，≤100；执行时经 `GetMappedModel` + `normalizeOpenAIModelForUpstream` |
| `reasoning_effort` | 空或 `none/minimal/low/medium/high/xhigh/max/ultra`（只做枚举校验，不按模型描述校验） |
| `runs` | 1–5，同账号内串行 |
| `timeout_seconds` | 30–600；0 取默认（chat 60、graded 300） |
| `scoring_mode` / `scoring_expected` | §4.3；chat 强制 `none`；`final_line`/`number_any` 去首尾空白，`contains`/`regex` 原样保存（可能依赖空白），全空白拒绝 |

修改测试项时：可变字段原地更新；版本字段有任何变化就新建版本（`revision+1`），`current_revision_id` 指向新版本。结果行记录执行时的 `revision_id`、判分时的 `scored_revision_id`。删除为软删除。测试项与配置都有乐观锁（`version`）。

### 4.2 模板

管理页"从模板添加"只做表单预填（`GPTIQCaseTemplates`）：

| key | 内容 |
| --- | --- |
| `hi` | chat，题面 `hi`，模型 `openai.DefaultTestModel`，1 次 |
| `candy` | graded，`gpt-6.1-sol`、`high`、3 次、`final_line` 期望 `21`；题面按原题玩法自行表述（附数量表），末尾要求"最终答案＝<整数>" |

糖果题标准答案 **21**（2026-11-23 用户定案）：形状靠手感可分辨，可以按形状挑——取 9 颗圆形（西瓜圆形只有 8 颗，必有苹果或桃）+ 12 颗星形（苹果+西瓜 ≤11、桃+西瓜 ≤10，必同时有苹果和桃），共 21 颗必能配对；20 颗及以下都有反例。按"随机摸、不能挑形状"理解才是 29，那不是本题玩法。要与 `haowang02/codex-candy-eval` 脚本逐字对照时，粘贴其原题并改用 `number_any`；该仓库未声明许可证，仓库内不复制其代码。

### 4.3 判分（`gpt_iq_test_scoring.go`）

| `scoring_mode` | 规则 |
| --- | --- |
| `final_line` | NFKC 后只看**最后一个非空行**（去掉两端 `*_`#>` 与空格）：该行必须整行是 `最终答案[:：=＝]X` 或 `Final answer: X`；期望为整数时 X（去尾部句号）必须**完整**解析为该整数，`21.5`、`不是21，是29`、`21 颗`、答案行后再追加更正都判错；否则比较 X 与期望文本。没有这一行判错 |
| `number_any` | 与脚本 `(?<!\d)21(?!\d)` 等价：原文（不做 NFKC）中出现期望文本，且前后不是 Unicode 十进制数字。全角 `２１` 与脚本一样不匹配 |
| `contains` | NFKC 后大小写不敏感包含 |
| `regex` | Go RE2 |

- Go RE2 不支持后行断言，脚本正则不能原样作为 `regex` 使用（保存时会被拒绝）。
- `number_any` 很宽松（"不是 21，是 29"也算对），只用于脚本对照；正式测试用 `final_line`。"取最后一个数"已否决：会把"答案 21，验算 9 + 12"判成 12。
- 只有 `status=ok` 的调用评分；`correct` 与 `status` 分开存储，重评只改 `correct`/`scored_revision_id`。
- **重新判分**：用测试项当前版本的规则，重算同一题面（`prompt_sha256` 相同）且**执行时就是判分题**（执行版本 `kind=graded`）的 ok 回答，不访问上游；执行时为 chat 的连通性结果即使题面相同也保持未评分。`answer_truncated` 的回答证据不足，保持原分并计入 `skipped_truncated`。写分版本单调（`scored_revision_id <= 目标`），返回的 `updated` 是实际以目标版本判分的行数（`CountScoredWith`），被更新版本挡下的旧重评回报 0。执行时总是用完整回答判分，存储上限 64 KB。

## 5. 糖果测试：脚本与服务端复刻

### 5.1 脚本行为（2026-11-23 读取，未执行）

`wget -qO- https://is.gd/codex_candy | python3 - -m gpt-6.1-sol -r high -n 3` 跳转到 `haowang02/codex-candy-eval` 的 `codex_candy_eval.py`：子进程执行本机 `codex exec --json --skip-git-repo-check -s read-only --disable memories -c model_reasoning_effort=<r> -c notify=[] [-m <model>]`，题面走 stdin；读最后一个 `agent_message` 为答案、`turn.completed.usage` 为 token；表格列 Run / 答案预览 / In / Out / Reason / Time / TPS / OK，失败行不评分；回答中出现独立 `21` 即对。

### 5.2 不在服务器跑脚本

`curl | python` 执行远程最新代码；要在服务器装并登录 Codex CLI；CLI 走自己的账号或中转，**无法指定测某个上游账号**。

### 5.3 服务端等价请求（`buildGPTIQProbeRequest`）

| 字段 | 取值 |
| --- | --- |
| URL / Host | `chatgptCodexAPIURL` / `chatgpt.com`（同测试连接 OAuth 分支） |
| `instructions` | `openai.CodexBaseInstructionsForModel(model)`（测试连接用 `DefaultInstructions`） |
| `input` | 单条 user `input_text` = 题面 |
| `reasoning` | `{effort}`，未配置不发 |
| `stream` / `store` | `true` / `false` |
| `prompt_cache_key` | 每次随机 UUID |
| 头 | `Authorization`、`accept: text/event-stream`、`OpenAI-Beta`、`resolveCodexOutboundIdentity` 的 Originator/UA（账号自定义 UA 优先）、`setOpenAIChatGPTAccountHeaders`、`enforceCodexIdentityHeadersWithUA`、`ApplyHeaderOverrides` |
| 出站 | `doOpenAIAccountTestUpstream`（插件链 + TLS 指纹）、账号代理、`HTTPUpstreamProfileOpenAI` |

指标：`response.completed.response.usage` 的 input/cached/output/reasoning tokens（缺失为 NULL，不当 0）、`response.model`；耗时 = 发出到 completed，首字 = 首个 `output_text.delta`；TPS = 输出 token / 耗时（读时计算）。没有增量时回退到 completed 中最后一条 message 的 `output_text`。

已知差异：不带 Codex 工具、`environment_context`、AGENTS.md，不发 `text.verbosity`；走 HTTP SSE 而非 WebSocket；耗时口径不同。结果与本机跑脚本不必逐位一致。已否决"完全保真"（附工具与环境上下文）：工具定义随 CLI 版本变化，对纯推理题收益小。

## 6. 执行器与无副作用

### 6.1 与测试连接的关系

`testOpenAIAccountConnection` 不能直接复用：绑定 `gin.Context` 输出 SSE；OAuth 路径 `createOpenAITestPayload` **固定发 hi、忽略 prompt**、不发 reasoning；有副作用（`extractOpenAICodexProbeUpdates`→`UpdateExtra` 写调度读的 `codex_5h_*/codex_7d_*`、401 `SetError`、429 `reconcileOpenAI429State`）。执行器在本地新文件 `gpt_iq_test_runner.go` 只调用其辅助函数，不修改上游文件 `account_test_service.go`；单测 `TestGPTIQProbeMatchesAccountTestHeaders` 对比两者的 URL、Host 与身份头，上游合并改了测试连接头时会失败。测试连接忽略 prompt 属上游行为，本功能不修。

### 6.2 无副作用（硬约束）与凭证例外

- 不写 `accounts.extra`，不调用 `SetError`、`BlockAccountScheduling`、`SetRateLimited`、`reconcileOpenAI429State`；不进 `usage_logs`、不计费、不进请求归档。
- **token 预检**（`gptIQTokenUsable`）：access token 已过期（含刷新提前量）且无 refresh token 时记 `token_unavailable`，不调用 token provider。依据：provider 此时会 `disableAccountMissingRefreshToken` 禁用账号（额度文档 §4.1）。
- **允许的例外**：经 `OpenAITokenProvider.GetAccessToken` 取 token，临近过期且有 refresh token 时会正常刷新并持久化新凭证（与额度采集和网关相同）。刷新失败只记日志、不标错误（`oauth_refresh_api.go`）。已否决"只读凭证、过期即不可测"：闲置展示账号会永久不可测。
- 执行前的额度读取复用额度展示的 `refreshAccount`（只读 wham/usage，写额度快照表，不写调度键，见 §7.4）。

### 6.3 结果状态与失败分类

`status`：`running`、`ok`、`error`、`skipped`、`cancelled`、`abandoned`。回答、错误摘要、模型名写库前去 NUL 与非法 UTF-8（服务层与仓储各一次）；`FinishResult` 非执行权错误记日志并重试一次，避免结果停在 running 被收尾改成 cancelled 而丢失真实失败。`reason` 为失败类别（沿用额度文档 §6.2，另有 `stream_incomplete`、`empty_answer`、`response_failed`）或跳过原因（§7.3）。管理页可见 `error_summary`（去 Bearer/`sk-`/邮箱/≥32 位串，≤500 字符）与 `http_status`；用户侧只见类别。

- 超时、5xx、传输错误、`stream_incomplete`：30 秒后重试 1 次（每次派发各占预算，§7.2）。
- 401/403/`token_unavailable`/`not_supported`：不重试；该账号本批次其余调用记 `skipped:after_auth_failure`。
- 429：不重试；按 Retry-After（缺省 5 分钟，上限 24 小时）写 `gpt_iq_test_backoff`，其余调用记 `skipped:after_rate_limit`；退避期内定时与手动都整账号跳过。

## 7. 排程、预算与并发

### 7.1 每日定时

- 时区固定 Asia/Shanghai（`gptQuotaDisplayLocation`）。`schedule_enabled` 默认 false；`run_time` 默认 08:30，可配 05:00–11:00；有效窗口 `run_time` 至 +3 小时，重启在窗口内补跑一次，过窗口当天不跑。
- 每 30 秒：回收心跳超时批次 → 每实例每天一次保留期清理（与是否跑批次无关）→ 定时检查。定时检查在 leader lock 下：读条目与启用测试项 → **先**插入运行中批次（部分唯一索引保证全站一个）→ **再**条件领取当日（`last_run_date` 只进不退）；领取失败把批次标 `aborted`。手动批次在途时插入失败，不消耗当日，窗口内重试。无账号或无启用项时只领取当日。
- 开启定时或修改 `run_time` 时若当日时刻已过，保存事务把 `last_run_date` 抬到今天，不隐式补跑。
- 已否决：定时批次中断后自动续跑同一批次或"结果未知调用"单列。原因：频率低，被回收的批次在页面显示为未完成，手动重跑即可；令牌栅栏已防止旧执行者回写。

### 7.2 每日派发预算

`gpt_iq_test_daily_usage` 按上海日期计数，**每次实际派发前原子预占**（`ReserveDispatch` 条件 upsert，`dispatched < budget`），首发与重试各占一次；预检拦截（未发出）的调用归还名额；跳过不占名额。预占失败：首发记 `skipped:budget` 并让本批次剩余调用都记 `budget`；重试时预占失败保留首发的失败。手动触发时当日已用满返回 409。默认 300/天。

### 7.3 账号跳过规则（`entrySkipReason`，按顺序）

`account_unavailable`（取不到账号）→ `not_supported`（资格不符）→ `account_error`（非 active）→ `account_rate_limited`（`rate_limit_reset_at`/`overload_until`/`temp_unschedulable_until` 未到）→ `backoff`（本功能 429 退避）→ `token_unavailable` → `low_quota`。手动批次另有同（账号, 测试项）5 分钟冷却 `cooldown`。**暂停调度（`schedulable=false`）的账号照常测试**，便于观察恢复。

### 7.4 额度保护

测试前对账号调用额度展示的 `refreshAccount`（冷却 60 秒、429 退避、singleflight 都沿用），再按**刷新状态 + 快照**判断（`quotaCheck` 返回足够/不足/未知）。额度展示失败时会保留上次成功快照，所以不能只看快照：本次刷新失败、处于 429 退避、或冷却中但最近一次尝试不是成功，一律**未知，跳过本账号（`quota_unknown`），不调模型、不占预算**；只有刷新成功，或正常 60 秒冷却且最近一次尝试成功时，才用快照。快照缺失或采样超过 15 分钟同样未知；窗口 `reset_at` 已过而采样早于重置，旧窗口失效，按未知处理；5 小时剩余 < `min_five_hour_percent`（默认 10）或 7 天剩余 < `min_seven_day_percent`（默认 5）为不足（`low_quota`）；上游未提供的窗口不参与判断；两个阈值都为 0 时不做额度保护。依据：08:30 时额度采集（09:30 起）尚未开始，前一日快照可能已重置；只开降智时根本没有定时采集；额度未知时继续测试会消耗真实账号额度（实现审核 P1-3、复审 P1）。

### 7.5 并发、心跳与执行权

- 跨账号并发 3；账号内测试项按 `sort_order`、次数按序号串行。批次上限 90 分钟。
- 批次带 `lease_token`。结果插入、结果完成、心跳、终态更新都要求 `status='running' AND lease_token=本令牌`；失败返回 `ErrGPTIQLeaseLost`，执行者立即停止。完成后的结果行不能再被覆盖。
- 心跳每 10 秒；数据库时钟下超过 10 分钟无心跳的运行中批次标 `abandoned`，其运行中结果标 `abandoned`，释放全站占用。
- 取消：设置 `cancel_requested`，心跳发现后取消批次 context，在途调用记 `cancelled`，不再开始新调用，批次标 `cancelled`。`Stop` 取消根 context、或批次到达 90 分钟上限时，批次标 `aborted`（不是 `finished`）。批次 context 结束后在途调用的任何失败都改记 `cancelled`（含预占预算时被取消），不算账号失败。

## 8. 降智判定（读时计算，`gptIQVerdict`）

结果按（账号, 测试项, 批次）分组为一组调用。**运行中批次的组不参与卡片、趋势与基线**。`cancelled`/`abandoned` 计为中断（`interrupted`），不算账号错误。卡片对每个启用测试项取最近一次**有实际调用（ok/error）**的那组（手动复测显示为"复测"；全部跳过的复测不覆盖之前的结论）；14 天趋势**只取定时批次**，当天没有定时结果就没有该日的点。每组带执行时的版本元数据（`revision`），历史结果不套用当前题面与参数。

| 判定 | 条件（按顺序） |
| --- | --- |
| `untested` | 没有结果 |
| `skipped` | 有效调用为 0 且有跳过 |
| `failed` | 有效调用（ok+error）都是上游失败（`error`）；只有中断不算失败 |
| `suspect` | 任一 graded 组在可评分数 ≥ `min_graded_runs` 时正确率 < `min_accuracy_percent`；或该组思考 token 中位数 < 同批次同版本各账号中位数的中位数 × `reasoning_ratio_percent`（需 ≥3 个账号有数据） |
| `insufficient` | 有 graded 组可评分数 < `min_graded_runs`（例如 1 对 2 超时），或只有中断（原因 `interrupted`） |
| `connected` | 只有 chat 组成功，没有可评价的 graded 组 |
| `normal` | 其余 |

默认阈值：正确率 60%、最少 2 次可评分、思考比例 30%。原因（`reasons`）给前端拼文案：`low_accuracy`（correct/graded）、`low_reasoning`（percent）、`insufficient_samples`、`all_failed`（detail=最常见类别）、`chat_failed`。`response.model` 只记录不参与判定（避免带日期后缀误报）。思考 token 缺失为 NULL，不参与基线。

## 9. 权限

- 两项能力分开（`GPTIQTestActor`）：`CanConfigure`（全站配置、测试项增改删、重新判分）与运行范围（`FullScope` 或 `Groups`）。
- 权限码 `admin.gpt_iq_test`（菜单键 `gpt_iq_test`，路由 `/admin/gpt-iq-test`）放行全部 9 个路由；`admin.organization_manage` 只放行 4 个（`gptIQTestOrganizationRouteRules`：GET 配置页、POST runs、GET runs/:id、POST runs/:id/cancel），**不改 I2 捆绑**，不回填数据。
- 服务层每次按 `DepartmentActorID` 重读用户（`LoadActor`）：`admin` 或完整管理员 API Key → 可配置 + 全站；持 `admin.organization_manage` 的组织负责人 → 范围为其 `organization_access_grants`，另持 `admin.gpt_iq_test` 时才可配置，**范围不扩大**；其他持 `admin.gpt_iq_test` 的子管理员 → 可配置 + 全站；其余 403 `GPT_IQ_TEST_SCOPE_DENIED`。受限 Admin API Key 未登记这些路由。
- 组织负责人（2026-11-23 实现审核 P1-1/P1-2 定案）：只持组织权限时可查看配置页（只读）、运行本组织、取消本人发起的手动批次；配置保存、测试项写入与重评在服务端拒绝（`requireGPTIQConfigure`），页面按 `scope.can_configure` 隐藏。手动触发的"全部"解析为本组织条目，指定组织外条目 403。配置页的账号列表（`entries`）**只返回运行范围内的条目**，其他组织的条目 ID、展示名与分组都不返回（公开结果页另有全员可见的规则）；最近批次只列包含其组织条目的批次，`entry_ids`、`total_calls`、计数都投影到其范围，发起人只在是本人时返回；退避只给本组织条目；批次详情同样投影，不含其条目的批次 404。不返回账号 ID 或原名。
- 审计：`POST /cases`、`PUT /cases/:id` 在 `auditBodyPromptDigestRoutes`：捕获请求体后由 `GPTIQAuditPromptDigest` **按白名单重建**再脱敏，从不交回原始请求体。键名按 encoding/json 规则大小写不敏感匹配（`Prompt`、`PROMPT` 同样会被绑定成题面）；题面字符串换成 `sha256:<hex>`（与版本表 `prompt_sha256` 同一规范化），非字符串题面换成 `<omitted>`；白名单字段只保留标量；未知字段丢弃并计数；不是 JSON 对象时只记 `{"_body":"<omitted>"}`。校验失败的提交同样只留摘要或省略标记（中间件测试 `TestGPTIQCaseAuditKeepsOnlyPromptDigest`）。

## 10. 接口

| 方法与路径 | 权限 | 行为 |
| --- | --- | --- |
| `GET /api/v1/gpt-quota/status` | 登录用户 | `{enabled, iq_test_enabled}`（改由 `GPTIQTestHandler.Status` 处理） |
| `GET /api/v1/gpt-quota/iq-tests` | 登录用户 | `enabled, server_time, poll_interval_seconds=900, schedule{enabled,run_time,timezone}, next_run_at, thresholds, cases[]（public_prompt 时含 prompt）, groups.xunyou/wsdashi[]`；卡片 `id, display_name, verdict, reasons[], tested_at, cases[]（组：runs[], total/ok/errors/skipped/graded/correct, median_reasoning_tokens, avg_duration_ms, avg_tps, trigger, run_date）, trend[]`；run 的 `answer_preview` ≤120 字（`public_answer=false` 时省略）。关闭时分组为空 |
| `GET /api/v1/gpt-quota/iq-tests/entries/:entry_id?case_id=&days=` | 登录用户 | 最近 `days`（≤30，默认 14）天全部组，新批次在前，`public_answer` 时含全文；条目不可见 404 |
| `GET /api/v1/admin/gpt-iq-test` | §9 | `config, server_time, timezone, next_run_at（当天未领取且仍在窗口内时为今天的计划时刻）, scope, cases[]（含题面与版本）, templates[], entries[]{id,display_name,group,runnable,backoff_until}, batches[]（最近 10 个，带计数）, running_batch_id, dispatched_today` |
| `PUT /api/v1/admin/gpt-iq-test/config` | §9 | 带 `expected_version`；409 `GPT_IQ_TEST_CONFIG_CONFLICT`；400 `GPT_IQ_TEST_INVALID` |
| `POST /cases`、`PUT /cases/:id`（带 `expected_version`）、`DELETE /cases/:id?expected_version=` | §9 | 409 `GPT_IQ_TEST_CASE_CONFLICT`、404 `GPT_IQ_TEST_CASE_NOT_FOUND` |
| `POST /cases/:id/rejudge` | §9 | `{checked, updated, skipped_truncated}`；chat 项 400 |
| `POST /runs` | §9 | `{entry_ids?, case_ids?}` → 202 批次；409 `GPT_IQ_TEST_BATCH_RUNNING` / `GPT_IQ_TEST_DAILY_BUDGET_EXCEEDED`；403 范围；400 `GPT_IQ_TEST_NOTHING_TO_RUN` |
| `GET /runs/:id` | §9 | 批次 + 逐条结果（回答 ≤2000 字、脱敏错误摘要、`entry_name`、`group`、`tps`），按范围过滤；`visible_total_calls` 为范围内计划调用数（负责人看全站批次时用作进度分母） |
| `POST /runs/:id/cancel` | §9 | 409 `GPT_IQ_TEST_BATCH_NOT_RUNNING`；负责人取消他人批次 403 |

**读路径零上游调用**：用户接口、配置页读取、批次详情都只查库，不触发测试或额度采集（单测 `TestGPTIQUserViewVisibilityAndNoUpstreamCalls`）。

## 11. 数据模型（`backend/migrations/265_gpt_iq_test.sql`）

| 表 | 要点 |
| --- | --- |
| `gpt_iq_test_config` | 单例；开关、`run_time`、`last_run_date`、判定阈值、额度阈值、`daily_call_budget`、`retention_days`、`version` |
| `gpt_iq_test_cases` | 可变属性 + `current_revision_id`（可延迟 FK）+ `version` + `deleted_at` |
| `gpt_iq_test_case_revisions` | 不可变版本；`UNIQUE(case_id, revision)`；`prompt_sha256` |
| `gpt_iq_test_batches` | `trigger`、`run_date`、`triggered_by`、`scope_groups`、`entry_ids`、`case_ids`、`status`、`failure_reason`（`timeout`/`stopping`/`persist_failed`）、`lease_token`、`cancel_requested`、`heartbeat_at`、`total_calls`；部分唯一索引 `((TRUE)) WHERE status='running'` |
| `gpt_iq_test_results` | `UNIQUE(batch_id, entry_id, case_id, run_index)`；`entry_id` FK 额度条目（CASCADE）；`revision_id`/`scored_revision_id`；`status`/`reason`/`correct` 分列；回答 ≤64 KB + `answer_truncated`（写入前去 NUL）；token/耗时为可空整数 |
| `gpt_iq_test_daily_usage` | `run_date` 主键，`dispatched` |
| `gpt_iq_test_backoff` | `account_id` 主键（FK accounts CASCADE），`retry_after` |

不存完整上游 JSON、响应头、加密思考内容、token、邮箱。保留期默认 90 天，按 `run_date` 清理结果、非运行批次、预算行与过期退避。

## 12. 代码地图

| 层次 | 位置 |
| --- | --- |
| 迁移 | `backend/migrations/265_gpt_iq_test.sql` |
| 服务 | `service/gpt_iq_test_service.go`（`GPTIQTestService`、排程 `runScheduled`、`executeBatch`/`runEntry`/`probeWithRetry`/`applyProbe`、`entrySkipReason`、`quotaLow`、管理操作）、`gpt_iq_test_views.go`（`UserView`、`EntryDetail`、`AdminView`、`BatchDetail`、`gptIQVerdict`、`gptIQTrend`、`gptIQBaselines`）、`gpt_iq_test_scoring.go`（`NormalizeGPTIQCaseInput`、`ScoreGPTIQAnswer`、`GPTIQCaseTemplates`）、`gpt_iq_test_runner.go`（`gptIQUpstreamProber`、`buildGPTIQProbeRequest`、`parseGPTIQStream`、`gptIQTokenUsable`）。注意：文件名不能以 `_test.go` 结尾 |
| 仓储 | `repository/gpt_iq_test_repo.go`（`LoadActor` 复用 `loadOrganizationGrants`） |
| 权限 | `service/admin_permission.go`：`AdminPermissionGPTIQTest`、`gptIQTestAdminRouteRules` |
| Handler/路由 | `handler/gpt_iq_test_handler.go`；`server/routes/user.go`（`/gpt-quota/status`、`/gpt-quota/iq-tests*`）、`admin.go`（`/admin/gpt-iq-test*`）；审计 `server/middleware/audit_log.go` |
| Wire | `ProvideGPTIQTestService`（构造即 `Start()`）；`cmd/server/wire.go` 的 `provideCleanup` 调 `Stop()`；`wire_gen.go` 已用 wire 重新生成（与先前手工修改逐字一致），生成后还原 `go.sum`；`cmd/server/wire_gen_test.go` 按位置传参，增减清理参数要同步 |
| 前端 | `api/gptIQTest.ts`、`api/admin/gptIQTest.ts`、`utils/gptIQTest.ts`；用户区 `components/user/GPTIQTest{Section,Column,Card,RunsDrawer}.vue`、`GPTIQTrendStrip.vue`（挂在 `views/user/GPTQuotaView.vue`，与额度区共用 15 分钟计时器、各自请求与错误状态）；配置页 `views/admin/GPTIQTestConfigView.vue` + `components/admin/gpt-iq-test/{CaseEditorDialog,RunPanel,BatchResultTable}.vue`（运行中 3 秒轮询批次）；菜单 `composables/useGPTQuotaVisibility.ts`（两开关分开记录：`setGPTQuotaDisplayVisibility` / `setGPTIQTestVisibility`）；路由 `meta.adminPermissionsAny = [admin.gpt_iq_test, admin.organization_manage]`；`utils/adminPermissions.ts` 落地页；`i18n/locales/{zh,en}/gptIQTest.ts`。模型文本一律纯文本 `pre-wrap`，无 `v-html` |

## 13. 关键约束与易错点

1. **读路径零上游调用**，包括额度采集。
2. **执行器无副作用**：从测试连接抄代码时最容易把 `UpdateExtra`、401 `SetError`、429 状态同步一起抄过来。
3. **token 预检先于 token provider**。
4. **预算按实际派发原子预占**，不按结果行计数；未发出要归还。
5. **先占批次、后领当日**；领取失败中止批次。
6. **所有写入带执行权令牌**；不要为"简化"去掉 `lease_token` 条件。
7. **判定读时计算**，阈值改动影响历史；`correct` 与 `status` 分开。
8. **重评只针对同题面且未截断的回答**；版本字段变化必须新建版本。
9. **公开开关按当前值过滤历史**：隐藏回答时预览与全文都省略。
10. **账号列表只来自额度条目**；配置页不返回账号 ID/原名（组织负责人可见）。
11. **组织范围在服务层重读**，"全部"解析为操作者范围；不信任前端条目列表。
12. **pq 数组的 nil 切片会编码成 NULL**，写 NOT NULL 数组列前换成空切片（集成测试曾因此失败）。
13. **服务文件不能命名为 `gpt_iq_test.go`**（以 `_test.go` 结尾会被当成测试文件）。
14. 测试消耗共享账号真实额度：默认关闭、额度保护、每日预算三者都不能省。
15. **批次到达上限不是"完成"**：要检查批次 context 的 `DeadlineExceeded`，在途调用记 `cancelled`。
16. **卡片不能取运行中或全部跳过的批次**，否则冷却内复测或进行中的批次会改写结论。
17. **所有写入 text 列的上游内容都要去 NUL**（不只是回答），否则 UPDATE 失败、结果被收尾改成 cancelled。
18. **预占名额时校验执行权**（`ReserveDispatch` 同事务 `FOR SHARE` 批次行，令牌/状态/取消标记），心跳之前被回收的旧执行者不能再派发。
19. **写结果的非执行权错误要传到批次**：批次标 `aborted` + `persist_failed`，不是 `finished`。
20. **重评写分版本单调**（`scored_revision_id <= 目标`），并发重评不会倒退；页面带 `revision_id`，过期返回 409。
21. **思考 token 基线按实际请求模型分组**，账号模型映射不同的不互相比较。
22. **`final_line` 只认最后一个非空行且值完整解析**，不要回退到"往上找答案行"或"取第一个整数"。

## 14. 验证

```bash
cd backend
go test -tags=unit -count=1 ./internal/service -run 'GPTIQ|AdminPermission|GPTQuota'
go test -tags=unit -count=1 ./internal/server/...
# PostgreSQL：DSN 指向新建空库（本机可用 E:/tmp/epg 的嵌入式 PG18，端口 55418）
SUB2API_POSTGRES_ONLY_INTEGRATION_DSN=... go test -tags integration -count=1 -run 'TestGPTIQTestRepository|TestMigrationsRunner' ./internal/repository/
cd ../frontend && pnpm run typecheck && pnpm exec vitest run
```

覆盖：判分（`final_line` 各形态、`number_any` 与脚本语义、全角、`121`/`21.5`）、输入校验与模板、SSE 解析（usage 缺失为 nil、回退文本、failed、截断）、请求头与测试连接一致、401/429/5xx 无账号副作用且摘要脱敏、token 预检不调 provider；批次存储与评分、重试与预算（余额 1 + 重试、并发争名额）、未发出归还、401/429 后跳过与退避、账号跳过规则、额度保护（新鲜采样、重置已过、额度未知跳过且不调模型不占预算、刷新失败或退避时不用旧快照）、组织范围（全部解析、越权 403、详情过滤、账号列表只含本组织）、取消范围、执行权丢失停止写入、手动冷却、定时窗口与当日领取、手动在途不领日期、保存配置不补跑、判定各分支与按实际模型分组的思考基线、仅定时批次进入趋势、用户视图可见性与零上游调用、重评（新版本、截断跳过、改题面不重评、历史 chat 不重评、回报实际写入数）、审计只留题面摘要、状态组合；仓储集成（配置/日期、版本不可变、单运行批次、令牌栅栏、心跳/取消/回收、预算 16 路并发、清理、操作者范围）。

| 时间 | 项目 | 结果 |
| --- | --- | --- |
| 2026-11-23（本机） | `go build ./...`、`go vet` 改动包 | 通过 |
| 2026-11-23 | `-tags=unit` service `GPTIQ`（31 个测试函数，`-race -count=20`）、`AdminPermission`、`GPTQuota`、`internal/server/...` | 通过 |
| 2026-11-23 | 后端全量 `go test -tags=unit ./...`；`golangci-lint v2.14 --new-from-rev=HEAD`（service/repository/handler/server/cmd） | 全部通过（首轮发现 `cmd/server/wire_gen_test.go` 需补 `provideCleanup` 新参数、2 处 errcheck，已修）；0 issues |
| 2026-11-23 | 前端 `pnpm run typecheck`、改动文件 eslint、`vitest run` 全量（448 文件 / 3758 用例） | 通过 |
| 2026-11-23 | 自审（code-review high）9 项：批次上限误标完成、卡片取运行中/全跳过批次、结果写入失败被吞与 NUL、预算预占时取消误记、状态接口不独立、负责人进度不到 100%、窗口内下次运行显示明天、regex/contains 期望值被去空白、wire_gen 手工修改 | 全部修复并补回归测试；`GPTIQ` 用例 `-race -count=20`、前端相关 spec、PG18 集成测试通过 |
| 2026-11-24 | 实现审核修复后全量回归：后端 `go test -tags=unit ./...`、`GPTIQ` `-race -count=10`、lint 0 issues、PG18 集成（新建空库）、前端 typecheck 与 vitest 448 文件 / 3771 用例 | 通过；唯一失败 `TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode` 在基线 `main@b1a869303` 干净 worktree 上同样 2/5 失败（上游 0.2.24 自带的计时类不稳定用例，与本功能无关） |
| 2026-11-24 | Codex 复审 5 项（P1 刷新失败或退避时仍用旧快照派发、P1 审计摘要非标准写法回退原文、P2 负责人账号列表含外组织、P2 历史 chat 被重评成判分、P3 并发重评更新数虚高） | 全部修复并补回归用例；Codex 上轮 9 个与本轮 5 个独立断言全部通过；后端全量单测（仅既有不稳定用例 `TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode` 失败）、`GPTIQ` `-race -count=10`、lint 0 issues、PG18 新建空库集成测试通过 |
| 2026-11-23 | Codex 实现审核 13 项（P1：负责人越权编辑、双重授权扩大范围、额度未知仍消耗、糖果判分误判；P2：负责人响应带其他组织数据、回收后旧进程仍派发、中断标成账号失败、写结果失败仍标完成、手测进入正式趋势、历史配当前题面、旧重评覆盖新分、模型映射使思考比较失真；P3：审计缺题面摘要） | 全部修复；Codex 的 9 个独立断言全部通过；新增回归用例；PG18 集成测试通过 |
| 2026-11-23 | 嵌入式 PG 18 新建空库：`TestGPTIQTestRepository*`（3 个）、`TestMigrationsRunner*`、`TestGPTQuotaDisplayRepository*` | 通过 |

## 15. 决策记录

| # | 决策 | 状态 |
| --- | --- | --- |
| Q1 | 糖果题答案 21，正式判分 `final_line`；`number_any` 只用于脚本对照 | 用户 2026-11-23 定案 |
| Q2 | 组织负责人只能查看、运行本组织、取消本人手动批次；全站配置、题目写入与重评需完整管理员或 `admin.gpt_iq_test`（负责人另持时可配置但范围仍限本组织） | 2026-11-23 实现审核定案 |
| Q3 | 降智开关独立于额度展示开关 | 采纳 |
| Q4 | 每天一次，默认 08:30 | 采纳 |
| Q5 | 回答默认全员可见，按测试项可关 | 采纳 |
| Q6 | 401 只记录，不标账号错误 | 采纳 |
| Q7 | hi 默认 `openai.DefaultTestModel` | 采纳 |
| Q8 | 判定变化邮件通知负责人 | 第二期 |

评审采纳（已并入正文）：派发预占预算（§7.2）、负责人范围与取消（§9）、隐藏回答同时隐藏预览（§4.1、§10）、额度新鲜度（§7.4）、最少可评分次数与 `insufficient`（§8）、不可变版本与按题面重评（§4）、执行权令牌与回收（§7.5）、执行跳过规则（§7.3）、趋势优先定时批次（§8）、菜单双开关（§3）、仅新权限的登录落地（前端）、审计不记题面与独立清理（§9、§11）、模板可直接运行（§4.2）。

已否决：自动续跑中断批次（§7.1）；按权限逐项拆分负责人能力（原因：两级即可，后续按需收紧）；只读凭证（§6.2）；"取最后一个数"判分（§4.3）；按模型描述校验思考强度（原因：描述只覆盖少数模型，枚举校验足够，上游会拒绝不支持的值）。

## 16. 状态与待办

- 已完成：后端全部（迁移、仓储、服务、执行器、权限、路由、审计、Wire）、前端全部（用户区、配置页、菜单与登录落地、i18n）及其单测/集成测试。
- 登录落地：只持 `admin.gpt_iq_test` 的子管理员在后台模式落地 `/admin/gpt-iq-test`、普通模式落地 `/dashboard`；部门/组织负责人另被授予它时仍落地原部门页（`getAdminLandingPath` 把它视为中性权限，2026-11-23 修复）。
- 待办：浏览器验收（双开关四种组合、仅新权限子管理员登录、负责人范围）；上线前用 1 个隔离账号真实跑 hi 与糖果题各一次，核对 token 量级并据此定默认预算；CI（PG18）再跑集成测试；第二期邮件通知。
