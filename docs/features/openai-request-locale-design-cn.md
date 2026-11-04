# OpenAI 请求时区改写设计

更新：2026-11-04。状态：**未合入。实现只在分支 `feature/hy/10212_sub4api_request_locale`（tip `82dd34428`，本地与 GitHub 远端都在）；main（`26f6363af`）没有这套实现，也没有替代实现。用户 2026-11-03 裁定"先保留"：本地与 GitHub 分支都保留，不合并、不重做。**

本文是 OpenAI 请求时区改写功能的唯一文档，记录需求来源、分支实现的合同、已知问题、合入时的约束和验证结果。审核原文已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/reviews/2026-09-29-sub4api-request-locale-review-cn.md` 查看；2026-11-03 的未合并工作复审见同提交下 `docs/reviews/2026-11-03-unmerged-work-claude-review-cn.md` §4、§8 与 `…-codex-review-for-claude-cn.md` §7）。分支阶段的安全启用说明、两轮 Codex 复核和复现用 overlay 测试只存在于仓库外，未入库。

阅读顺序建议：
- 决定要不要继续：先读 §1、§6；
- 准备合入或重做：读 §2、§3、§5、§7；
- 定位分支代码：读 §4。

## 1. 需求来源与现状

- **是什么**：全局开关与账号开关都开启时，把 Codex 隐藏的环境块 `environment_context` 里的 `timezone`，以及 Web Search 工具的 `user_location.timezone`，改成账号所选时区（默认 `America/Los_Angeles`）后再发上游。不改 `Accept-Language`，不改后台显示时间。
- **需求来源**：仓库内唯一来源是 2026-09-29 迁移审核的一句"对标 MACOS-DO/sub4api 的账号请求时区"。没有需求方、工单或故障记录。
- **动机**：推测是降低共享账号暴露中国时区的风控信号。**这是推断，未验证**；也没有证据表明它能降低封号——相对 IP 与出口代理，时区是弱信号。
- **main 现状**：main 只**读取** `environment_context`（Token 分析的归因、风险、来源判定），从不改写时区。`git grep` 在 main 的 backend、frontend、llm-wiki 中找不到 `openai_request_timezone`、`applyOpenAIRequestLocale`、`enable_openai_request_timezone_rewrite`、`normalizeOpenAIRequestLocale`。
- **范围外**：sub4api 的 Codex 打票、降智检测、BPS 不迁移。

## 2. 分支实现的合同（`82dd34428`）

分支经历两版：2026-09-29 首版是"缺省即改写、默认 `Asia/Singapore`"；2026-09-30 改为下表的"默认全关"版，tip `82dd34428` 是后者。

### 2.1 生效条件

| 条件 | 结果 |
| --- | --- |
| 全局 `enable_openai_request_timezone_rewrite` 为 `true`，且账号 `extra.openai_request_timezone_rewrite_enabled` 为 JSON `true`，且平台是 OpenAI | 改写 |
| 其它任何组合（字段缺失、`null`、字符串 `"true"`、数字、读取失败） | 原样返回 body：不解析、不改写、不碰请求头 |

- 默认关闭与默认时区是两回事：两个开关默认都关；目标时区 `extra.openai_request_timezone` 缺省为 `America/Los_Angeles`，只在开关开启后用到；账号开关关闭时保留已选时区。
- 优先级：全局关 > 账号关 > 账号目标时区 > 默认美西。
- Spark 影子账号：开关与时区取本次 attempt 暂存的母账号配置，影子自身 extra 一律忽略；母账号未暂存或 ID 不匹配时不改写。
- 读取失败：全局设置读取失败 fail-closed（不改写），失败结果缓存 5 秒后重试。账号开关为关时不读 settings。

### 2.2 改写规则（`normalizeOpenAIRequestLocale`）

- 只处理 user 角色的字符串 `content` 或 `input_text`；assistant、developer、tool、`function_call_output` 不改。
- 要求 trim 后的整段文本恰好是一个完整的 `environment_context`。普通句子、带前缀的示例、代码围栏、引用块保持原样；重复 `timezone`、自关闭、嵌套、错误闭合、两个根节点都拒绝改写。
- `current_date` 不改；Web Search 只改 timezone，country/city/region 保留。
- 全有或全无：任一 sjson 写入失败即回退原 body。
- 日志只记账号 ID、transport、enabled、reason 和计数，不记客户端时区值或正文；级别为 Debug。

### 2.3 接入点

- HTTP 普通/透传/compact：`openai_gateway_forward.go` 的 `Forward()`，在 `prepareCodexAccountIdentitySource` 之后、透传分支之前。
- WS ingress（ctx_pool/shared/dedicated/http_bridge）：`openai_ws_forwarder_ingress.go` 的 `parseClientPayload`，每轮执行。
- WS v2 passthrough：`openai_ws_v2_passthrough_adapter.go` 的首帧与后续 `response.create`。
- **不改写**：`/v1/chat/completions`、Anthropic Messages 桥、Alpha Search（自行组装 `user_location`）、input-tokens 端点。第三方兼容账号若由 Responses 转成 Chat Completions，改写发生在转换之前，仍生效。

### 2.4 生效时限

| 变更 | 生效 |
| --- | --- |
| 全局开关（保存所在实例） | 立即 |
| 全局开关（其它实例） | 最迟 60 秒（`gatewayForwardingCacheTTL`） |
| 已建立的 WS 连接 | 每个 `response.create` 重判全局开关，最迟 60 秒停止改写，不断开连接 |
| 账号开关/时区/影子母账号 | HTTP 每次 attempt、WS 建连或 failover 重选账号时确定；已建立的 WS 连接中途不变 |

紧急关闭：系统设置 → 网关 → "OpenAI 请求时区改写（全局）"关闭并保存，等价于 `PUT /api/v1/admin/settings {"enable_openai_request_timezone_rewrite": false}`，无需重启。

### 2.5 管理写入校验（`ValidateOpenAIRequestTimezoneExtra`）

| 情况 | 错误码 |
| --- | --- |
| 开关不是布尔，或非 OpenAI 账号设为 `true` | `INVALID_OPENAI_REQUEST_TIMEZONE_REWRITE` |
| 时区不在白名单（`openai_request_timezones.txt`，463 个，`go:embed`，不含 UTC） | `INVALID_OPENAI_REQUEST_TIMEZONE` |
| 影子账号写入任一非 null 值（null 用于清理残留） | `OPENAI_REQUEST_TIMEZONE_MANAGED_BY_PARENT` |
| 批量更新的目标含非 OpenAI 或影子账号 | 整批 400 `OPENAI_BULK_TARGET_INVALID`，不写入 |

覆盖路径：Create、Duplicate、导入（走 Create）、Update（整份 extra）、`UpdateAccountExtra`（局部合并）、`BulkUpdate`。前端 `OpenAIRequestTimezoneField.vue` 只在管理员动过（touched）时写两个字段，Codex 重导入不覆盖已有账号。

无数据库迁移：全局开关存 settings 表，账号字段在 `extra` JSONB；回退到旧版本时新增 key 被忽略。

## 3. 已知问题与风险

### 3.1 未修复：设置缓存三次 CAS 耗尽后返回被拒绝的旧值

- 位置：分支 `setting_gateway_runtime.go` 中 `getGatewayForwardingSettingsCached` 的重试循环（tip 第 902、908 行 `return loaded.result(), nil`）。
- 现象：三次加载期间快照都被保存或失效替换、最后又没有未过期快照时，直接返回最后一次 CAS **失败**的结果，可能是关闭前读到的 `true`。Codex 用 overlay 测试确定性复现：最后保存 `false`，getter 仍返回 `true`。
- 影响范围窄：需要短时间内连续保存/失效叠加回读失败才会连续三次命中；只影响单实例触发该次加载的一批请求（含 singleflight 等待者），结果不写回缓存，下一次成功读取即恢复。
- 修法（一行）：重试耗尽且没有有效当前快照时返回"不改写"（对新开关返回关闭/不可用），保持有界重试。
- 注意：该改造替换的是**所有网关转发设置共用**的缓存（指纹、cch、dateline 等），不只时区；修 F3 时要明确其它字段的降级策略，避免无关行为变化。

### 3.2 已在分支修复（2026-09-30）

- 首版"缺省即改写全部 OpenAI 流量"→ 改为双开关默认全关。
- 批量更新绕过时区白名单 → `BulkUpdate` 先校验值、再校验目标平台，整批拒绝。
- 在途旧读取覆盖刚保存的关闭值（F1）→ 写回改用 `CompareAndSwap`，有效期从开始读库计时。
- 部分更新写库成功但回读失败时保留旧开启缓存（F2）→ 回读失败调用 `invalidateGatewayForwardingSettings()`。
- 加载失败文案 → 改为"时区列表加载失败，仅显示当前值"。
- 其它进程缓存（`backendMode`、`versionBounds`）仍是"读完无条件写回"，未改。

### 3.3 行为风险（即使实现正确也存在）

1. 模型以为用户本地时间偏差 15—16 小时（中国客户端改成美西）。
2. `current_date` 不改，日期与时区可能自相矛盾；Web Search 时区与 country/city 不一致，搜索"今天"可能错。
3. 覆盖不一致：Chat Completions、Messages、Alpha Search、input-tokens 端点保留客户端时区。
4. 会话中途切换开关或时区会改变请求前缀，打断 prompt cache。
5. 请求归档与用量 hash 记录的是进入 `Forward()` 前的客户端 body，上游看到的是改写后的；排障时两边时区不同。
6. `environment_context` 若与用户问题写在同一段或格式不完整，改写静默不生效。
7. 未证明能降低封号。

## 4. 分支代码地图（`82dd34428`，相对 main 42 个文件，+2668/−109）

| 位置 | 内容 |
| --- | --- |
| `backend/internal/service/openai_request_timezone.go` | `DefaultOpenAIRequestTimezone`、`ValidateOpenAIRequestTimezoneExtra`、`openAIRequestTimezoneSource`、账号开关 getter |
| `backend/internal/service/openai_request_timezones.txt` | 时区白名单（`go:embed`，漏提交会编译失败） |
| `backend/internal/service/openai_request_locale.go` | 唯一 gate `applyOpenAIRequestLocale`、改写 `normalizeOpenAIRequestLocale`、`rewriteOpenAIRequestEnvironment` |
| `backend/internal/service/setting_gateway_runtime.go` | 全局开关快照、`OpenAIRequestTimezoneRewriteState`、CAS 重试 |
| `backend/internal/service/openai_bulk_account_settings.go`、`admin_account.go` | 写入校验 |
| `backend/internal/service/setting_gateway_forwarding_cache_test.go` | F1/F2 回归测试 |
| `frontend/src/components/account/OpenAIRequestTimezoneField.vue` | 账号开关与时区选择器 |
| `frontend/src/views/admin/SettingsView.vue` | 网关标签页全局开关 |

## 5. 合入或重做时的约束

1. **先回答"现在还要这个行为吗"。** 需要的是用途确认，不是 CAS 实现是否正确。没有明确需求就维持保留状态。
2. **修 §3.1 再灰度。** 重试耗尽返回"不改写"。
3. **基于当前 main 接入。** 2026-11-03 `merge-tree` 显示 6 处小冲突：`routes/admin.go` 相邻路由、WS v2 adapter 改名、两个账号弹窗测试、两页 wiki。挂点 `openai_gateway_forward.go` 仍在；账号弹窗约 1500 行变动可自动合并但需 UI 复测。估计半天到一天，不含真实上游验证。
4. **保持默认全关。** 两个开关默认 false；不要恢复"缺省即改写"。
5. **隔离灰度。** 开启账号开关的专用账号放在只有验收 API Key 使用的独立分组；有 Spark 影子时确认影子也不在共享分组。
6. **验收不以模型回答为唯一标准。** 关联 `request_id`、实际路由账号、`reason=replaced/global_disabled` 和正常完成结果；关闭全局后检查已有 WS 的下一轮。字节一致性证据要记录原始 body 字节或哈希，而不是解析后的 JSON。
7. **CI 门禁是 `-tags=unit`。** 分支阶段把 service 测试放在无 tag 文件里绕过了当时的 unit 编译错误；合入时测试要能在 `make test-unit` 下运行。
8. 合入后需在 `llm-wiki/wiki/backend.md`、`ops.md` 补合同与紧急关闭说明，并在 `docs/README.md` 登记本文。

## 6. 决策记录

| 日期 | 决策 |
| --- | --- |
| 2026-09-29 | 迁移审核：首版无 P0；P1 为缺省改写全部流量，P2 为批量更新绕过白名单、Chat/Messages/Alpha Search 未覆盖 |
| 2026-09-30 | 改为双开关默认全关、默认目标美西；修 F1/F2；第二轮复核判"默认全关版本功能层面条件 GO，开启灰度前需修 F3" |
| 2026-11-03 | 未合并工作复审：需求只有"对标 sub4api"，main 无替代；用户裁定**先保留**，本地与 GitHub 分支都保留，不合并、不重做 |

已否决：把 `Accept-Language` 一并改写。原因：HTTP 与透传已走既有白名单，WS 握手复制非空值，API Key 的 `header_overrides` 可覆盖；本功能只改请求体时区。

## 7. 验证（分支阶段，2026-09-30）

- 专项单测：请求时区/HTTP/WS/BulkUpdate/局部更新 18 个顶层测试通过，`-race -count=5` 无竞争；10 项变异测试全部被抓到。
- 快照测试 `TestGatewayForwardingSnapshot*` 通过；F3 overlay 测试失败（证明 §3.1 存在）。
- 前端：`check:i18n`、`typecheck`、`lint:check` 通过；Create/Edit/OpenAIRequestTimezoneField/localeKeyCompleteness/SettingsView 5 个文件 152 例通过。
- 本地整栈演练（嵌入式 PG 18 + Redis + 假上游，未调真实模型）：35/35 通过，覆盖默认状态、开关组合、流式、compact、透传、兼容账号、紧急关闭、管理校验与日志无泄漏。基准：关闭约 55ns/次，开启约 0.72ms/次、0.6MB 分配（约 200KB、400 条历史的请求）。整栈数字是 Claude 的报告，Codex 未独立重跑。
- 未验收：真实 Codex CLI 的环境上下文格式、真实 WS 全链路、OAuth 真实转发、Spark 影子实际调度、真实 compact、多轮 prompt cache 命中率。
- 2026-11-04：本文未重跑分支测试（分支未检出，只读核对 tip 源码）。
