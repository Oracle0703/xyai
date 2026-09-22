# 共享算力池保障方案 三方案对比 评审意见

评审对象：`docs/features/shared-compute-pool-three-options-comparison-cn.html`（另参 `shared-compute-pool-protection-report-cn.md`）

评审日期：2026年9月22日 · 依据：当前工作区源码（分支 `feature/hy/10207_department_usage`）

评审方式：逐条核对方案引用的代码位置，并追查方案未引用但对结论有决定性影响的路径。文中所有行号均为核对时的实际位置。

---

## 〇 结论摘要

文档本身的质量是好的：结构清晰、边界表比多数方案稿都诚实、演示器逻辑自洽（已手工复算 steady 场景至第 8 步，方案一剩 300 专用、方案二落到 210、方案三留 250，与页面 takeaway 完全一致）。

问题不在表达，在地基。

| 序号 | 问题 | 严重度 | 影响 |
| --- | --- | --- | --- |
| 1.1 | R 的数据源 `actual_cost` 含用户计费倍率，与方案自身验收标准冲突 | 阻断 | 保底线随折扣缩水 |
| 1.2 | 目标号池（OAuth 账号）不存在美元口径的余量账本，`E − R` 无定义 | 阻断 | 方案二/三的核心公式不成立 |
| 1.3 | “移出含 5 小时限额的账号”可能是空集 | 阻断 | 上线前置条件无法验收 |
| 2.1 | 准入点选在 `CheckBillingEligibility`，全项目 23 处调用 | 高 | 工作量估算偏低 |
| 2.2 | 组级原子限速是全新一致性面；限请求数不等价限 Token | 高 | 方案三增量远大于 1 人日 |
| 2.3 | 恢复 FSM（多实例/通知丢失/重启）成本被压进“回归测试 0.5–1 人日” | 中 | 试点期返工风险 |

核心判断：**方案二和方案三的主干公式在目标号池上目前没有定义**，不建议现在开工。方案一可立即落地。另建议一条第四路线（分组级窗口水位门），在不做任何单位换算的前提下同时拿到方案一的隔离质量和方案二的资源效率。

---

## 一 阻断级问题：R 与 E 不是同一个单位

方案二、方案三的全部判定都归结为一个减法：有效余量 E 与需求基线 R 比较。核查结论是这个减法在目标号池上没有定义。

### 1.1 R 这一侧：引用的数据源与方案自己的验收标准正面冲突

文档将 R 定义为“普通组近 7 天累计用量 × 1.05”，数据源列为 `backend/internal/repository/custom_group_usage_rollup_repo.go`。

该表聚合的是 `actual_cost`：

- `custom_group_usage_rollup_repo.go:97` — `COALESCE(SUM(rollup.actual_cost), 0) AS actual_cost`
- `custom_group_usage_rollup_repo.go:256` — 日桶重建时同样取 `SUM(actual_cost)`

而 `actual_cost` 的定义是：

```go
// backend/internal/service/billing_service.go:1616
bd.ActualCost = bd.TotalCost * rateMultiplier
```

`rateMultiplier` 中揉入了分组倍率、用户级覆盖倍率与高峰倍率（见 `gateway_profit_control.go:40-44` 的同源解析链）。

**影响**：给普通组打八折，R 立刻下降 20%，保底自动缩水。这与文档第 06 节边界表第 2 行写明的验收要点“改用户倍率不改变保护量”直接矛盾。

**修复代价（方案未计入）**：口径上应改用 `total_cost`（`usage_logs` 中“原始总费用”，`backend/migrations/001_init.sql:157-158`）。但 `usage_group_daily_rollups` 表没有这一列，需要加列 + 回填 + 汇总水位失效重算。当前工作量表中“规则与数据口径确认 0.5–1 人日”装不下这项。

**附带的好消息**：滚动 168 小时查询本身可行，`backend/migrations/062_add_scheduler_and_usage_composite_indexes_notx.sql` 已建 `idx_usage_logs_group_created_at_not_null`。这部分不构成性能风险。

### 1.2 E 这一侧：OAuth 账号在本项目中没有美元口径的额度账本

这是更硬的问题。共享算力池中是 OAuth 订阅账号（ChatGPT / Codex、Claude），其额度账本只有百分比：

```go
// backend/internal/service/account_scheduling_threshold_eval.go:250-282
case "7d":
    usedPercentKey = "codex_7d_used_percent"
    resetAtKey     = "codex_7d_reset_at"
```

项目中确实存在账号级的美元周额度（`quota_weekly_limit` / `quota_weekly_used`，`account.go:2544`、`account.go:2994-3005`），但它只对 apikey / bedrock 类型账号递增：

```go
// backend/internal/repository/usage_billing_repo.go:204
if cmd.AccountQuotaCost > 0 && (strings.EqualFold(cmd.AccountType, service.AccountTypeAPIKey) ||
                                strings.EqualFold(cmd.AccountType, service.AccountTypeBedrock)) {
```

调度侧同样如此：`gateway_scheduling.go:1313-1318` 的 `isAccountSchedulableForQuota` 开头即 `if !account.IsAPIKeyOrBedrock() { return true }`。

**结论**：`E = Q − D` 中的 Q，在目标号池上不存在。上游只给出“某个不透明容量窗口的已用百分比”，而 R 是美元。两者不可减。

**这不是边界，是前提**。文档在第 06 节边界表第 2 行承认了该限制，但随后的第 01 节评分表（“普通组保障 88”）、第 02 节四个情景演示、第 04 节公式表，均按该减法成立来叙述。建议在口径确认前不要给出保障维度的具体分值。

### 1.3 “含 5 小时限额的账号移出目标号池”可能是空集

上线前置条件要求先移出含 5 小时限额的账号、只保留周额度账号。核查发现平台侧不存在“只有周窗口”的账号类型：

| 平台 | 窗口 | 位置 |
| --- | --- | --- |
| OpenAI | 每个账号同时产出 `5h` 与 `7d` 候选 | `account_scheduling_threshold_eval.go:187-190` |
| Anthropic | `session_window_utilization`(5h) + `passive_usage_7d_utilization` | 同文件 `:284-305` |
| 国产 Coding Plan | `5h` + `weekly`（opencode-go 另有 `monthly`） | 同文件 `:355-367` |

若该条实际指的是“配置了 `window_cost_limit` 的账号”（那确实是 5h 窗口费用阈值，`account.go:3009-3019`），属于另一件事，需要改写。

**建议**：把这条前置条件改写成可验收的判定式——明确是按账号类型、按套餐、还是按某个具体配置字段筛选，并给出预期账号清单规模。否则“已移出”无法判定完成。

---

## 二 高风险问题：工作量估算的落点选错了

### 2.1 准入点在 `CheckBillingEligibility`，有 23 处调用

方案二/三将准入判断放在“HTTP / WS 准入”层。该层的统一入口为 `BillingCacheService.CheckBillingEligibility`（`billing_cache_service.go:735`），全项目生产代码共 **23 处**调用，分布于：

```
gateway_handler.go(×3)        openai_gateway_handler.go(×3)   gateway_handler_responses.go
gateway_handler_chat_completions.go   gateway_web_search.go    gemini_v1beta_handler.go
grok_audio.go(×2)             grok_media.go                    openai_alpha_search.go
openai_chat_completions.go    openai_embeddings.go             openai_images.go
openai_gateway_count_tokens.go(×2)    openai_live.go           …
```

更本质的问题是：**准入层此时还不知道这次请求会落到哪个账号、哪个资源范围**。要在这里判定“池余量”，必须另建一套资源范围解析，与现成的 `listSchedulableAccounts` + 调度器重复。文档第 06 节“池总量有余而目标模型耗尽”那一行，根源正在于此。

**建议**：准入点下移到调度候选过滤层（见第三节）。这条对三个方案都成立。

### 2.2 组级原子限速是全新对象

现有的分组 RPM 是 per (user, group) 计数、Redis 故障 fail-open（`billing_cache_service.go:788-855`）。文档已正确指出它“不能直接当作全组限速”。

要做“整组共享的原子限速计数”，意味着：新 Redis key 设计、新的跨实例一致性面、新的 fail 策略（这里不能 fail-open，否则保护失效）。方案三还要在其上叠加档位切换、迟滞、连续观测计数——这是一个完整的分布式状态机，不是“在方案二基础上增加约 1 人日”。

另外文档自己也写明：“限请求数不能等价限制 Token 或费用”。档位限速挡不住单个 200k context 的大请求；要真正封顶必须做原子预占 + 未知输出上界，而那被明确排除在范围外。**即限速档位无法给出保底的硬保证，只能减缓速度**——这一点在第 01 节“普通组保障 88 分”中被高估了。

### 2.3 恢复 FSM 的成本被压扁

第 06 节边界表已完整列出多实例重复任务、通知丢失、服务重启、人工停用不自动恢复、进入/退出阈值分离、独立新鲜快照判定等要求。这些是分布式状态机的标准难点，但在第 07 节工作量表中全部落在“回归测试与试点准备 0.5–1 人日（方案二）/ 1–1.5 人日（方案三）”内。

**建议**：把恢复 FSM 单列为一个工作包并独立估算，或在首期明确采用“只记录不执行”的观察模式，把 FSM 推到第二期。

---

## 三 建议方案（第四条路线）：分组级窗口水位门

> **机制**：给分组配置一个“可用水位上限 X”。高用量组只能调度 `used_percent < X` 的账号；每个账号最后的 `(100 − X)%` 在物理上只有普通组能使用。

这条路线不是对方案二/三的修补，是换一个保底的计量面：**从“池的金额余量”换到“每个账号自己的窗口水位”**。

### 3.1 为什么它更好 — 逐条对应上文问题

**单位天然一致（解 1.1 / 1.2）**
比较发生在同一个账号的同一个窗口内，percent 对 percent。不需要 `$ ↔ %` 换算，不需要 `total_cost` 加列回填，不需要 168 小时滚动统计，不需要“消耗修正 D”这个新账本。1.1 与 1.2 同时消失。

**保底量随真实容量自动缩放**
保底总量 = Σ (100 − X)% × 各账号真实周容量。增删账号、更换套餐，保底自动跟随，运营不必重新核定 R。文档第 06 节“历史用量低于真实需求”“样本不足时人工确认启动基线”这一整类问题不再存在。

**错峰重置天然正确**
每个账号的 `used_percent` 在其自身 `reset_at` 之后由上游归零。不需要“预计重置只触发后台查询、不能提前记为可用”这套规则，也不需要跨账号的恢复观测协调。

**“先减速后暂停”不必新造轮子**
Anthropic 账号已有成熟的三态软门：

```go
// backend/internal/service/gateway_scheduling.go:1370-1377
switch schedulability {
case WindowCostSchedulable:    return true
case WindowCostStickyOnly:     return isSticky      // ← 软档：只放粘性会话
case WindowCostNotSchedulable: return false         // ← 硬档
}
```

配套字段 `window_cost_limit` + `window_cost_sticky_reserve`（`account.go:3009-3034`）。配软/硬两档水位即可实现方案三想要的临界平滑，**且不需要组级原子计数器**。

**改动落点从 23 处降到 3 处**
`filterAccountsBySchedulingThreshold` 的三个调用点全在同一个函数内，且 `groupID` 已在作用域：

- `gateway_scheduling.go:1018`（快照路径）
- `gateway_scheduling.go:1083`（mixed 回退路径）
- `gateway_scheduling.go:1118`（单平台回退路径）

粘性选号路径另补一处 `gateway_scheduling.go:1530-1535`（Grok free 软门已挂在该处，有现成写法）。终检形态抄 `gateway_profit_control.go:83` 的 `profitControlVetoLatest`。

**零新增查询、零新增 Redis 往返**
判定所需的 `account.Extra` 本来就随调度快照加载（`filterAccountsBySchedulingThreshold` 当前就在读同一份数据）。相比方案三“每请求一次组级原子计数 + 一次状态读取”，这条路线在请求路径上是纯内存比较。

**有成熟先例可抄**
ProfitControl 就是“分组级开关 → 只过滤候选账号，不改变既有排序/评分/粘性/熔断”（`group.go:126-133` 的注释即如此定义）。评审口径、灰度方式、回归范围、终检形态全部现成，实现与评审风险都显著低于新建状态机。

**降级行为已存在**
- 快照过期：`openAICodexSnapshotStaleForPause`（`account_scheduling_threshold_eval.go:274`）已处理
- 候选被过滤空：`diagnoseSelectionFailure`（`gateway_scheduling.go:2519`）已能给出可诊断的失败原因

不需要新建“保护暂停 / 恢复观察”状态机。

### 3.2 配置形态

`groups` 表新增两列（0 = 不启用）：

| 字段 | 含义 |
| --- | --- |
| `quota_headroom_soft_percent` | 账号 `used_percent` 超过该值后，本分组只允许粘性会话继续，不接纳新会话 |
| `quota_headroom_hard_percent` | 超过该值后，本分组完全不调度该账号 |

普通组不配置；高用量组配置示例 70 / 85。管理页复用现有分组配置表单，与 ProfitControl 同一区域。

### 3.3 实现上必须避开的坑

现有的 `RateLimitService.ApplyAccountSchedulingThreshold` 会**落库写 `TempUnschedulableUntil`**：

```go
// backend/internal/service/ratelimit_service.go:198-215
account.TempUnschedulableUntil = cloneTimePtr(decision.Until)
account.TempUnschedulableReason = reason
s.notifyAccountSchedulingBlocked(account, *decision.Until, "account_scheduling_threshold")
_ = s.accountRepo.SetTempUnschedulable(ctx, account.ID, *decision.Until, reason)
```

这是**全局停调**——所有分组一起失去该账号。核查确认该函数在生产代码中只有一个调用方（`ratelimit_service.go:175`，另有 Fable 变体 `:232`），其余命中均为 `.gocache` 构建产物与 `_testmain.go`。

**分组级水位门必须是纯只读候选过滤，绝不能复用这条写路径**，否则普通组会被自己的保底机制连带挡住。这正是原报告第四节所指出的“OpenAI 账号用量阈值会让所有使用该账号的分组同时失去该资源”。

其他需注意点：

- 该门只应作用于 token 算力请求，不应影响 models 列表、metadata、count_tokens 等旁路（参考 `withGatewayProfitControlGate` 用 `gatewayTokenRequestPricingAtFromContext` 做的入口收窄，`gateway_profit_control.go:14`）。
- 分组兜底（`FallbackGroupID` / `FallbackGroupIDOnInvalidRequest`，`group.go:81-83`）会改变生效分组，兜底后需按新分组重新取水位配置。
- 复合分组（`PlatformComposite`）的语义需单独确认，首期建议排除。

### 3.4 它不保证什么（需向业务侧说明）

- 它保的是“每个账号最后 (100 − X)% 只给普通组”，**不是“普通组一定够用”**。普通组需求超过 Σ(100 − X)% × 容量时仍须扩容。这一点对任何方案都成立。
- 上游 percent 快照存在延迟，高用量组会略微冲过软档。但超冲吃掉的是软硬档之间的缓冲带，**不会侵蚀硬档以下的普通组保底**。这比原方案对“在途消耗修正 D”的精度要求低一个量级，因为不需要建立 D 这个新账本。
- 粘性会话跨档会多消耗一些额度。要么接受，要么规定硬档不放粘性。
- 覆盖不了“某个模型的专属账号耗尽而其他模型有余”。与原方案相同，首期需限定可互换的模型范围。
- 依赖 `used_percent` 快照的新鲜度。快照整体不可信时，行为退化为“高用量组保守不调度”，与原方案的保护性暂停同向。

---

## 四 若决定仍走原方案，最少需补齐三项

1. **换 R 的数据源**：从 `actual_cost` 改为 `total_cost`，并把 `usage_group_daily_rollups` 加列、回填、水位失效重算单独列入工作量表。
2. **定义 E**：二选一——
   - 先建立 `$ / 1%` 标定（可行：`GetAccountWindowStats` 可取任意账号窗口内的 `StandardCost`，对 Δ`used_percent` 做回归），或
   - 明确承认本版只做“近似保护”，并从第 01 节评分表中撤下保障维度的具体分值。
3. **下移准入点**：从 `CheckBillingEligibility`（23 处）移到调度候选过滤（3 处）。

---

## 五 对文档本身的两点意见

**演示区建议加口径警示。** 第 02 节的数字算得准确、逐步可播放，但全部建立在“E 与 R 同单位”这一未验证前提上。汇报对象看到可交互仿真，会自然认为这套数已经可算。建议在演示区顶部直接标注“单位口径未确认，数值仅示意机制”。

**方案一被低估了。** 它是目前唯一不需要任何单位换算、可以立即 100% 落地的方案——账号绑定本身就是物理隔离，不涉及 percent ↔ $。其“资源效率 65 分”也非定数，通过调整绑定范围可以压低闲置率。若按“今天能否真正交付出保障”评分，方案一与第三节的水位门是仅有的两个候选。

---

## 六 建议推进顺序

| 步骤 | 内容 | 说明 |
| --- | --- | --- |
| 1 | 按方案一完成账号绑定保底（1–2 人日） | 立即获得确定性保障，且是后续任何方案的安全网 |
| 2 | 并行实现分组级窗口水位门 | 改动 3 处候选过滤 + 1 处粘性路径，无新增查询、无新状态机 |
| 3 | 水位门运行稳定后，回头评估是否缩减方案一的专用账号 | 用实际闲置率与拒绝率决策，而非预设比例 |
| 4 | 方案二 / 方案三 | 暂缓。其核心公式在目标号池上尚未定义，需先完成第四节第 1、2 项 |

---

## 附 核查位置索引

| 主题 | 位置 |
| --- | --- |
| `actual_cost` 含计费倍率 | `backend/internal/service/billing_service.go:1616` |
| 分组日桶聚合 `actual_cost` | `backend/internal/repository/custom_group_usage_rollup_repo.go:97, 256` |
| `usage_logs` 成本列定义 | `backend/migrations/001_init.sql:157-158` |
| 分组+时间复合索引 | `backend/migrations/062_add_scheduler_and_usage_composite_indexes_notx.sql` |
| OAuth 账号只有百分比窗口 | `backend/internal/service/account_scheduling_threshold_eval.go:250-282` |
| 账号美元额度仅 apikey/bedrock 递增 | `backend/internal/repository/usage_billing_repo.go:204` |
| 调度侧同样限定 apikey/bedrock | `backend/internal/service/gateway_scheduling.go:1313-1318` |
| 各平台均含 5h 窗口 | `backend/internal/service/account_scheduling_threshold_eval.go:187-190, 284-305, 355-367` |
| 5h 窗口费用阈值字段 | `backend/internal/service/account.go:3009-3034` |
| 计费准入入口（23 处调用） | `backend/internal/service/billing_cache_service.go:735` |
| 分组 RPM 按用户计数 | `backend/internal/service/billing_cache_service.go:788-855` |
| 候选过滤三处落点 | `backend/internal/service/gateway_scheduling.go:1018, 1083, 1118` |
| 粘性路径软门先例 | `backend/internal/service/gateway_scheduling.go:1530-1535` |
| 三态软门（可调度/仅粘性/不可调度） | `backend/internal/service/gateway_scheduling.go:1370-1377` |
| 分组级候选门先例（ProfitControl） | `backend/internal/service/gateway_profit_control.go:14, 83`；`group.go:126-133` |
| 全局停调写路径（勿复用） | `backend/internal/service/ratelimit_service.go:175, 198-215` |
| 快照过期保护 | `backend/internal/service/account_scheduling_threshold_eval.go:274` |
| 选号失败诊断 | `backend/internal/service/gateway_scheduling.go:2519` |
| 分组兜底字段 | `backend/internal/service/group.go:81-83` |

> 说明：全仓 grep 会扫入 `backend/.gocache/` 下的构建产物（含大量 `_testmain.go`），建议检索时限定 `backend/internal`。
