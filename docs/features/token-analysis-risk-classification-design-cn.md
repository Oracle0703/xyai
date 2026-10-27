# Token Analysis 风险分类设计(成本观察与缓存复用)

状态: 数据基础与分类计算(冷暖、原因、资格、金额、版本传播、分类作业)已实现(第 16 节); 读接口与页面、连续活跃、受控重索引尚未实施。分支 `feature/hy/10221_token_risk_classification`。

用"成本观察 + 有证据的缓存复用机会"取代 `/admin/token-analysis` 现有的 `risk_score`。所有结果只用于展示和排序, 不作为惩罚依据。

本文是该功能唯一的设计文档。2026-10-26 设计过程中各轮审核的结论已并入正文; 第 13 节列出其中不明显、容易被改回去的约束。源码引用用函数名, 不用行号。

## 1. 背景与样本证据

### 1.1 旧规则的问题

`backend/internal/service/token_analysis_risk.go` 用 8 条规则按固定分值相加, 阈值和分值没有数据依据:

| 规则 | 14 天命中 | 问题 |
| --- | ---: | --- |
| `oversized_system_prompt` | 95.5% | agent 客户端的 instructions 天然很大, 且会走缓存; 93.5% 的请求分数落在 20-29, 几乎只命中这一条 |
| `low_cache_hit_large_input` | 1.22% | 信号有效, 但没有区分原因 |
| `huge_input_tiny_output`、`tool_heavy_short_output` | 0.28%、0.38% | agent 调工具那一轮输出本来就短; 与上一条同源, 叠加计分 |
| `large_tool_history`、`giant_tool_output` | 0.59%、少量 | 摘要器对 Responses/Messages 的工具输出有盲区(4.1), 对 Codex 基本无效 |
| `repeat_uncached_body` | 2 次 | `same_body_recent_count` 97% 的行缺失 |
| `rapid_similar_requests` | 0 | `SimilarRecentCount` 全仓库没有计算 |

### 1.2 样本证据(生产只读样本, 2026-10-12 ~ 10-26, 694,658 条摘要)

- **token 口径**: 各平台 93.6%-97.9% 的行 `cache_read_tokens > input_tokens`, 说明入库的 input 是未命中缓存部分。OpenAI 写账单时把总输入拆成 input、cache_read、cache_creation 三个互斥桶(`openai_gateway_usage.go`, `actualInputTokens`)。
- **钱在哪**: 命中 `low_cache_hit_large_input` 的请求费用合计 8,564, 不超过窗口总费用的 17%。费用最高的 50 条请求中, 未缓存输入占 99.1%, 单条 55-77 万 token 只命中约 4k-22k 的固定前缀。未缓存输入单价是缓存读的 10-20 倍。
- **缓存断裂与闲置**: 同会话相邻请求的断裂率: 间隔 ≤5 分钟 1.3%, 5-30 分钟约 10%, >30 分钟 64%, >1 小时 74%。缓存热时换账号, 27% 会冷掉。大上下文冷请求率按上游账号分布在 0%-12%。
- **重复请求**: 同 body 重复组的总费用 156.43, 不足窗口费用的 0.31%; 多为没有计费的失败重试。
- **流量构成**: Codex 约 84%, 其次是 Copilot、Claude Code、hertz、OpenCode、SDK 脚本; 9.7% 的请求未关联计费。
- **使用形态**: 多数 Key 夜间请求占比 <10%; 抽样 60 个 Key 中 4 个连续活跃 ≥24 小时, 最长 233 小时。

样本含生产数据, 不入库。会话类比例来自 57 个偏向高成本的抽样会话, 只看方向, 不外推全站。

## 2. 已定政策(2026-10-26)

| 形态 | 政策 |
| --- | --- |
| SDK / 脚本调用(含非编程批处理) | 允许, 不标记 |
| 第三方云端代调 | 允许, 不标记 |
| 公司自有客户端 hertz | 单列为自有客户端 |
| 无人值守长时间 agent | 允许, 页面标出 |

据此不做 IP 多样性、夜间活跃、SDK 类标记。

## 3. 范围与分期

首版内容:
1. **冷暖观察**: 按计费口径统计大上下文请求的缓存比例, 按账号/模型/客户端分层。
2. **冷请求原因**: 只描述观察到的差异, 不判定平台或用户责任。
3. **条件性复用机会金额**: 只覆盖 OpenAI 原生 Responses, 且三类证据同时成立(第 7、8 节)。
4. **长上下文**: 只展示 `long_context_billing_applied` 的计数和占比。该标记只覆盖模型阶梯, 不含渠道/分组区间价。
5. **观察到的连续活跃**: 疑似无人值守的 agent。
6. **工具输出诊断**: 条数和字节, 不出金额。

不做(推迟或移出): Anthropic 及其他平台的金额、平台/用户归责汇总、长上下文附加费金额、工具携带金额、重试/重复调用视图(交给运维监控)。

| 阶段 | 内容 | 对外可见 |
| --- | --- | --- |
| 0 前置调查 | 第 11.1 节; 产出"支持路径清单" | 否 |
| 1 数据层 | 摘要器修复、客户端识别、前缀指纹与候选边、转发侧记录、冷暖状态; 并入受控重索引 | 否 |
| 2 观察页面 | 真实用量、冷暖与原因、长上下文计数、连续活跃、工具诊断 | 是(`observation_enabled`) |
| 3 金额 | 第 7、8、10 节验收全部通过, 且转发侧记录已积累足够新行 | 是(`amount_enabled`) |

## 4. 摘要与客户端识别

### 4.1 摘要器修复(`token_analysis_summary.go`)

| 协议 | 现状 | 修复 |
| --- | --- | --- |
| Responses | `function_call_output` / `custom_tool_call_output` 等条目的正文在 `output` 键(`tool_search_output` 的结果在 `tools`), `tokenAnalysisExtractText` 不读这些键, 工具输出哪里都没计; assistant 消息落入 default 分支, 计进了 `user_chars` | 按条目类型取结果字段计入工具字段; `user_chars` 不含 assistant 文本 |
| Messages | user 消息中的 `tool_result` 计进了 `user_chars` | 计入工具字段, 从 `user_chars` 剔除 |
| Gemini | `functionResponse` 没计 | 计入工具字段 |
| 图片数 | 内容中的任意非空字符串都被记为一张图片; Responses/Chat/Messages 都允许 `content` 为纯文本字符串, 纯文本请求因此 `image_count > 0`, 被金额门槛误判为图片请求(2026-10-27 审核 R7) | 只按图片字段(`image_url`、`input_image`、`inline_data` 等)与图片块类型(`image`、`input_image`、`image_url`)计数; 字符串只在图片字段下才算一张图; Responses 工具输出(如 `function_call_output` 的 `input_image` 块)同样计入(复核 N1) |
| 工具定义数 | 只数顶层 `tools` 的条目; Codex 把工具定义放在 `input` 的 `additional_tools` 条目中(namespace 内嵌叶子工具), 顶层没有 `tools`, 因此 `tools_count` 几乎恒为 0(2026-10-27 样本: Codex Desktop 各版本 99%-100%) | `tools_count` 统计顶层 `tools` 与 `additional_tools` 中的叶子工具定义, namespace 递归计其叶子; namespace 子项与 apicompat 适配器口径一致: 非空 `tools` 优先, 否则回退 `children`(只对 namespace 认 `children`, 第四轮复核 F2) |

工具字段指 `tool_message_count` / `tool_output_bytes` / `max_tool_output_bytes`。旧风险分以这些字段为输入, 修复后会变化, 这是已接受的结果; 旧规则在 12.4 的 C 阶段下线。

现有请求明细的 `duplicate_count` 按净输入 `content_sha256` 分组, `same_body_recent_count` 是整 body 的信号。两者口径不同, 均保持不变。

### 4.2 客户端识别

- UA 取归档事件的 `user_agent`, 写入摘要行; 未关联计费的行也能识别。
- 映射为可配置的有序规则: 正则、不区分大小写、首个命中生效, 未命中归为 `other`。
  - 内置族: codex、claude_code、copilot、cursor、cline_like、gemini_cli、opencode、sdk_script、hertz(自有客户端)。
  - "agent 族"指除 sdk_script、other 以外的族。
- UA 截断到 512 字符, 经 `sanitizeTokenAnalysisText` 清洗, 每行记录 `client_family_version`(规则内容的哈希, 0 表示列上线前的历史行)。
- 规则变化后, 自动索引每轮按 id 游标补映射一批旧版本行, 扫完一遍后隔 1 小时再复查, 使滚动发布期间旧实例晚写入的行最终也被补上。写回按读出时的 UA 与版本快照条件提交, 期间被改过的行跳过, 由下一遍重新读取。版本是哈希, 不能按大小比较先后。
- 运行约束: 所有实例必须使用相同的 `client_family_rules`; 规则不同的实例长期并存会互相改写对方的结果, 只允许在滚动发布期间短暂出现。
- UA 是客户端自报的, 不作可信度依据。

## 5. 前缀指纹与候选

### 5.1 解析与规范化

- 请求体只解码一次: 用 `json.Decoder` 配合 `UseNumber`(保留数字原始字面量, 大整数不转成 float64), 摘要器与指纹共用解码结果(`parseTokenAnalysisRequestBody`)。顶层值之后的多余内容按解析失败处理, 与 `json.Unmarshal` 一致。
- 重复键按解码"后者生效"处理, 不单独检测(2026-10-27 决定): 网关与多数上游解析器同样是后者生效; 单独检测需要再做一遍逐 token 解析, 而生产归档一天约 130 GB。
- 每个条目先做一次 SHA-256, 前缀链与关联链在这个摘要上各做一次 HMAC, 条目内容只哈希一遍。实测约 5 MB、700 条的请求体, 解析加摘要 14.3 ms, 再加指纹 17.2 ms。
- 规范化编码: 每个节点都带类型标签和长度前缀。
  - 对象: `O` + 成员数 + 按键字节序排列的(键长度 + 键 + 值)
  - 数组: `A` + 元素数 + 元素
  - 字符串: `S` + 字节长度 + 原始字节
  - 数字: `N` + 原始字面量
  - 布尔/空值: 各自的标签
- 不做脱敏、NUL 清洗或数值归一; 这只会导致"前缀变化"误报, 不会产生错误的"已验证"。
- body 截断、降级、或 multipart 时 `fp_state = unsupported:<原因>`。

### 5.2 链与字段划分

按协议族定义"条目字段"和"非前缀字段白名单", 两者都随 `fp_version` 版本化:
- 条目字段: Responses 为 `input`, Messages/Chat 为 `messages`。Gemini 首版不支持。
- Responses 的 `input` 为字符串时, 视为单个带专用标签的条目, 永远不会与数组形式的条目相等。
- 非前缀字段白名单: `stream`、`stream_options`、`max_tokens`、`max_output_tokens`、`temperature`、`top_p`、`metadata`、`user`、`store`、`include`、`service_tier`、`client_metadata`。
  - `client_metadata` 是 Codex 的请求元数据(turn/thread/window id 等), 不进入提示词。2026-10-27 样本中它在同一会话里频繁变化而缓存照常命中; 若计入 `h0`, Codex 请求几乎都对不上。
  - 顶层 `tools`、`parallel_tool_calls` 保留在 `h0` 中: 样本中它们变化的那次请求确实冷了。

计算方式:
- `h0 = HMAC(k, 协议族 ‖ canon(顶层对象 − 条目字段 − 白名单 − prompt_cache_key))`。其余顶层字段(instructions/system、tools、tool_choice、reasoning、`prompt_cache_retention` 等)变化都视为前缀变化。
- `h_i = HMAC(k, h_{i-1} ‖ canon(item_i))`, 持久化 `prefix_end_hash = h_n` 和 `prefix_tail1_hash = h_{n-1}`(n ≥ 2 时)。
  - `prefix_tail1_hash` 用于"父请求去掉末尾 1 条后是子请求的前缀": Claude Code 每轮都会替换末尾一条临时 system 消息, 样本中 29 对相邻请求全部只在这一条上不同, 去掉后 29/29 成立; 再多去一条没有额外收益。
- 关联链 `g_0 = HMAC(k, 协议族)`、`g_i = HMAC(k, g_{i-1} ‖ canon(item_i))`, 持久化 `items_end_hash = g_n`。它不含 system/tools, 只作对话关联线索。
- 其他持久化字段:
  - `prefix_items`: int4。超过 1,000,000 条时 `unsupported:too_many_items`。
  - `client_cache_key_hash`: 入口 `prompt_cache_key` 的 HMAC。
  - `has_continuation_ref`: 存在 `previous_response_id`、`conversation` 等外部续接引用时为 true。

### 5.3 密钥与版本

- HMAC 密钥存在 `security_secrets` 表(键 `token_analysis_fingerprint_secret`), 启动时与 JWT 密钥同一机制引导: 不存在就生成 32 字节随机值并持久化, 所有实例共享。哈希截取 128 位。
- `fp_version` 包含规范化规则版本、字段清单版本和密钥 ID; 不同版本的行之间不匹配(`unknown:fp_version_mismatch`)。
- 指纹不经接口返回, 保留期与摘要行一致。

### 5.4 候选边

索引子请求 C 时, 原文就在内存中, 可以算出完整的 `h_1..h_n` 和 `g_1..g_n`:

- 前缀边: 同 `api_key_id`、同 `fp_version`、同协议族, `prefix_end_hash = ANY(h_1..h_n)`, `matched_items ≥ 1`, `event_time ∈ [C − L, C)`。只保留 `matched_items` 最大的那一组并列者, 最多 K 个。超过 K 时记 `cand_overflow = true`, 前缀证据为 `unknown:tie_overflow`。
  - 命中意味着父请求的全部条目都是 C 的前缀, 是完整前缀证据, 不依赖检查点。
  - 另用 `prefix_tail1_hash = ANY(h_1..h_n)` 查"去掉末尾 1 条"的候选, 边的 `kind` 记为 `prefix_minus_tail`, `matched_items = n - 1`。完整前缀与去尾前缀同时存在时优先完整前缀。
  - 必须在同 Key 的所有更早请求中查找, 不能只看上一条: Codex 同一 `session_id` 下会交替运行主 agent 与子 agent 的多个线程(`client_metadata.thread_id` 不同), 相邻请求常属于不同线程。样本中只看相邻时 A3 组仅 9/29 成立, 全局查找后 27/29。
- 关联边: 用 `items_end_hash` 按同样规则查找, 只保留最长一组(≤K)。
- `session_link` 来自双方都已关联计费的 `usage_logs.session_id`, 只作观察标签, 在批任务中补充。
- 边写入 `token_analysis_prefix_edges(child_archive_id, parent_archive_id, kind, matched_items)`, 在 `parent_archive_id` 上建索引。
- 父请求的选择在批任务中进行:
  1. 从子请求的前缀边出发, 在当前层里找有资格的父请求(第 7 节), 按(`matched_items` desc、`completed_at` desc、`archive_id` asc)确定。
  2. 当前层都没有资格时, 沿这些父请求自己的前缀边逐层回溯, 每层保留与 C 共享条目数最多的一组。回溯不超过 D 层, 不超出 C 的 L 窗口; 对访问过的节点去重; 每个子请求的访问预算默认 256 个节点, 超出记为 `unknown:walk_budget`。
     - 祖先 A 经后代 X 的边 X→A 推出与 C 的关系: 共享条目数取 `min(边的 matched_items, X 与 C 的共享数)`; 只有边为完整前缀且 A 的条目数不超过 X 与 C 的共享数时, A 才是 C 的完整前缀。
     - 剪枝(不改变结果): 祖先比后代更早, 超出 W 的节点及其祖先都不可能有资格, 不再展开; 只依赖 C 自身的门槛(续接引用、转发路径、最终缓存键、usage 来源)不满足时, 只看第一层给原因。
  3. 遇到候选快照未完成的祖先时停止回溯; 祖先的快照提交后, 经 10.2 的传播触发 C 重算。当前层有候选账单待复核(含缺完成事实)且没有有资格者时也停止, 资格记为 `pending`。
- 前缀关系有传递性, 链上的祖先也是 C 的前缀。回溯可能漏掉"不在链上、但长度介于两层之间"的合格候选, 结果只会选到更短的父请求或选不到, **只会少算, 不会多算**。页面要说明这一点。
- `api_key_id` 为 NULL 的行不查找。

### 5.5 候选快照的版本

候选边是独立的派生结果, 用自己的版本对 `cand_input_version` / `cand_checked_version`:
- `cand_input_version` 在以下情况加一: 本行指纹新增或变化、窗口内出现新的可能父请求、以本行为父的边所指的父请求指纹变化或失效。
- 候选计算的步骤:
  1. 读取 `cand_input_version = v`, 计算候选边。
  2. 在同一事务内删除旧边、写入新边, 并设 `cand_checked_version = v`, 条件为 `WHERE cand_input_version = v`。不满足条件就回滚。
- 读者只会看到完整的旧集合或完整的新集合。分类只在两个版本相等时使用候选边, 否则该行为 `pending:candidates`。
- **晚到的父请求**: P 的指纹新增或变化时, 在同一事务内把同 Key、`(P.event_time, P.event_time + L]` 内**相同指纹版本且已有指纹**的行的 cand 版本加一(cls 版本随分类阶段一并加)。
  - 正常在线索引时 P 是最新的行, 窗口内没有行, 这一步没有开销。
  - 按文件顺序首次补算历史指纹时, 之后的行还没有指纹, 不会被标脏, 它们处理到时会自行找到 P, 避免成批标脏。指纹版本变化后的重算同理。
  - 摘要写入先在事务内锁定读取旧指纹(`SELECT ... FOR UPDATE`), 指纹未变的重写不加版本、不传播。
- **候选锁(同 Key 串行)**: 有 API Key 的摘要写入, 以及候选查找+写边, 都在事务开头取同一把 `pg_advisory_xact_lock('token_analysis_candidates:' || api_key_id)`, 之后才锁摘要行。候选查找在持锁的写边事务内进行(`TokenAnalysisPrefixEdgeWrite.Query/Select`)。
  - 否则会漏: P 的事务已标记过"之后的行"(那时 C 尚未提交)但还没提交, C 提交后查候选看不到 P、确认空集合; P 提交后没有任何版本变化通知 C(2026-10-27 审核 R2, 已复现)。
  - 持锁后两种顺序都安全: P 先持锁, C 的查找等 P 提交后才查, 能看到 P; C 先持锁, P 之后才插入并标记已提交的 C。
  - 每个子请求的查找按完整前缀、去尾前缀、关联三类各取条目数最多的 K+1 行(多取 1 行判断溢出), 每行只归入优先级最高的一类, 结果行数有界。
- **父请求指纹变化或失效**: 同一事务内, 经边表给以它为父的子请求加 cand 版本。本行变得不可比较时, 删除它作为子请求的边, 版本对齐, `cand_state` 记为指纹状态。
- **索引时立即计算**: 写入后若候选待计算, 用内存中的链查找候选并按写入返回的版本条件提交; 失败只记日志, 行保持待重新匹配, 由自动索引每轮的重新匹配任务(每轮最多 500 行)补算。
- **重新匹配**: 按 5.6 重读原文, 重算链和边; 原文不可用时写入 `cand_state = unknown:body_unavailable`, 同样走条件提交。

### 5.6 原文定位

- 新行持久化 `source_line_start` 和 `source_line_length`。
- 旧行的 `source_offset` 是**行尾之后**的位置(索引循环中的 `lineOffset`)。重读时:
  - 先跳过末尾的换行符(末行无换行时不跳过)。
  - 再向前有界查找上一个换行符, 上限为归档单行上限。
- 归档单行上限按可配置的最大正文上限(512 MiB)乘以 JSON 字符串转义的最坏膨胀 6 倍(如 `<` 变 `\u003c`), 另加 8 MiB 元数据余量; 只防读到损坏的位置, 不限制合法的未截断请求(审核 R8)。
- 读出后校验 `event = request`、`archive_id`, 正文未截断, 并对读到的正文**重算** SHA-256 与摘要存下的 `body_sha256` 比较; 不信任行内的 `body_sha256` 字段(审核 R6)。
- 重读正文按当前规则与密钥算出的指纹必须与摘要存下的指纹(`fp_version` 与三个哈希)相同, 才能为这份快照确认候选; 不同(规则或密钥已变、原文漂移)时同样记为不可用。
- 文件不存在、已压缩、已替换或校验失败时, 记为 `unknown:body_unavailable`。

## 6. 冷暖观察与原因

### 6.1 冷暖状态

- `ctx = input + cache_read + cache_creation`。
- `cache_obs_state` 的取值:
  - `small`: ctx < 50,000
  - `hit`: `cache_read / ctx ≥ 0.2`
  - `cold`: `cache_read / ctx < 0.2`
  - `no_usage`: 未关联、失败或 not_billable
  - `unsupported`: 非 token 计费
- 冷暖统一按**计费口径**判定, 页面命名为"计费缓存比例"。`usage_cache_source` 标注这些桶能否代表上游原值:
  - `upstream_parsed`: 转发侧记录确认 usage 来自上游解析, 未改写。
  - `rewritten`: 发生了改写。
  - `unknown`: 历史行或没有记录。
- Anthropic 路径的 usage 可能被本地改写: `recordUsageCore` 中的 `ForceCacheBilling` 会把普通输入改记成缓存读, TTL 覆盖(`applyCacheTTLOverride`)会重新划分 5m/1h。这些行只按计费口径统计。
- 冷率 = cold / (hit + cold), 分母包含热请求; 覆盖率单独展示。可按 `usage_cache_source` 筛选。
- 当前请求的账单关联待复核时, 冷暖照常计算, 但标记 `usage_link_pending`, 页面显示"账单待复核", 可筛选排除。
- 分层维度: 平台 × 实际发送模型 × 客户端族 × 上下文区间 × 首条/续接。单元格样本少于 200 时只显示计数。

### 6.2 维度口径

- **账号**: `usage_logs.account_id`, 即最终服务账号; 不覆盖请求内部的 failover 链。
- **实际发送模型**: `upstream_model` 非空就用它, 否则用 `usage_logs.model`, 即 `COALESCE(NULLIF(TRIM(upstream_model), ''), model)`, 与现有用量报表的读取口径一致。
  - 依据: 写账单时调用 `upstreamSentModel(result.Model, result.UpstreamModel)`(Anthropic 与 OpenAI 两处), `result.Model` 存入 `usage_logs.model`。
  - 各平台路径已核对(11.1): 多数路径 `upstream_model` 总是写入映射后的模型; 只有 OpenAI 透传与 WS v2 透传在"未映射"时为空, 此时发送模型就是 `model`。
- `requested_model` 是渠道映射前的客户端模型, 只用于展示。`upstream_response_model` 只作观测维度。
- NULL 与 NULL 不相等。

### 6.3 原因(观察, 不归责)

对 `cold` 行:
1. 选出有资格的父请求, 且本条仍冷 → `missed_with_eligible_parent`。
2. 有前缀边但都没有资格 → 取第一层(C 自己的前缀边), 报告其中按第 7 节门槛顺序走得最远的候选遇到的不合格项: `parent_unusable`、`parent_pending`、`parent_overlapping`(父请求完成不早于子请求开始)、`model_changed`、`account_changed`、`continuation_ref`、`unsupported_path`、`prompt_rewritten`(网关改写了最终条目或顶层字段)、`cache_key_changed:<client|gateway>`(入口 `client_cache_key_hash` 不同记 client, 相同记 gateway)、`usage_not_upstream`、`beyond_reuse_window`。
3. 只有关联边 → `prefix_changed`。
4. 没有任何边 → `no_candidate`, 子原因为 `none_in_window`(含无 API Key)、`fingerprint_unavailable`(无密钥、正文截断/未解析、协议不支持)、`body_unavailable` 或 `tie_overflow`。`fp_version_mismatch` 需要按其他指纹版本再查一次, 首版不产生。

只有 hit 与 cold 行查找父请求(hit 行用于金额, 不给原因); `small`、`no_usage`、`unsupported` 是无父终态。

页面措辞只描述观察: 例如"超过配置的复用窗口", 不说"缓存已过期"; 例如"账号不同", 不说"平台导致"。

## 7. 证据分级与资格

| 字段 | 取值 |
| --- | --- |
| `prefix_evidence` | `full_prefix` / `prefix_minus_tail` / `items_link` / `session_link` / `none` / `unknown:<原因>` |
| `cache_eligibility` | `eligible_assumed` / `ineligible:<原因>` / `unknown:<原因>` / `pending` |
| `price_evidence` | `row_exact` / `estimated`(首版不实现) / `unavailable:<原因>`(`prefix_minus_tail`、`openai_cache_write`、`image_input`、`no_cache_read`、`no_input_price`) |

账单关联状态是父请求和当前请求**共同的**门槛, 双方各自按下表判定。`RecordResponse` 登记复核时只增加 `usage_link_version`, 不清空已有的 `usage_log_id`, 所以"有 usage 行"不等于"账单已确认"。`usage_link_retry_until` 只是等待晚到账单的截止时间。

| 关联状态 | 处理 |
| --- | --- |
| 有 `usage_log_id`, 且 `usage_link_version = usage_link_checked_version` | 继续判定其余门槛 |
| 完成事实缺失, 或两个版本不等(含复核期间仍挂着旧账单) | `pending`, 金额为 NULL。当前请求为 `cls_state = pending:usage_link`, 冷暖照常给出; 父请求为 `parent_pending` |
| 已确认无法关联、失败或 not_billable | 父请求为 `ineligible:parent_unusable`, 当前请求为 `no_usage` |
| 任一状态变化(含 `SetUsageLink`、`ClearUsageLinkPending`) | 自身及依赖重新判定 |

`cache_eligibility = eligible_assumed` 要求父子成对满足以下全部条件, 按顺序检查:
1. **账单已确认**: 双方都处于上表第一行; 父请求为 token 计费, 不是 count_tokens。已计费输入说明上游处理过该提示词; "缓存因此建立并保持可用"是平台假设, 因此叫 `assumed`。
2. **时序**: `父.completed_at < 子.event_time`, 严格小于, 两者都必须存在。
3. **成对一致**: 实际发送模型、账号、协议族、`fp_version` 都相同; 双方都没有续接引用。入口的 `client_cache_key_hash` 只用于原因标注(区分客户端改了键还是网关改了键), 不作门槛。
4. **转发路径**: 双方都有转发侧记录(11.2), 都走 OpenAI 原生 Responses(未做协议转换), 且双方的 `upstream_cache_key_hash` 相同。
   - 比较"最终发往上游的缓存键", 而不是"缓存键是否被改写": OAuth/SetupToken 账号的 `prompt_cache_key` 总会被按账号作用域确定性改写(`scopeCodexAccountIdentityValue`), 同账号、同 API Key、同原始键的结果相同; 指纹 session/full 模式会换成账号级常量。比较最终值同时覆盖这两种情况。
   - `cache_key_handling`(passthrough/absent/scoped/injected/deleted)只作观测。
   - 经过协议转换(转 Chat Completions/Anthropic)、或没有记录 → `ineligible:unsupported_path` 或 `unknown:transform_policy`。
   - **最终提示词仍保留入口前缀**: 指纹来自入口(归档)正文, 原生路径也会改写条目——例如 `NormalizeCompactionTriggerInputOrder` 把 compaction 触发器移到 input 末尾、`invalid_encrypted_content` 重试与会话失效记录剥离加密 reasoning(审核 R1, 已复现)。因此要求双方 `upstream_items_hash` 都等于各自的 `items_end_hash`(最终条目与入口条目逐条相同), 且父子 `upstream_root_hash` 相同(最终顶层字段相同); 不满足为 `ineligible:prompt_rewritten`, 缺记录为 `unknown:transform_policy`。不以路径名或"缓存键未改"作为前缀保持的证明。
5. **usage 来源**: 双方 `usage_cache_source = upstream_parsed`, 否则为 `ineligible:usage_not_upstream`。
6. **复用窗口**: `子.event_time − 父.event_time ≤ W`, 从父请求**开始**时刻起算。缓存在父请求开始后才写入, 所以开始时刻是写入时间的下界, 判定是保守的; 也不假设中途有读取刷新 TTL。W 取平台文档明确的最短保留期, OpenAI 默认 300 秒; 没有文档依据的组合记为 `unknown:no_window`。

## 8. 条件性复用机会金额(仅 OpenAI 原生 Responses)

只有 `prefix_evidence = full_prefix`、`cache_eligibility = eligible_assumed`、`price_evidence = row_exact` 三者同时成立的行才进入汇总。`prefix_minus_tail` 不进入金额: 被去掉的末尾条目不属于子请求的前缀, 按父请求总提示词计算可复用量会多算; 它只用于观察, 以及将来按 Anthropic 断点口径计算。页面写明前提: "在平台保留缓存至少 W 的前提下"。

- **可复用量**: `父总提示词 = 父.input + 父.cache_read + 父.cache_creation`, 可复用量 = `floor(父总提示词 / 128) × 128`, 且须 ≥1024。
  - 父或子 `cache_creation > 0` 时为 `price_evidence = unavailable:openai_cache_write`(资格仍按门槛判定), 待 11.1 确认缓存写入语义后再纳入。
  - 可复用量不足 1024 时记 0, 金额为 0(不是 NULL)。
- **可挽回量**: `R = clamp(可复用量 − 子.cache_read, 0, 子.input)`。前缀缓存的读取总是从头开始。
- **单价**(`row_exact`): 只用子请求本行的历史有效单价, 不读价卡。
  - 要求纯文本: `image_count = 0`、`image_input_tokens = 0`(`computeTokenBreakdown` 会混入图片缓存读价)。
  - 要求 `子.cache_read_tokens > 0`; 读价 = `cache_read_cost / cache_read_tokens`, 输入价 = `input_cost / input_tokens`。
- **金额**: `amount = max(0, R × (输入价 − 读价))`。
  - 首版只有一个未缓存桶, 上下界相等。将来支持 Anthropic 多桶时, 按单价从低到高/从高到低分配得到上下界。
  - 口径为 `total_cost`(不含用户倍率); `actual_cost` 口径为整笔乘以本行 `rate_multiplier`。两者都不代表平台采购损失。
- **估算**(首版不实现, 记为 `unavailable:no_cache_read`): 子请求没有缓存读时, 可用"本行输入价 × 价比快照"得到 `amount_estimated`, 标记 `price_evidence = estimated`。
  - 快照持久化来源、适用模型与档位、采样时间和规则版本。
  - 估算只保证重算稳定, 不是历史值的上下界, 不进入任何汇总。
- **冷输入费用**: `cold_input_cost` = cold 行的 `input_cost + image_input_cost + cache_creation_cost`, 适用所有平台, 按计费口径。图片输入费用单独记账, 必须计入。
- **输入绑定**: `amount_inputs_hash` = 子与父的 `usage_log_id`、用量与费用字段、规则版本的 HMAC(指纹密钥, 128 位)。
- 金额对 hit 与 cold 行都计算(条件同上); 费用按 `NUMERIC` 精确运算, `R × (input_cost × cache_read − cache_read_cost × input) / (input × cache_read)` 合成一个有理式, 只在最后做一次 10 位舍入; 分别求单价会先各自舍入, 末位可能偏差(复核 N4)。
- 复用机会金额、冷输入费用、长上下文、工具诊断之间**不相加**。

## 9. 观察到的连续活跃(疑似无人值守)

- 小时活跃表 `token_analysis_key_hour_activity(api_key_id, hour_start, agent_requests, cost, version)`: 小时按 Asia/Shanghai 划分; 只统计 agent 族; `request_origin` 为 unknown 时不当作 AI。
- 摘要行的增、改、删把对应的(Key, 小时)标脏。任务重算这些小时, 从变化的小时向两侧扩展到无请求的小时为止, 重写 `token_analysis_key_activity_runs`。历史回填同样走这条路径, 不受 48 小时限制。
- 连续 ≥24 个"观察到有请求"的小时, 标为"疑似无人值守", 区间左闭右开。
- 没有记录的小时只说明"未观察到请求": 归档队列满会丢行(`request_archive.queue_full_dropped`), 不当作已证明停机。
- 受控重索引时整段重建, 期间显示"重建中"。

## 10. 计算、失效与发布

### 10.1 版本对与触发点

分类用两对版本(migration 259), 任一对不等即待分类:
- `cls_input_version` / `cls_checked_version`: 本行**自身**的分类输入版本。只在下列源事件中加一, 且总在同一事务内登记失效(10.2); 子请求记下的 `parent_cls_version_used` 与它比较。
- `cls_dep_version` / `cls_dep_checked_version`: 依赖版本。由依赖传播(10.2)、规则版本扫描(10.5)、候选变化标记(5.5 的晚到父请求与父指纹变化)加一, 只表示"需要重算", 不改变本行对子请求的版本。
- 拆开的原因(第四轮复核 F1, 已复现): 若传播也改输入版本, 一个消费者(过期的或正常的)在查完子孙之后才给某行加版本时, 期间新挂上并已按旧版本写回的子请求会与它永久失配, 且没有任何登记能修复。只改依赖版本则不会让任何子请求的父版本失配。

沿用计费关联的版本对模式。`cls_state` 取 `classified`、`pending:usage_link`(当前请求账单待复核)、`pending:candidates`(候选快照未完成); 三者都按版本写回, 待相应输入变化经传播重新登记, 不长期保持脏状态。以下事件在**同一事务**内给自身加版本:
- 索引 upsert
- 任何使 `usage_link_version` 或 `usage_link_checked_version` 变化的语句: `SetUsageLink`(含被夺回置为 released)、`ClearUsageLinkPending`、`RecordResponse`、`MergePendingResponses`
- 完成事实或转发侧记录变化
- 候选快照提交
- 规则、映射、`fp_version`、L/W/K/D 配置变化(按范围批量加)
- 维护任务发现 usage 已删除或费用变化

### 10.2 依赖传播

候选版本(第一行)的传播在源事务内完成, 并受候选锁保护(5.5); 分类版本(第二行)的传播在源事务**提交后**完成:

| 源变化 | 传播对象 |
| --- | --- |
| P 新增, 或指纹变化 | 同 Key、`(P, P + L]` 内所有行的 cand 和 cls 版本 |
| P 的其他输入变化 | 以 P 为父的边所指的子请求的 cls 版本; 再沿边逐层传播, 不超过 D 层、不超出 L |

实现:
- 源写入(`UpsertRequestSummary`、`ReplacePrefixEdges`、`RecordResponse`、`MergePendingResponses`、`SetUsageLink` 含被夺回的行、`ClearUsageLinkPending`)在事务内只给自身加 cls 版本, 并把自身写入失效登记表 `token_analysis_cls_invalidations`, 见 `markTokenAnalysisClassificationDirty`。每次登记(新建或重复)都从全局序列 `token_analysis_cls_invalidation_seq` 取新的 `seq`: 若只在行内加一, 登记被删除后重建又从 1 开始, 持有旧快照的另一消费者会按相同的 `(archive_id, seq)` 误删新登记(第三轮复核 T1, ABA, 已复现); 单实例锁是 best-effort, 不能排除多消费者。
- 分类作业每轮先处理登记: 读取 `(archive_id, seq)`, 从这些行出发逐层查前缀边(不含关联边)找子孙, 去重后最多 D 层、不按 L 截断(只会多算), 按 id 顺序加锁给登记行及其子孙加**依赖版本**(`propagateTokenAnalysisClassification`), 再按读取时的 `seq` 删除登记; 处理期间又被登记的行保留到下一批。过期或重复的消费者只会让这些行多重算一次。
- 为什么必须在提交后查边(复核 N3, 已复现): 若在源事务内查边, 并发提交的新边 C→P 对它不可见; C 若在 P 提交前按 P 的旧事实分类并写回, P 提交后再无通知。提交后查边则不会漏: 凡是可能读到 P 旧事实的子请求, 其边在 P 提交前已提交, 处理登记时必然可见; 之后才提交的边, 其子请求的分类已能读到 P 的新事实。
- 所选父请求的版本另由写回条件(10.3)与读路径(10.4)立即保护; 子孙在登记处理前(通常不超过一轮)保持旧的已对齐状态, 读路径的 `classified_through` 须把未处理的登记计入。

死锁处理(审核 R3, 已复现): 源事务在传播前已持有源行(晚到父请求的 id 可能大于子请求), 只靠"传播按 id 排序"不能统一加锁顺序。因此:
- 分类写回用 `FOR UPDATE ... SKIP LOCKED`: 正被源事务持有的行直接跳过(它的版本即将变化, 下一轮重算), 写回从不等待行锁, 不会与源事务互相等待。
- 源写入事务(摘要 upsert、写边、完成事实、暂存合并、关联写入与确认)与失效登记处理遇到死锁(`40P01`)或序列化失败(`40001`)时整体重试, 最多 3 次。这些事务只读写数据库、是输入的纯函数, 整体重放安全; 索引出错会推进水位, 不能只记日志放弃。
- 候选锁是事务里的第一把锁(5.5)。

### 10.3 写回

- 计算时记下子请求的 `cls_input_version`、`cls_dep_version`、`cand_checked_version`, 以及所选父请求的 `cls_input_version`。
- 有父请求时, 写回条件为这四个值都未变; 写回同时确认两对版本。
- 没有父请求的终态(`no_candidate`、`small`、`unsupported`、`ineligible`)只校验子请求自己的 cls 输入、cls 依赖与 cand 确认三个版本, 保证能写回, 不会一直保持脏状态。四个条件(含父请求输入版本)各自有效, 由 `TestTokenAnalysisClassificationCASGuardsAreIndependent` 逐项验证。
- 一批结果用一条 `jsonb_to_recordset` 条件 UPDATE 写回, 按 id 顺序加锁; 版本已变的行不写, 下一轮重算。

### 10.4 发布可见性

金额字段在读路径中按以下条件判定; 不满足时, 列表、排序和详情一律返回 NULL:
- 子请求的 cls 输入、cls 依赖、cand 三对版本都相等; `cls_state = classified`, 规则版本为当前版本
- 三类证据满足第 8 节的门槛
- 父请求当前的 `cls_input_version` 等于 `parent_cls_version_used`
- 子请求和父请求都满足 `usage_link_version = usage_link_checked_version`。这一条与 cls 版本检查相互独立, 防止重算先后顺序导致金额短暂重新出现。
- 子请求和父请求的 usage 行都能通过 JOIN 查到(不只是 id 非空)
- 不在进行中的重索引的不可发布范围内

接口同时返回 `classified_through`(最早一条脏行或未处理失效登记的时间)、覆盖率和规则版本。

### 10.5 作业资源

- 每分钟运行一次, 整轮期限 5 分钟; 使用 `tryAcquireSingletonLeaderLock`, TTL 10 分钟。
- 这把锁是 best-effort 的, 只用来减少重复工作; 正确性由 10.3 的条件写回保证。
- 每批 2,000 行, 单条 SQL 超时 30 秒; 每轮最多 20 批, 原文重读最多 500 行。
- 批次之间检查期限; 收到停机信号时完成当前批次后退出。失败的批次下一轮重试, 结果是输入的纯函数。
- 一批内分类器缺少祖先行或候选边时, 汇总缺口一次加载后对未完成的行重算, 每层回溯至多两次加载。
- 每轮先处理失效登记(10.2、10.7), 每批 2,000 条, 每轮最多 10 批。
- 维护扫描(每轮最多 10 段、每段 2 万个 id, 扫完全表后 1 小时再扫): 只给已对齐但 `cls_rule_version` 不是当前版本的行加**依赖**版本(不传播, 不改输入版本, 以免分段扫描使已重算的子请求与后扫到的父请求失配)。首次上线时全部存量行因规则版本为 0 被逐段登记。
- 作业在 `index_enabled` 时随服务启动(`ProvideTokenAnalysisService` 注入 `LeaderLockCache` 与 `*sql.DB`), 与自动索引共用停机控制; `RunClassification` 执行一轮(不取锁), 供测试与手动触发。

### 10.6 受控重索引

- 持久化任务 `token_analysis_reindex_jobs(id, range_start, range_end, phase, progress, started_at, finished_at)`。
- **开始时**: `[range_start, range_end + L]` 标为不可发布(金额为 NULL, 冷暖照常), 连续活跃显示"重建中"。
- **两遍处理**: 第一遍写入指纹, 第二遍重读原文计算候选边, 都关闭逐行传播。进度持久化, 重启后续做。
- **完成时**, 在一个事务内:
  - 给以下行的 cand/cls 版本加一: 范围内全部行、`(range_end, range_end + L]` 内同 Key 的行、以范围内请求为父的子请求。
  - 把任务标记为完成。
- **解除不可发布**: "两遍完成"和"依赖追平"是两个阶段。只有任务已完成, 且受影响范围内脏行数为 0, 才在同一事务内复查并解除; 重启或与在线索引交错时都不能提前解除。
- 在线索引在重建期间照常进行。

### 10.7 源数据变更

- usage 行只会被 DELETE: 保留期清理(`CleanupUsageLogs` 的分批 DELETE)和管理员清理任务(`DeleteUsageLogsBatch`)都会触发外键 `ON DELETE SET NULL`, 把 `usage_log_id` 置 NULL。10.4 用 JOIN 判定存在性, 金额立即不可见。
- 失效登记(migration 256/257): 摘要表上的 `BEFORE UPDATE` 触发器在 `usage_log_id` 由非空变为空时, 同一语句内给该行加 cls 版本, 并写入 `token_analysis_cls_invalidations`(`seq` 取全局序列新值); 与源写入的登记一样由分类作业在提交后沿前缀边传播(10.2)。
  - 不能靠扫描"账单为空但冷暖不是 no_usage"的行: 子请求可能已用父请求当时的事实分类, 父请求仍待分类时账单被删, 下一轮先把父请求分类成 no_usage, 扫描就再也找不到它, 子请求永远收不到通知(审核 R4, 已复现)。登记独立于分类结果, 不会被覆盖。
  - 应用代码自己置空 `usage_log_id`(解除关联、被夺回、重新索引)也会登记, 与事务内的传播重复, 无害。
- 正式 schema 中 `usage_logs` 不是分区表: migration 035 只在已手工分区时加分区。按分区 DROP 的清理只在手工分区的部署上发生, 且不会触发外键动作; 而本表引用 `usage_logs(id)` 的外键在分区表上无法建立, 生产成功应用过 migration 136, 说明生产不是分区表。
- 应用代码不会 UPDATE 已写入 usage 行的用量或费用字段(只有一次性 migration 回填过非费用列), 因此"usage 写入后不变"的前提成立, 不需要回看比对。`amount_inputs_hash` 仍保留, 用于发现关联到另一条账单。

## 11. 前置调查与转发侧记录

### 11.1 前置调查

2026-10-26 已按源码完成能在本地回答的部分:

| 项 | 结论 |
| --- | --- |
| 原生 Responses 的转发链路 | 路由进入 `OpenAIGatewayHandler.Responses`, 换号循环中每次尝试都调用 `OpenAIGatewayService.Forward`。非透传分支在 `buildUpstreamRequest`、透传分支在 `buildUpstreamRequestOpenAIPassthrough` 中定型 body 与请求头, 再经 `doOpenAIUpstream` 发出。账号配置可让请求改走 Chat Completions/Anthropic(`resolveUpstreamProtocolFor`) |
| `prompt_cache_key` | OAuth/SetupToken 账号: 只要账号有身份命名空间就按作用域确定性改写(`applyCodexAccountIdentityClientMetadataMap`); 指纹 session/full 模式(账号 extra `codex_fingerprint_mode`)换成账号级常量; 兼容桥标记的请求会被删除。API Key 账号从不改动。compact 请求会从 body 中去掉。由此门槛 4 改为比较最终缓存键哈希 |
| 其他缓存路由提示 | OAuth 非透传删除客户端 `session_id`/`conversation_id`, 由缓存键派生隔离值重设; 透传保留客户端头但同样做隔离。API Key 账号原样透传这些头。因此转发侧哈希同时包含最终的 `session_id`、`conversation_id` 头 |
| usage 改写 | 原生 Responses 解析 usage 后只做三桶互斥拆分; `ForceCacheBilling` 只在 `GatewayHandler.Messages` 设置, 原生 Responses 走不到; TTL 覆盖也只属于 Anthropic 网关。流式响应失败(如 `response.failed`)时不记录 usage |
| OpenAI `cache_creation` | 来自 `input_tokens_details.cache_write_tokens` 等字段(`openAICacheCreationTokensFromUsage`)。上游是否真的返回这些字段无法从代码判断, 首版继续排除 `cache_creation > 0` 的行 |
| WS 模式 | WS 端点是 GET, 归档中间件只归档 POST; WS 请求不进入 Token Analysis |
| usage 清理与改写 | 见 10.7: 只有 DELETE, 外键置空生效; 应用代码不改写已写入的用量和费用 |
| 发送模型 | 见 6.2: 全平台统一为 `upstream_model` 否则 `model` |

仍需服务器只读调查(需要时另行安排导出):
1. ~~Codex `tools_count` 多为 0 的原因~~: 已由 2026-10-27 结构样本确认(工具定义在 `input` 的 `additional_tools` 条目中), 结论见 4.1、5.2。
2. 平台文档中的最短缓存保留期与 `prompt_cache_retention` 语义, 用于确定 W。
3. OpenAI OAuth 插件路由(`PluginManager.RoundTripOpenAIOAuth`, 外部进程)是否改动请求; 启用该路由的账号在确认前记为 unknown。
4. Anthropic 支持(后续阶段)所需的强制缓存计费、TTL 覆盖、`cache_control` 改写的触发统计。

### 11.2 转发侧记录(小改动, 不改变转发行为)

- **不设全局默认值。** 每次转发尝试进入 `Forward` 时清空上一次尝试的记录, 并记下入口的 `prompt_cache_key`。只有 `Forward` 内调用两个原生请求构造函数(`buildUpstreamRequest`、`buildUpstreamRequestOpenAIPassthrough`)的两处调用点写入记录; 构造函数本身也被兼容转换路径复用, 所以不在构造函数里写。改走协议转换或其他平台的尝试不写, 一律为 unknown。换账号重试时, 最终留下的是最后一次尝试的记录。
- **实际 transport 是插件时降级。** 发送由 `doOpenAIUpstreamReportingTransport` 报告本次是否实际交给了 OpenAI OAuth 插件(route 可随时切换, 只有这里的判定与真实 transport 一致)。交给插件时, 记录改为 `forward_path = openai_responses_plugin`, 只保留缓存键处理方式作观测, 清空哈希与 usage 来源; 资格判定为 unknown, 直到插件行为核实(11.1)。
- 记录内容:
  - `forward_path`: `openai_responses`(非透传)或 `openai_responses_passthrough`
  - `cache_key_handling`: 比较最终 body 与入口的 `prompt_cache_key`, 相同为 `passthrough`, 两边都没有为 `absent`, 否则为 `scoped` / `injected` / `deleted`, 只作观测
  - `upstream_cache_key_hash`: 对最终 body 的 `prompt_cache_key` 与最终请求头 `session_id`、`conversation_id` 的**原值**做 HMAC(密钥见 5.3), 取 128 位十六进制。不做 TrimSpace: 首尾空白不同的键会被原样发往上游, 是不同的键(审核 R5); 入口 `client_cache_key_hash` 同样按原值
  - `upstream_items_hash` / `upstream_root_hash`: 最终 body 的条目链末端与 h0, 算法、密钥、端点族与索引器对归档正文的计算相同(`tokenAnalysisUpstreamPromptHashes`), 用于门槛 4 验证网关没有改写条目。正文取自原生构造器产出的请求(`GetBody`), 即实际交给 transport 的字节, 包含构造器内部的改写(如 DeepSeek 原生 Responses 的 `normalizeDeepSeekResponsesRequestBody`, 复核 N2); 最终缓存键也从这份正文读取。每次尝试只保留取正文的函数, 在归档读取记录时才读取与计算: 这是在同一请求协程内、响应结束之后的后移计算, 不是异步, 也不是零成本; 插件路径不计算哈希
  - `usage_cache_source = upstream_parsed`: 原生 Responses 的 usage 管线不改写缓存桶(见 11.1), 由路径保证
- 只有小型字符串和哈希, 不保存转发正文。
- 链路:
  1. 归档 response 记录在 `c.Next()` 之后读取这些值, 写入 `TokenAnalysisResponseFacts`。
  2. 响应比请求先到时, 进入 `token_analysis_pending_responses`(新增对应列), 再经 `MergePendingResponses` 合并。
  3. 值变化时按 10.1 触发重算。
- 上线前的历史行一律为 `unknown:transform_policy`、`usage_cache_source = unknown`。
- 存储: migration 250(路径、缓存键、usage 来源)与 255(`upstream_items_hash`、`upstream_root_hash`), 摘要表与暂存表各一份。

## 12. 存储、配置、接口与权限

### 12.1 新 migration

只新增, 所有字段可空或有默认值。

摘要表新增列:
- 客户端: `user_agent`、`client_family`、`client_family_version`
- 指纹: `fp_version`、`fp_state`、`prefix_end_hash`、`prefix_tail1_hash`、`items_end_hash`、`client_cache_key_hash`(均为 bytea 16)、`prefix_items`(int4)、`has_continuation_ref`
- 原文定位: `source_line_start`、`source_line_length`
- 转发记录: `forward_path`、`cache_key_handling`、`upstream_cache_key_hash`、`usage_cache_source`、`upstream_items_hash`、`upstream_root_hash`(migration 250/255)
- 候选快照: `cand_input_version`、`cand_checked_version`、`cand_state`、`cand_overflow`
- 冷暖与归因: `cache_obs_state`、`cls_input_version`、`cls_checked_version`、`cls_dep_version`、`cls_dep_checked_version`(259; 待分类部分索引 `idx_token_analysis_cls_due` 由 260 建立, 替换 254 的 `idx_token_analysis_cls_pending`)、`cls_rule_version`、`cls_state`、`cause`、`prefix_evidence`、`cache_eligibility`、`price_evidence`、`parent_archive_id`、`parent_cls_version_used`
- 金额: `reusable_tokens`、`recoverable_tokens`、`amount`、`cold_input_cost`、`amount_inputs_hash`、`classified_at`(migration 253; `amount_estimated` 随估算实现再加)

其他:
- `token_analysis_pending_responses` 增加 11.2 的标量列。
- 失效登记表 `token_analysis_cls_invalidations(archive_id, seq, created_at)` 与摘要表触发器 `token_analysis_usage_detached`(migration 256; `seq` 由 257 加入, 258 改为取全局序列 `token_analysis_cls_invalidation_seq`)。
- 附表: `token_analysis_prefix_edges`、`token_analysis_key_hour_activity`、`token_analysis_key_activity_runs`、`token_analysis_reindex_jobs`, 以及(Key, 小时)的脏标记。
- 索引: `(api_key_id, prefix_end_hash)`、`(api_key_id, prefix_tail1_hash)`、`(api_key_id, items_end_hash)`(非 NULL 部分索引); 边表 `parent_archive_id`; 部分索引 `WHERE cls_input_version <> cls_checked_version`、`WHERE cand_input_version <> cand_checked_version`。

### 12.2 容量

- 每个子请求最多 2K 条边, 与请求数呈线性关系; 1,000 条连续追加约 2,000 条边。
- 摘要行每行增加约 250 字节, 另加 6 个索引。
- 验收用真实分布与极端场景(长会话、同 Key 高频相同输入、持续追加、多协议分支)测出边数、存储、批算吞吐和传播成本, 以实测为准。

### 12.3 配置

| 键 | 默认 | 校验 | 变化时 |
| --- | --- | --- | --- |
| `token_analysis.classification.observation_enabled`(运行时设置) | false | 布尔 | 只影响展示 |
| `token_analysis.classification.amount_enabled`(运行时设置) | false | 布尔 | 只影响展示 |
| `token_analysis.classification.lookback_hours`(L) | 24 | 1-72 | 保留期内范围加 cand/cls 版本(首版为代码常量 `TokenAnalysisCandidateLookback`, 暂不可配置) |
| `token_analysis.classification.reuse_windows`(W, 按平台/模型) | openai: 300 秒 | 30-3600 秒, 须注明文档依据 | 保留期内范围加 cls 版本(首版为代码常量 `tokenAnalysisOpenAIReuseWindow`) |
| `token_analysis.classification.max_tie_candidates`(K) | 8 | 1-64 | 加 cand 版本(首版为代码常量, 暂不可配置) |
| `token_analysis.classification.max_ancestor_depth`(D) | 64 | 1-256 | 加 cls 版本(首版为代码常量 `TokenAnalysisClassificationMaxDepth`) |
| 规则版本 | 代码常量 `TokenAnalysisClassificationRuleVersion` | — | 维护扫描逐段给旧版本行加 cls 版本 |

YAML 配置在启动时与已存配置的哈希比对, 变化时按"变化时"一列处理。

### 12.4 接口与切换

- DTO 新增: `cache_obs_state`、`usage_cache_source`、`usage_link_pending`、`cause`、三类证据字段、`amount`、`amount_estimated`、`cold_input_cost`、`classified_through`、覆盖率。
- `Requests` 接口的 `sort_by` 白名单增加 `amount`、`cold_input_cost`; 并列时按 `event_time desc, id desc` 稳定排序。
- 新增筛选: `cache_obs_state`、`usage_cache_source`、`cause`、`cls_state`。
- 汇总单独返回 pending/unknown 计数。
- 页面区分"0""计算中""无法估算""不支持""重建中"。
- 切换分三个阶段:
  - A: `observation_enabled` 控制, 新旧并存, 旧风险分照常计算。
  - B: 默认显示新版。
  - C: 另一次发布中下线旧筛选, 停止计算旧规则。
- 回切: A、B 可由设置即时回切; 从 C 回切需先运行旧评分补算任务覆盖停算期间的行, 补算完成前旧分数显示"未计算"。

### 12.5 权限

- 新增的 GET 接口逐条加入 `AdminPermissionTokenAnalysis` 白名单, 不按前缀放行。
- 配置写入、重算和重索引触发只给完整管理员。

## 13. 关键约束与依据

以下约束在设计审核中逐条对照源码确认。它们不直观, 实施或后续修改时不要改回去:

| 约束 | 依据 |
| --- | --- |
| 不合成单一风险分, 各维度分开且不相加 | 旧规则一条命中 95.5%; 同一请求可同时命中多个维度 |
| 入口 body 相同, 不等于上游缓存键相同 | 归档在 `c.Next()` 之前采集入口 body。`prompt_cache_key` 会被改写、删除或注入: 账号作用域改写(`openai_codex_account_identity.go`, `scopeCodexAccountIdentityValue`)、指纹会话(`openai_codex_fingerprint.go`)、兼容桥删除(`openai_codex_transform.go`、`openai_gateway_messages.go`)、Grok 设置或删除(`openai_gateway_grok_cache.go`) |
| 账单里的缓存桶不一定是上游事实 | `recordUsageCore` 的 `ForceCacheBilling` 把普通输入改记为缓存读; `applyCacheTTLOverride` 重分 5m/1h; `gateway_forward.go` 会改写 `cache_control`、注入 1h TTL。因此金额首版只做 OpenAI 原生 Responses |
| 稀疏检查点不能证明完整前缀 | 父请求 6 条、子请求 7 条时, 检查点 {1,2,4,6} 与 {1,2,4,7} 只能证明共同前缀到 4; 改用"末端哈希 + 子请求在内存中的完整链" |
| 不能保存全部前缀候选 | 连续追加时, 每个历史请求都是后续请求的前缀, 候选数按平方增长(1,000 次追加约 50 万对); 只存最长边并回溯 |
| 候选集合要单独做版本 | 否则重新匹配期间晚到的更优候选, 会被旧集合覆盖并按新版本发布 |
| 冻结的价比只能当估算 | 首次分类发生在计费之后, 冻结的价比不等于历史价比; 合成例子中, 历史差额 2 被算成 9 |
| OpenAI 提示词量是三个桶之和 | `openai_gateway_usage.go` 中入库 input 已扣除 cache_read 与 cache_creation |
| 发送模型回退到 `usage_logs.model`, 而不是 `requested_model` | OpenAI 写账单调用 `upstreamSentModel(result.Model, result.UpstreamModel)`; `requested_model` 是映射前的客户端名 |
| 已有 usage 行不等于账单已确认 | `RecordResponse` 登记复核时只增加版本, 不清空旧 `usage_log_id`; 分类版本已确认也不等于关联已确认 |
| `retry_until` 不代表仍在 pending | 它只是等待晚到账单的截止时间; 已确认的行可以同时保留未来的 `retry_until` |
| `source_offset` 是行尾 | 索引循环写入的是 `offset + len(line)`, 直接 Seek 会读到下一行 |
| TTL 从父请求开始时刻起算 | 缓存可能在输入处理阶段就已写入, 从完成时刻起算会接受已过期的长流式场景 |
| 单实例锁不保证正确性 | `tryAcquireSingletonLeaderLock` 是 best-effort 的(Redis 租约不续期, 出错时降级为 PG 锁) |
| 图片输入单独计费 | `computeTokenBreakdown` 把图片输入记到 `ImageInputCost`, 图片缓存读按单独单价 |
| 长上下文标记不覆盖区间价 | 有区间定价时不走模型阶梯; 只持久化了 bool, 没有基准费用 |
| 重试/重复视图移出 | 金额不足 0.31%; 判断失败需要 SSE 终态和请求内部换号的证据, 不在本数据范围; count_tokens 的 not_billable 不是失败 |
| 连续活跃只说"观察到" | 归档队列满会丢行, 无记录不等于停机 |
| 比较最终缓存键, 不是"缓存键是否被改写" | OAuth/SetupToken 账号的缓存键总被按作用域确定性改写; 只接受"未改写"会让几乎全部 Codex 流量不合格 |
| 转发记录每次尝试先清空 | 一次网关请求内可能换号或改走协议转换; 不清空会把前一次原生尝试的记录留给后一次转换路径 |
| WS 请求不在分析范围 | 归档中间件只归档 POST, WS 端点是 GET |
| `client_metadata` 不计入 `h0` | Codex 的 turn/thread 元数据频繁变化但不进入提示词, 计入会让几乎所有 Codex 请求对不上(2026-10-27 样本) |
| 父请求在同 Key 的全部更早请求中查找 | Codex 同一会话交替运行多个线程, 相邻请求常属于不同线程 |
| 允许去掉父请求末尾 1 条, 但不进金额 | Claude Code 每轮替换末尾临时 system 消息; 被去掉的条目不属于子请求前缀 |
| `tools_count` 要算 `additional_tools` | Codex 的工具定义不在顶层 `tools` |
| 晚到父请求只标"同版本且已有指纹"的后续行 | 否则按文件顺序首次补算历史指纹时, 每写一行都会把之后 24 小时的行全部标脏 |
| 原生路径不等于提示词未变 | Forward 内的触发器重排与加密 reasoning 剥离会改条目; 用最终条目哈希与入口条目哈希相等来证明, 不用路径名 |
| 新父请求与子请求候选查找同 Key 串行 | 只靠版本 CAS 时, 未提交的父请求与刚提交的子请求互相看不到, 漏掉的候选永远不会补回 |
| 分类写回 SKIP LOCKED, 源事务死锁重试 | 源事务先持有源行再传播, 排序不能统一锁顺序; 写回从不等锁, 源事务可整体重放 |
| 分类版本在源事务提交后传播 | 源事务内查边看不到并发提交的新边; 提交后查边才能覆盖所有可能读到旧事实的子请求 |
| 最终提示词证据取自构造器产出的请求 | 构造器内部还会改写正文, 调用方手里的字节不是实际发送内容 |
| 传播只改依赖版本 | 改输入版本会让查完子孙后新挂上的子请求与父版本永久失配; 输入版本只随本行事实变化并总有登记 |
| 失效登记的 seq 取全局序列 | 行内递增在删除重建后会复用旧值, 旧消费者误删新登记(ABA) |
| 账单删除用触发器登记 | 扫描"账单为空但未判 no_usage"会被后续分类覆盖, 子请求永久收不到通知 |
| 缓存键按原值哈希 | 网关原样发送首尾空白, TrimSpace 会把不同的键归并成相同证据 |
| 重读校验对正文重算哈希 | 行内 `body_sha256` 只是元数据, 正文变了它仍可能是旧值 |
| 重复键不单独检测 | 单独检测需要再逐 token 解析一遍, 成本与解码相当; 解码结果与网关、上游一致为后者生效 |

## 14. 验收

| 场景 | 期望 |
| --- | --- |
| 6→7、9→10、513→514 条正常追加 | `full_prefix`, `matched_items` 等于父请求条目数 |
| Codex 同一 session 交替的主/子 agent 线程(2026-10-27 样本 A2、A3) | 各线程找到本线程的父请求, 不因相邻请求属于其他线程而判为前缀变化 |
| Claude Code 末尾临时 system 消息每轮被替换(样本 C-claude_code) | `prefix_minus_tail`, 不进入金额 |
| Codex `client_metadata` 每轮变化 | 不影响前缀匹配 |
| 1,000 次连续追加 | 边数为线性; 回溯能找到合格的父请求 |
| 同 Key 高频相同输入, 并列超过 K | `unknown:tie_overflow`, 无金额 |
| system/tools 变化; 同 Key 两段对话模板相同 | `prefix_changed`, 或另一段对话不是前缀候选 |
| 更早但更长的父请求晚到 | C 的 cand/cls 版本加一; 重新匹配后改选更长者 |
| 重新匹配进行中又出现更好的候选 | 旧的候选提交被条件拒绝; 分类为 `pending:candidates` |
| 分类读取后父请求被更新 | 写回失败, 下一轮重算 |
| 当前请求先经 timing 关联到 U0, 再到达需要复核的完成事实, 分类先于关联任务运行 | 金额持续不可见, 直到复核完成并重算; 冷暖显示"账单待复核" |
| 父请求已确认关联, 但 `retry_until` 在未来 | 照常判定; 仅经 `ClearUsageLinkPending` 确认时, 依赖的子请求被重新判定 |
| 无父终态 | 能写回, 不长期保持脏状态 |
| 父请求流式运行超过 W, 子请求紧随其后 | `beyond_reuse_window` |
| Anthropic 强制缓存计费(上游冷、计费为热) | 计费口径为 hit, `usage_cache_source = rewritten`, 无金额 |
| 父子最终缓存键哈希不同(如指纹模式切换); 改走协议转换; 缺少转发记录 | `cache_key_changed`; `unsupported_path`; `unknown:transform_policy` |
| OAuth 账号缓存键被按作用域改写, 同账号前后两次 | 最终哈希相同, 不因改写而不合格 |
| 响应先于请求入库 | 转发记录经暂存表合并, 触发重算 |
| 续接引用、字符串 input 与数组 input 之间 | unsupported 或不匹配 |
| OpenAI 三桶(input=1,024、cache_creation=98,304) | `unknown:openai_cache_write` |
| 有图片的行; 子请求无缓存读 | `price_evidence = unavailable`; 只有 estimated |
| `OriginalModel=public-alias`、`result.Model=mapped-model`、`UpstreamModel` 为空 | 发送模型为 mapped-model |
| 图文混合的冷请求(文本输入费 0、图片输入费 10) | `cold_input_cost` 包含 10 |
| 价卡更新前后重算 | `amount` 不变 |
| usage 被删除 | 金额立即不可见, 随后重算 |
| 按范围重索引, 范围外有子请求依赖范围内父请求; 两遍完成后、追平前重启 | 子请求版本加一; 追平前一直不可发布 |
| 原生转发重试剥离加密 reasoning; 触发器被移到末尾 | `prompt_rewritten`, 无金额 |
| 父请求写入事务未提交时子请求入库并查候选 | 子请求等父请求提交后才查, 得到指向父请求的边或保持待重新匹配 |
| 晚到父请求 id 大于子请求, 源事务持有父行时分类写回 | 写回跳过父行、不死锁; 源事务死锁时整体重试 |
| 父请求待分类时账单被删, 下一轮先把父请求判为 no_usage | 子请求经失效登记重新待分类 |
| 缓存键 `"k"` 与 `" k "` | 最终缓存键哈希不同 |
| 归档正文被改而行内哈希不变; 合法正文转义后超过 128 MiB | 不可用; 可重读 |
| 纯文本字符串 content | `image_count = 0`, 价格证据可为 `row_exact` |
| 工具结果含 `input_image` | `image_count = 1`, `unavailable:image_input` |
| DeepSeek 原生 Responses 构造器提升工具图片 | 记录的提示词哈希等于实际发送正文的哈希 |
| P 的事实变化未提交时 C 建立 C→P 并按旧事实分类 | P 提交后处理登记, C 重新待分类并按新事实重算 |
| 两种单价分别舍入会偏差的金额 | 只在最后舍入一次 |
| 登记被消费者 A 删除后由新源事务重建, 消费者 B 持旧快照删除 | 新登记保留, 子请求随后重算 |
| 过期消费者在 C 挂上并写回之后才处理 P 的旧登记 | P 的输入版本不变, C 的父版本仍一致 |
| namespace 只有 `children`、或 `tools` 为空回退 `children` | 工具数按叶子计入 |
| 回溯超过访问预算; 同一祖先经多条边可达 | `unknown:walk_budget`; 同一节点只访问一次 |
| 旧行重读(有尾换行、无尾换行、文件已压缩) | 读到本行, 或 `unknown:body_unavailable` |
| 补入 10 天前的一小时活跃 | 对应区间被延长或合并 |
| 新旧前后端组合、子管理员、新排序、从 C 阶段回切 | 无 400、无越权; 未知与 0 可区分; 补算前显示"未计算" |

测试方式:
- 单元测试: 规范化、指纹、边选择与回溯、资格、金额、传播谓词。
- 集成测试: migration、条件写回、候选快照条件提交、原子传播、重索引任务, 在嵌入式 PG 上验证。
- 回放: 新算法在服务器上以只读方式, 从归档原文为 Codex、Claude Code、Copilot、hertz 各取连续请求, 人工标注父请求和可判定性, 约 200 对。旧样本包只能验证降级路径。

## 15. 实施顺序

1. 11.1 前置调查: 定稿支持路径清单、W 和 5.2 的字段划分。
2. 11.2 转发侧记录, 以及归档和暂存表字段。尽早上线, 积累有证据的新行。
3. 摘要器修复、客户端识别、指纹、候选边与快照版本、原文定位字段; 受控重索引。
4. 冷暖状态、原因、版本对与传播、批任务; 上线观察页面(阶段 2)。
5. 连续活跃表与区间。
6. OpenAI 原生 Responses 金额(阶段 3), 验收通过后打开开关。
7. 完成后同步 `llm-wiki/wiki/backend.md`、`frontend.md`、`data-and-domain.md`、`security-and-reliability.md`。

## 16. 实施进度

| 项 | 状态 |
| --- | --- |
| 4.1 摘要器修复 | 已实现; 工具输出判定与来源分类共用 `tokenAnalysisIsToolOutputItemType` / `tokenAnalysisIsToolResultBlock` |
| 4.2 客户端识别 | 已实现(migration 249); 规则版本变化后由自动索引按存下的 UA 补映射 |
| 11.1 前置调查(源码部分) | 已完成, 结论见 11.1 表 |
| Codex 结构样本(2026-10-27) | 已分析: `tools_count` 计入 `additional_tools`(已实现); `client_metadata` 加入白名单、去尾 1 条匹配(已写入 5.2/5.4, 随指纹实现落地) |
| 11.2 转发侧记录 | 已实现(migration 250); 指纹密钥经 `security_secrets` 引导 |
| 2026-10-26 数据基础层代码审核 | 4 项已修正: 插件 transport 降级(F1); `tool_search_output` 结果取 `tools`(F2); 客户端族补映射改为游标分批 + 低频复查(F3); 写回按 UA 与版本快照条件提交(F4)。审核报告见 `docs/reviews/2026-10-26-token-analysis-data-layer-code-review-cn.md` |
| 5 前缀指纹与候选 | 已实现(migration 251/252): 指纹、去尾哈希、候选边与快照版本对、晚到父请求与父指纹变化的传播、索引时计算、重新匹配任务、行首/长度定位; K/L 为代码常量。生产结构样本回放与离线分析一致(Codex 完整前缀 27/26/27, Claude Code 去尾 29/29, Copilot/hertz 28/28) |
| 6-8 冷暖、原因、资格、金额; 10.1-10.3 版本对、传播、条件写回; 10.5 分类作业与维护扫描; 10.7 账单删除 | 已实现(migration 253/254): `token_analysis_classification.go`(分类器, 纯函数)、`token_analysis_classification_job.go`(作业)、`token_analysis_classification_repo.go`。W/D/阈值为代码常量; 估算金额、`session_link`、`fp_version_mismatch` 首版不实现 |
| 2026-10-27 分支审核(8 项) | 已修正(migration 255/256): R1 最终提示词哈希门槛; R2 同 Key 候选锁与锁内查找; R3 写回 SKIP LOCKED + 源事务死锁重试; R4 账单删除触发器与失效登记; R5 缓存键原值哈希; R6 重读正文重算哈希并核对存储指纹; R7 图片计数只认图片字段/块; R8 归档行上限按正文上限与转义推出。另: 当前请求缺完成事实按待复核; 候选查找结果有界。审核复现改写为回归测试。审核报告见 `docs/reviews/2026-10-27-token-analysis-branch-code-review-cn.md` |
| 2026-10-27 第二轮复核(4 项) | 已修正(migration 257): N1 工具结果图片计入图片数; N2 最终提示词证据取自构造器产出的请求(`GetBody`); N3 分类版本改为源事务内自登记、提交后由作业沿边传播; N4 金额单次舍入。复现改写为回归测试。报告见 `docs/reviews/2026-10-27-token-analysis-branch-rereview-cn.md` |
| 2026-10-27 第三轮复核(1 项) | 已修正(migration 258): T1 失效登记 seq 改取全局序列, 消除删除重建后的 ABA; 复现改写为回归测试 `TestTokenAnalysisInvalidationSeqCannotReuseDeletedGeneration`。报告见 `docs/reviews/2026-10-27-token-analysis-branch-third-review-cn.md` |
| 2026-10-27 第四轮复核(2 项) | 已修正(migration 259/260): F1 分类版本拆为输入版本与依赖版本, 传播/规则扫描/候选标记只改依赖版本; F2 namespace `children` 别名按适配器口径计入工具数。另删除示例配置重复注释。复现改写为回归测试 `TestTokenAnalysisStaleConsumerCannotStrandCleanChild`、`TestTokenAnalysisToolCountNamespaceChildren`。报告见 `docs/reviews/2026-10-27-token-analysis-branch-fourth-review-cn.md` |
| 2026-10-27 第五轮复核 | 通过, 无 P1/P2。P3 已处理: 条件写回测试改为逐项隔离(`TestTokenAnalysisClassificationCASGuardsAreIndependent`), 10.3 无父终态的校验表述已同步。报告见 `docs/reviews/2026-10-27-token-analysis-branch-fifth-review-cn.md` |
| 10.4 读路径可见性、12 接口与页面、9 连续活跃、10.6 受控重索引任务 | 未开始 |
