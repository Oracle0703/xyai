# 《用户订阅自助重置日额度方案》设计审核报告

被审文档：`docs/features/subscription-self-daily-reset-design-cn.md`（日期 2026-09-22，状态：设计稿，未实现）
审核日期：2026-09-22
代码基线：分支 `feature/hy/10208_sub_admin_assign_subscription`（基于 `main@d120d499e`）
审核方式：对设计稿中引用的**每一处现有实现**做源码核对，再基于核对结果评估设计结论。未执行迁移、未启动服务、未做真实重置。

---

## 0. 给审阅者的背景（自包含摘要）

被审设计要做的事：在用户端订阅卡片上增加一个「重置（N）」按钮，允许用户自助清空**某个订阅当天的日用量**，每天有限次数，次数上限按组织（由邮箱域名推导：`xunyou` / `wsdashi` / `other`）配置。

设计稿的关键选型：

- **计次单位**：每个订阅独立计次（`subscription_id` 为键）。稿中 §1 同时注明「共享计次（按 user_id）方案尚未确认改选」。
- **只重置日用量**，不动日限金额、周/月用量、余额、到期时间。
- **次数刷新**：按服务端配置时区（默认 `Asia/Shanghai`）的自然日，0 点刷新，不累积。
- **原子性**：日用量清零 + 次数扣减 + 幂等成功记录，必须在同一个 PostgreSQL 事务内提交，复用现有 `AtomicSuccess` 幂等机制。
- **数据模型**：新建小附属表 `subscription_self_daily_reset_usage`，主键 `subscription_id`，含 `quota_date` / `used_count` / `updated_at`，单行覆盖式，无需清表任务。
- **策略存储**：复用 `settings` 表，新 key `subscription_self_daily_reset_policy`，JSON 形如 `{"daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}`。
- **接口**：用户侧 `GET /api/v1/subscriptions/self-reset-status`、`POST /api/v1/subscriptions/:id/reset-daily`（强制 `Idempotency-Key`）；管理侧 `GET/PUT /api/v1/admin/subscriptions/self-reset-policy`（仅完整管理员）。

**审核总评**：方向正确、边界克制得当（不碰计费主链路、不加定时任务、不扩散核心字段、复用既有幂等事务）。设计稿对现有实现的引用**准确度很高**，经得起逐条核对。存在 **3 个阻断级问题**、**4 个需修正的设计决策**、**6 条改进建议**。其中 1 个阻断级问题（一次性日卡）本质是产品定价口径问题，建议先与需求方确认再动工。

---

## 1. 已核实为准确的引用

以下是设计稿中的事实性论断，逐条核对结果均为**属实**。列出以便审阅者信任其余推论的基础。

| # | 设计稿论断 | 核实位置 | 结论 |
| --- | --- | --- | --- |
| 1 | 现有用户幂等 helper 不使用 `AtomicSuccess`，且 coordinator 缺失时会直接执行 | `backend/internal/handler/idempotency_helper.go:24-33` | ✅ 属实 |
| 2 | `AtomicSuccess` 把业务执行包进事务，并支持提交后回调 | `backend/internal/service/idempotency.go:440-464`、`:116-128` | ✅ 属实 |
| 3 | repository 能透过 ctx 取到事务 client | `backend/internal/repository/error_translate.go:26-31`（`clientFromContext`：`if tx := dbent.TxFromContext(ctx); tx != nil { return tx.Client() }`） | ✅ 属实 |
| 4 | `ResetUsageWindows` 已走 `clientFromContext`，可在新事务中复用 | `backend/internal/repository/user_subscription_repo.go:551-565` | ✅ 属实 |
| 5 | `AdminResetQuota` 的缓存失效发生在方法内，不能直接包进新事务当原子操作 | `backend/internal/service/subscription_service.go:912-935`（`InvalidateSubCacheSync` + `billingCacheService.InvalidateSubscription` 在返回前同步调用） | ✅ 属实 |
| 6 | 日额度按配置时区自然日对齐，不是从点击时刻算 24 小时 | `backend/internal/service/user_subscription.go:117-129`（`automaticDailyWindowStartAt`）、`:187`（`timezone.StartOfDay(*s.DailyWindowStart).AddDate(0,0,1)`） | ✅ 属实 |
| 7 | 组织口径来自邮箱域名，大小写不敏感，子域名不匹配主域名 | `backend/internal/repository/organization_usage_repo.go:278-284`（`LOWER(SPLIT_PART(email,'@',2)) = 'xunyou.com'` …… `ELSE 'other'`） | ✅ 属实 |
| 8 | `settings` 普通读取直接用默认 client，不走事务 | `backend/internal/repository/setting_repo.go:20-33`（`r.client.Setting.Query()`，非 `clientFromContext`） | ✅ 属实 |
| 9 | 用户路由已有 JWT、面板限流、审计中间件 | `backend/internal/server/routes/user.go:20-26` | ✅ 属实（但漏了一个，见 §4.3） |
| 10 | 前端 store 缓存 60s、轮询 5 分钟 | `frontend/src/stores/subscriptions.ts:12`（`CACHE_TTL_MS = 60_000`）、`:95`（`5 * 60 * 1000`） | ✅ 属实 |
| 11 | 用户订阅页独立调用 `getMySubscriptions()`，不由 store 驱动 | `frontend/src/views/user/SubscriptionsView.vue:297` | ✅ 属实 |
| 12 | 迁移是追加型 SQL 文件、不可修改已应用迁移 | `backend/migrations/README.md`（SHA256 校验 + 不可变原则）、`backend/migrations/migrations.go`（`//go:embed *.sql`） | ✅ 属实 |

### 1.1 一条设计稿没写、但对该方案有利的发现

§1 要求「子管理员原有订阅重置权限不自动获得配置权限」。核对发现子管理员权限是**显式路由白名单**，不是路径前缀匹配：

```go
// backend/internal/service/admin_permission.go:33-48
var adminPermissionRouteRules = map[string][]adminRouteRule{
	AdminPermissionSubscriptions: {
		{"GET",  "/api/v1/admin/subscriptions"},
		{"POST", "/api/v1/admin/subscriptions/:id/reset-quota"},
		{"POST", "/api/v1/admin/subscriptions/reset-daily-filtered"},
		// ... 逐条枚举
	},
```

因此新增的 `GET/PUT /api/v1/admin/subscriptions/self-reset-policy` **默认就不会**被 `admin.subscriptions` 权限覆盖，该需求无需额外代码即成立。

> **建议**：正因为它是「默认成立」，反而容易被后来者顺手加进白名单破坏。必须补一条测试钉死「self-reset-policy 路由不在任何子管理员规则中」。

---

## 2. 阻断级问题（P0，实施前必须修改设计）

### P0-1　§4「一次性日卡可自助重置」是免费翻倍额度的漏洞

**设计稿原文（§4，第 74 行）**：

> 一次性日卡仍沿用原有"不自动补发日额度"的合同。若组织允许自助，它可在有效期内手动重置，但不延长到期时间；自助机会按自然日补给不等于日卡自动补发额度。

**问题**：这条把「不自动补发」当成了可以绕开的实现细节，但它其实是该商品的**定价合同**。

证据链：

```go
// backend/internal/service/user_subscription.go:70-75
func (s *UserSubscription) HasOneTimeDailyQuota() bool {
	if s == nil || s.StartsAt.IsZero() || s.ExpiresAt.IsZero() {
		return false
	}
	return !s.ExpiresAt.After(s.StartsAt.AddDate(0, 0, 1))   // 生命周期 ≤ 1 天
}

// backend/internal/service/user_subscription.go:117-129
func (s *UserSubscription) automaticDailyWindowStartAt(now time.Time) (time.Time, bool) {
	if s.DailyWindowStart == nil {
		return time.Time{}, false
	}
	if s.HasOneTimeDailyQuota() {          // ← 日卡被显式排除出自动刷新
		return time.Time{}, false
	}
	...
}
```

一次自助重置会执行 `ResetUsageWindows(id, true, false, false, StartOfDay(now), now)`，即 `daily_usage_usd = 0`。对于日卡：

1. 它本来在整个生命周期内**只有一份**日额度；
2. 重置后立刻获得**第二份完整日额度**；
3. 因为 `automaticDailyWindowStartAt` 对日卡返回 `false`，这个增发不会被任何自动逻辑拉回——是永久的。

管理端有这个能力没问题（管理员是被信任方，且是人工个案）。开放给用户等于把日卡单价打五折。

**放大因素**：策略粒度是**按组织**的，不是按分组 / 按商品。一旦为 `other`（即非 `xunyou.com` / `wsdashi.com` 的外部付费用户）配置为 1 次，**所有日卡买家自动获得翻倍额度**，没有任何单独开关可以只关这一类。

**建议**：

- v1 直接硬排除 `HasOneTimeDailyQuota()` 为真的订阅，返回 `disabled_reason: ONE_TIME_QUOTA`；
- §4 边界表、§5 策略表、§10 验收表同步修改（§10 现有「一次性日卡不改变到期或自动补额规则」这条描述不足以覆盖本问题）；
- 若确实要开放，必须是**独立于次数的单独开关且默认关闭**，不能隐含在组织次数配置里。

**性质**：产品定价口径问题，不是技术问题。**建议先与需求方确认再动工**。

---

### P0-2　`ObserveOnly` 默认为 true，空幂等键会绕过事务，产生半完成状态

**设计稿原文（§8 第 1 步）**：

> handler 验证认证、ID、JSON、日期格式及非空幂等键。即使全局 `idempotency.observe_only=true` 也必须有键。

设计稿只用一句话带过，但这句话是**承重的**，其重要性被严重低估。

证据：

```go
// backend/internal/service/idempotency.go:248-253
if key == "" {
    if opts.RequireKey && !c.cfg.ObserveOnly {   // ObserveOnly 默认 true → 这里不报错
        return nil, ErrIdempotencyKeyRequired
    }
    data, execErr := execute(ctx)                 // ← 直接裸跑，完全没有事务包裹
    ...
}

// backend/internal/service/idempotency.go:76-85
func DefaultIdempotencyConfig() IdempotencyConfig {
	return IdempotencyConfig{
		DefaultTTL:           24 * time.Hour,
		...
		ObserveOnly:          true, // 默认先观察再强制，避免老客户端立刻中断
	}
}
```

后果：只要 handler 的前置校验被漏写、被重构掉、或被某条新路径绕过，业务逻辑就会在**没有事务**的情况下执行「清零日用量 + 扣减次数」两步写入。这正好产出设计稿 §10 明令杜绝的「只扣次或只清零」半完成状态——**而且不会抛出任何错误，无声发生**。

`RequireKey` 这个参数在默认配置下是失效的，不能依赖它。

**建议**：

1. handler 前置校验保留（必要但不充分）；
2. **增加自证断言**：service 层在执行任何写入前检查 `if ent.TxFromContext(ctx) == nil { return ErrIdempotencyStoreUnavail }`。成本几行代码，把「必须在事务内」这个不变式在**写入点**钉死，而不是依赖调用链上游的纪律；
3. §10 验收增加定向用例：`observe_only=true` + 不带 `Idempotency-Key` 的 POST，必须返回 400 且**数据库无任何变化**。

---

### P0-3　`quota_date` 用 DATE 列并绑定 `time.Time` 有跨日错位风险

**设计稿原文（§6）**：

> | quota_date | date，非空 | 这行计数归属的服务端自然日 |

**问题**：数据库会话时区是可配置的，且测试环境用的是 UTC：

```go
// backend/internal/config/config.go:1612-1617
"host=%s port=%d user=%s dbname=%s sslmode=%s TimeZone=%s"

// backend/internal/repository/integration_harness_test.go:92
dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
```

当 Go 侧把 `2026-09-23 00:30:00 +08:00` 这个 `time.Time` 绑定到 `DATE` 列时，PostgreSQL 按**会话时区**做 `timestamptz → date` 转换。在 `TimeZone=UTC` 的会话下，该值落成 `2026-09-22`。

故障表现：每天 **00:00–08:00** 这个窗口内的自助重置被记到「昨天」，导致次数不刷新（用户次日凌晨用不了）或重复可用（取决于比较方向）。且**只在部分环境复现**，是典型的难排查缺陷。

**建议**：§6 明确增加约束条款：

- `quota_date` 一律绑定由 Go 计算的字符串：`timezone.StartOfDay(now).Format("2006-01-02")`；
- SQL 中**禁止**出现 `CURRENT_DATE`、`NOW()::date`、任何 `::date` 强转；
- §10 验收增加：在 `TimeZone=UTC` 的会话下跑一遍完整的跨日用例（23:59 → 00:30 → 08:30）。

---

## 3. 需要修正的设计决策（P1）

### P1-1　§8 第 4 步「持有写锁后再读组织上限」应当反过来

**设计稿原文（§8 第 3–4 步）**：

> 3. 在同一事务中按固定顺序锁定归属于该用户的订阅，再读取计数行；
> 4. 获得写锁后，以同一个 now 计算服务端日期、**组织上限**和有效日用量……

两个问题叠加：

**（a）阻塞计费落账。** 计费路径对同一行做盲写：

```go
// backend/internal/repository/user_subscription_repo.go:718-734
func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	const updateSQL = `
		UPDATE user_subscriptions us
		SET daily_usage_usd = us.daily_usage_usd + $1, ...
		WHERE us.id = $2 ...`
```

`SELECT ... FOR UPDATE` 会阻塞它，且持锁区间要跨过 `MarkSucceeded`（`idempotency.go:463`，在事务内）。考虑到重置频次极低（每订阅每天 1 次），实际争用可忽略——**但没有理由把锁持得比必要更久**。

**（b）连接池自锁（更实在的风险）。** `SettingRepository.Get` 用的是默认 client（`setting_repo.go:20`），**在持有行锁期间会再向连接池申请第二条连接**。这是典型的池耗尽死锁模式：并发数接近池上限时，所有连接都在持锁等待第二条连接，系统从「慢」直接变成「死」。

设计稿 §3 已经注意到「新重置事务中的权威读取应明确走事务 executor」，但给的理由只有快照一致性。真正更重要的理由是避免二次取连接。

**建议**：

- 策略读取移到**取锁之前**（甚至开事务之前）；锁内只做日期、有效日用量、次数余额和归属状态复核；
- §3 那一行的理由补上「避免持锁期间二次占用连接池」；
- 若坚持在事务内读，必须提供事务感知的 settings 读取路径，不能复用 `SettingRepository.Get`。

---

### P1-2　组织解析在 Go 侧不存在，需新建且要防两侧漂移

设计稿假定可以「复用当前 main 的组织口径」，但核对发现：

- **只有 SQL 版本**：`organization_usage_repo.go:278-284` 的 CASE 表达式；
- Go 侧只有**反向**映射（org → domain）：`user_subscription_repo.go:445-454`；
- 且现有常量里**根本没有 `other`**：

```go
// backend/internal/service/subscription_admin_filter.go:10-13
const (
	SubscriptionOrganizationXunyou  = "xunyou"
	SubscriptionOrganizationWsdashi = "wsdashi"
)   // ← 没有 Other

// :44  管理端筛选也只允许这两个值
if !subscriptionAdminFilterValueAllowed(filter.Organization, "", SubscriptionOrganizationXunyou, SubscriptionOrganizationWsdashi) {
```

而设计稿的策略 JSON 要求三个组织码**必须完整**，包括 `other`。

另有一处真实陷阱：`SPLIT_PART(email, '@', 2)` 取的是第 2 段。若 Go 侧用 `strings.LastIndex(email, "@")` 实现，在含多个 `@` 的地址上会与 SQL 得出**不同**结果，导致同一用户在「用量统计」和「自助次数」两处被归入不同组织。

**建议**：

- 新增单一实现 `service.ResolveSubscriptionOrganization(email string) string`，加上 `SubscriptionOrganizationOther` 常量；
- 补一条 parity 测试，断言 Go 实现与 SQL CASE 在以下用例上一致：大小写混合、子域名 `a@team.xunyou.com`（应为 `other`）、多 `@`、空域名、`@` 结尾；
- §3、§5 补充说明这不是「复用」而是「新建 + 对齐」。

---

### P1-3　计数表主键锁死了计次身份，而该选型在文档内尚未定稿

文档内部存在一处未闭合的风险：

- §1 写：「用户已选择"每个订阅分别计算次数"；**后续询问共享方案的实现速度和利弊，尚未确认改选**」；
- §6 却把主键定死为 `subscription_id`，并自己承认：「计数身份必须在实施前定稿，**不能上线后直接改键而忽略当天历史**」。

即：文档同时承认「还没定」和「定了就不能改」。一旦按现稿写完 migration 再改共享模式，等于重建表 + 折算当天历史。

**建议（低成本对冲）**：现在就把 `user_id BIGINT NOT NULL` 作为冗余列写进表结构（独立计次模式下不参与主键，只加普通索引）。这样将来若改共享模式，变更范围是「加一个唯一索引 + 换读写路径」，而不是「重建表 + 数据迁移」。多一列的成本约等于零。

同时建议在 §6 注明：该冗余列在独立计次模式下不参与任何唯一性约束，仅为未来模式切换预留。

---

### P1-4　幂等记录 TTL 默认 24 小时，要 48 小时必须显式传参

**设计稿原文（§8 第 2 步）**：「幂等记录保留期建议至少 48 小时。」

但默认值是 24 小时：

```go
// backend/internal/service/idempotency.go:78
DefaultTTL: 24 * time.Hour,
```

若不在 `IdempotencyExecuteOptions.TTL` 显式赋值，默认值会静默生效，与设计意图不符。

**建议**：§8 明确写出「必须显式传 `TTL: 48 * time.Hour`，不依赖 `DefaultIdempotencyConfig` 默认值」。

---

## 4. 改进建议（P2）

### 4.1　`disabled_reason` 枚举未定义完整

§7 的示例只给了 `DAILY_LIMIT_REACHED` 一个值，但 §4 的边界表列出了至少 6 种禁用情形。前端要按它做文案映射，漏一个就是空白提示。

**建议**在 §7 列成封闭枚举：

```
POLICY_DISABLED        组织上限配置为 0
NO_DAILY_LIMIT         分组无日限
NO_USAGE               当前有效日用量为 0
DAILY_LIMIT_REACHED    当天次数已用尽
SUBSCRIPTION_INACTIVE  过期 / 暂停 / 撤销
GROUP_DISABLED         分组停用
ONE_TIME_QUOTA         一次性日卡（若采纳 P0-1）
```

### 4.2　前端刷新语义要写准，且有两个端点

设计稿 §3、§9 说「成功后需同步使 store 失效并刷新」，但：

```ts
// frontend/src/stores/subscriptions.ts:124-126
function invalidateCache() {
  lastFetchedAt.value = null      // ← 只清时间戳，不发请求
}
```

`invalidateCache()` 单独调用**不会刷新数据**，必须再调 `fetchActiveSubscriptions(true)`。

更重要的是：**store 与页面用的是两个不同端点**——store 调 `getActiveSubscriptions()`（`subscriptions.ts:60`），页面调 `getMySubscriptions()`（`SubscriptionsView.vue:297`）。重置成功后两处都要刷新，否则会出现「卡片归零了、但全局余额提示还是旧值」。

**建议**：§9「前端用户页」那一行展开为两条具体动作，§10 前端同步验收增加「两个端点都刷新」的断言。

### 4.3　§3 漏了一个用户路由中间件

```go
// backend/internal/server/routes/user.go:20-26
authenticated.Use(gin.HandlerFunc(jwtAuth))
authenticated.Use(middleware.BackendModeUserGuard(settingService))   // ← 设计稿未提及
authenticated.Use(panelRateLimiter.Global())
authenticated.Use(gin.HandlerFunc(auditLog))
```

`BackendModeUserGuard` 在后端模式关闭时会拦截新接口。前端需要能区分这类拒绝与 `SELF_RESET_DISABLED`，否则用户会看到误导性的「组织未开放自助重置」。

### 4.4　`can_reset` 应由服务端单一判定，避免两个快照打架

§4 的「当前有效日用量已经为 0 → 按钮禁用」若由前端拿订阅列表里的 `daily_usage_usd` 自行推断，就和 status 接口的 `can_reset` 成了**两个时间点的快照**，必然出现不一致（列表说可点、点下去服务端 409）。

核对确认列表接口返回的已是「有效」用量——`ListUserSubscriptions` 调用了 `normalizeExpiredWindows`（`subscription_service.go:784-792`），会把过期日窗口的用量显示清零，与服务端判定口径一致。所以数据本身没问题，问题只在于**两次请求的时间差**。

**建议**：`can_reset` 由服务端一次算全（含 `有效日用量 > 0` 判定），前端只认 `can_reset` + `disabled_reason`，列表里的 `daily_usage_usd` 仅作展示，不参与按钮可用性推断。

### 4.5　`next_reset_at` 的计算方式要写死

建议在 §7 明确：`next_reset_at = timezone.StartOfDay(now).AddDate(0, 0, 1)`，**不要**用 `now.Add(24 * time.Hour)`。当前配置 `Asia/Shanghai` 无 DST 不会出事，但配置项是开放的（`timezone.Init` 接受任意时区名，`backend/internal/pkg/timezone/timezone.go:22-40`），换成有夏令时的时区就会错位一小时。

### 4.6　按钮位置建议调整

设计稿 §1 说放在「续费按钮后」。核对该按钮的实际位置：

```vue
<!-- frontend/src/views/user/SubscriptionsView.vue:60-80 -->
<div class="flex items-center gap-2">
  <span :class="[...状态徽章...]">{{ t(`userSubscriptions.status.${subscription.status}`) }}</span>
  <button v-if="subscription.status === 'active'" ...>{{ t('payment.renewNow') }}</button>
</div>
```

它在**卡片头部右上角**，与状态徽章同排。塞进第三个元素后窄屏必然拥挤（§10 已经预见到「窄屏按钮布局正常」这条验收）。

**建议改放到「日用量」进度条那一行的右侧**（`SubscriptionsView.vue:104-138`）：

- 位置即语义——重置的是日额度，放在日额度旁边不需要额外解释；
- 天然解决「无日限时不显示」——那一整块本来就有 `v-if="subscription.group?.daily_limit_usd"`；
- 头部两元素布局不变，窄屏风险消失。

---

## 5. 可以精简的部分

### 5.1　§2 的九行对比表

在「已选定独立计次」之后，这张表属于决策记录而非设计内容，压成一段话即可。

但请注意：**计次身份必须在写 migration 前拍板**，不要带着「尚未确认改选」往下走（见 P1-3 的对冲方案）。

### 5.2　§6「无需凌晨批量清表」是真优点，但有隐性代价

单行覆盖式设计（主键 `subscription_id`，靠 `quota_date == 今天 ? used_count : 0` 公式判定）确实避免了调度任务和表膨胀，这个取舍我支持。

但代价是**没有次数历史**。审计中间件记录的是请求（`user.go:26`），不是「昨天全组织自助重置了多少次」这类聚合。而 §10 有一条「多设备并发：同一计数身份并发提交不同键，成功次数不超过组织上限」的验收——出问题时靠什么观测定位，文档里没给答案。

**建议**：增加一个 metrics counter（不建表、不加调度），维度为组织 + 结果，供 §10 验收和线上排查使用。

---

## 6. 需要补充但设计稿未覆盖的实现约束

以下几条在实施时容易出错，建议写进文档：

1. **计数行的并发创建**：§6 说「主键满足查询和并发需要」。这个结论**只在「先锁订阅行、再操作计数行」的前提下成立**（§8 第 3 步的顺序）——因为不存在的行无法被锁。计数行写入必须用 `INSERT ... ON CONFLICT (subscription_id) DO UPDATE`，不能是先 SELECT 再 INSERT。
2. **路由注册顺序**：用户侧 `/subscriptions` 分组现有路由全是静态段（`""`、`/active`、`/progress`、`/summary`，见 `user.go:130-135`）。新增 `POST /:id/reset-daily` 会在同一层级引入参数段。gin 支持静态与参数兄弟节点共存（管理端 `/admin/subscriptions/:id` 与 `/admin/subscriptions/search-groups` 已并存），但新增时要确认 `self-reset-status` 这个静态段排在 `:id` 之前注册，避免被当成 ID 解析。
3. **幂等指纹必须显式包含订阅 ID**：`executeUserIdempotentJSON` 用的 `c.FullPath()` 是路由**模式**（`/api/v1/subscriptions/:id/reset-daily`），不含具体 ID。设计稿 §7 已经要求 payload 含订阅 ID —— 这条不能省，否则同一把 key 用在不同订阅上会被错误匹配。
4. **策略不要接入 `SettingService` 的进程内缓存**：`SettingService` 对若干特定 key 有 `atomic.Value` 缓存（如 `panelRateLimitCache`、`codexRestrictionPolicyCache`，见 `backend/internal/service/setting_service.go:124-155`）。设计稿 §5 要求「每次读取数据库当前策略」，因此新 key 必须走裸 `settingRepo.GetValue`，**不要**照搬那批缓存字段的写法。

---

## 7. 结论与优先级

| 优先级 | 条目 | 性质 | 建议动作 |
| --- | --- | --- | --- |
| **P0-1** | 一次性日卡自助重置 = 额度翻倍 | 产品定价口径 | **先与需求方确认**；建议 v1 硬排除 |
| **P0-2** | `ObserveOnly=true` 时空幂等键绕过事务 | 正确性 | 加 service 层事务断言 + 定向测试 |
| **P0-3** | `quota_date` DATE 绑定时区错位 | 正确性 | 绑定字符串；禁用 SQL 侧日期函数；UTC 会话验收 |
| **P1-1** | 持锁期间读策略（阻塞计费 + 池自锁） | 可靠性 | 策略读取移到取锁之前 |
| **P1-2** | Go 侧缺组织解析函数，且无 `other` 常量 | 一致性 | 新建单一实现 + 与 SQL 的 parity 测试 |
| **P1-3** | 计数主键锁死计次身份，选型未定稿 | 演进成本 | 现在加 `user_id` 冗余列对冲 |
| **P1-4** | 幂等 TTL 默认 24h ≠ 设计要求的 48h | 一致性 | 显式传 TTL |
| P2 | 6 条（枚举 / 前端双端点 / 中间件 / can_reset 单一判定 / next_reset_at / 按钮位置） | 完整性与体验 | 补进文档 |

**总体判断**：这是一份质量高于平均水平的设计稿——对现有实现的引用逐条核对全部属实，边界控制（不碰计费主链路、不加定时任务、不扩散核心字段、复用既有幂等事务）判断正确。

P0-2 与 P0-3 是实现约束，补进文档并增加对应测试即可闭环。P0-1 需要产品侧决策。P1 的四条改完之后，本稿可直接作为实施依据。

---

## 附：审核未覆盖的范围

- 未执行数据库迁移、未启动服务、未做真实重置操作；
- 未评审尚未合入本分支的部门功能对组织口径的影响（设计稿 §3 已声明不依赖它）；
- 未对前端 i18n 文案、具体 Vue 组件拆分方式做评审；
- 性能结论（如 P1-1 中的锁争用）基于代码路径推断，未做压测验证。
