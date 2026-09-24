# 用户订阅自助日重置交付记录

日期：2026-09-23。分支：`feature/hy/10208_sub_admin_assign_subscription`，基于 `main@d120d499e`。状态：本地实现，未提交、未推送、未部署；上一项子管理员分配订阅改动保留。

## 已实现行为

| 项目 | 结果 |
| --- | --- |
| 用户按钮 | 续费后增加“重置（N）”；确认后只清当前订阅日用量，成功消耗 1 次，用完显示 0 并置灰 |
| 次数 | 每订阅独立；按服务端配置时区自然日恢复，昨天次数不累积，无定时任务 |
| 组织设置 | 管理订阅页增加完整管理员专用弹窗；迅游/速宝/其他默认 1，支持 0–100 整数，0 关闭；同日修改按已用次数重算 |
| 灰度开关 | `rollout=admin` 默认仅完整管理员；`off` 全部关闭；`all` 全员开放。旧配置缺少字段时按 `admin` 处理 |
| 特殊情况 | 一次性日卡、无日限、零有效日用量、失效订阅和停用分组拒绝自助；十年订阅不按剩余时间误判为日卡 |
| 管理员重置 | 原管理接口不限自助次数，也不扣减或补回用户次数；管理员走用户入口仍然计次 |
| 权限 | 锁查询同时限定 user_id 和 subscription_id，非本人/不存在统一 404；策略不向子管理员开放 |
| 可靠性 | 非空幂等键＋48h TTL，次数/日用量/成功结果同事务；失败回滚，提交后本机、billing cache、跨实例通知；未知结果按订阅保留原键原日期（内存＋按用户隔离的 sessionStorage），关闭弹窗或刷新页面后再次确认仍沿用原键 |

## 主要实现位置

| 部分 | 路径 |
| --- | --- |
| migration | `backend/migrations/241_subscription_self_daily_reset.sql`：附属计数表与独立设置项；避开部门分支已用 239/240 |
| 后端 | `backend/internal/service/subscription_self_reset.go`、`backend/internal/repository/subscription_self_reset_repo.go`、`backend/internal/handler/subscription_self_reset_handler.go` |
| API | 用户 `GET /api/v1/subscriptions/self-reset-status`、`POST /api/v1/subscriptions/:id/reset-daily`；管理员 `GET/PUT /api/v1/admin/subscriptions/self-reset-policy` |
| 前端 | `frontend/src/composables/useSubscriptionSelfReset.ts`、`frontend/src/api/subscriptionSelfReset.ts`、用户/管理订阅页面、`SubscriptionSelfResetPolicyDialog.vue`、中英文 locale |
| 接线保护 | `service/wire.go#ProvidePluginManager` 保留插件 SetAccountDirectory 接线，原独立 provider 文件已并回 Wire 源定义，重复生成结果稳定 |

## 验证

| 验证 | 结果与实际覆盖 |
| --- | --- |
| 前端专项 | 初版 9 文件/47 项通过；本轮精简后 9 文件/53 项通过，随后增加离页后全局状态刷新场景，受影响 2 文件/16 项再次通过。覆盖用户按钮/日卡、未知结果跨挂载重试、重复点击、Retry-After、日期变化、旧响应抛弃、完整刷新重试、午夜刷新、策略弹窗及既有管理订阅和 locale/API 回归 |
| 灰度专项 | 新增 rollout `off/admin/all` 解析与管理员/普通用户判定；线上先保存 `admin`，验证后改为 `all` |
| Go 专项 | service、handler、middleware、routes 通过；策略 JSON、日卡/长期订阅、升降限、零用量、上海/UTC/夏令时、空键/无事务、权限撤销和白名单边界 |
| PostgreSQL 集成 | 3 项集成测试通过：新建隔离数据库并应用仓库 migration；8 个不同键并发只成功一次、单连接池、同键重放、48h TTL、错误回滚、成功记录写入失败回滚、真实提交后模拟确认丢失的只读恢复、跨日懒计数、管理员不扣次、独立订阅、组织分类与 DATE 字符串一致性 |
| 缓存回读 | 两个独立 SubscriptionService/BillingCacheService 实例，使用真实 PostgreSQL＋miniredis TCP/PubSub；确认旧 L1 已预热、共享 billing cache 已填充，再重置并验证两个实例均回读 0，而非只统计发布函数调用 |
| Wire/编译 | 重复 `go generate ./cmd/server` 结果稳定；`go test ./cmd/server -run Wire` 编译通过（此筛选无运行测试） |
| 前端静态检查 | typecheck、修改文件 ESLint 通过 |
| 后端静态检查 | golangci-lint 2.13.0 / Go 1.27，受影响 service/repository/handler/server/cmd 包 `--new-from-rev=HEAD`：0 issues |
| 文档与差异 | `git diff --check`、wiki LF 检查通过；无 go.mod/go.sum 依赖变化 |
| 页面观察 | 实际用户 Vue 页面＋隔离模拟 API：桌面、375px 窄屏中文、无空格长名称英文；确认弹窗、成功后日用量 0、重置（0）禁用及弹窗关闭均验证 |

复跑命令见 `llm-wiki/wiki/ops.md` 的“自助日重置专项验证”。Go 使用仓库缓存与 fresh GOTMPDIR，集成只连接本轮临时 PostgreSQL。

## 2026-09-23 自审后的简化

| 定位 | 本轮处理 |
| --- | --- |
| 两套刷新入口造成部分重试漏刷新 | 合并为单一 refresh；初次进入、手动重试、成功回读和跨日都刷新同一组数据，删去页面的重复加载入口及 catch 后原样 throw |
| loading 和 outcomeUncertain 与现有状态重复 | 删除两项状态；status 为空表示不可操作，操作键存在表示已经尝试提交。操作键推迟到首次确认提交时生成，取消未提交确认不保存操作 |
| HTTP 环境直接调用 randomUUID 失败 | 使用 getRandomValues 生成 128 位随机操作键，不新增 UUID 依赖或多层兜底 |
| 结果未知后重试被 429 限流会丢原键 | 401/408/429 保留原键原日期，遵守 Retry-After；补“未知结果→限流→再重试”用例 |
| 重复列举状态和错误文案 | 通用 SELF_RESET 前缀去除后复用现有枚举文案，只保留三项名称不同的别名；未知错误保留原失败提示 |
| 手工关联拼装和零散接线文件 | Ent 在锁父行后分开加载用户/分组，删去两段手工关系查询；插件 provider 并回已有 wire.go，删除独立文件 |

生产代码较本轮前净减少 31 行、删除 1 个文件；不通过删测试或压缩格式计数。保留归属 WHERE、FOR UPDATE、事务存在断言、原子扣次、幂等键持久化、过期响应保护和提交后缓存失效。

本轮复验：Go service/handler/routes 专项、server 编译、隔离 PostgreSQL 3 项集成、Wire 重生成一致性、ESLint、类型检查及 golangci-lint（0 issues）通过。新增覆盖未提交取消、HTTP 键生成、401/408/429 保留原键、读取失败后完整重试和提交后离页的全局状态刷新；临时数据库已停止。

## 上线与 PostgreSQL 验证

当前代码的 PostgreSQL 集成测试没有实际执行过（上文自审记录里的集成结果无法确认，也不覆盖之后的改动），线上会是这些 SQL 第一次在真实 PostgreSQL 上运行。按下面顺序做，任何一步不符合预期，就在「自助重置设置」里把开放范围改成「关闭」，立即生效，不需要回滚表或数据。

### 发布前（推荐）：在有 Docker 的机器上跑一次集成测试

```bash
cd backend
go test -tags=integration -p 1 -count=1 ./internal/repository -run SubscriptionSelfReset -v
```

testcontainers 会自动启动 `postgres:18-alpine` 和 Redis，跑完销毁，不碰业务库。**输出里必须有 `TestSubscriptionSelfResetIntegration`、`TestSubscriptionSelfResetCrossInstanceCaches`、`TestSubscriptionSelfResetOrganizationParityAndDates` 三个 `--- PASS`。** 没有 Docker 时测试会打印 `docker is not available; skipping integration tests` 并返回 `ok`，这不算通过。

机器上没有 Go 1.27 时，可以用临时容器（Linux）：

```bash
docker run -d --name sr-pg -e POSTGRES_PASSWORD=pg -p 127.0.0.1:55432:5432 postgres:18-alpine
docker run --rm --network host -v "$PWD":/src -w /src/backend golang:1.27 \
  env SUB2API_POSTGRES_ONLY_INTEGRATION_DSN="host=127.0.0.1 port=55432 user=postgres password=pg dbname=postgres sslmode=disable" \
  go test -tags=integration -p 1 -count=1 ./internal/repository -run SubscriptionSelfReset -v
docker rm -f sr-pg
```

这个 DSN 模式会对目标库执行全部迁移，只能指向临时库。

### 上线后第 1 步：迁移和配置（只读 SQL）

```sql
SELECT filename, applied_at FROM schema_migrations WHERE filename = '241_subscription_self_daily_reset.sql';

SELECT column_name, data_type, is_nullable FROM information_schema.columns
 WHERE table_name = 'subscription_self_daily_reset_usage' ORDER BY ordinal_position;

SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint
 WHERE conrelid = 'subscription_self_daily_reset_usage'::regclass;

SELECT value FROM settings WHERE key = 'subscription_self_daily_reset_policy';
```

期望：迁移有 1 行；4 列依次为 `subscription_id bigint`、`quota_date date`、`used_count integer`、`updated_at timestamptz`；约束有主键、`REFERENCES user_subscriptions(id) ON DELETE CASCADE` 和 `used_count >= 0`；配置为 `{"rollout":"admin","daily_limit_by_organization":{"xunyou":1,"wsdashi":1,"other":1}}`。

### 第 2 步：读取接口

- 完整管理员打开「订阅管理 → 自助重置设置」，能看到开放范围「仅管理员」和三项次数。
- 完整管理员打开「我的订阅」，有效订阅上出现「重置（N）」；普通用户账号看不到按钮和提示。
- 服务端日志没有 `SELF_RESET_UNAVAILABLE`。

这一步说明状态查询（订阅列表、计数表 JOIN、`to_char(quota_date, 'YYYY-MM-DD')`）在真实库上能执行。

### 第 3 步：管理员真实重置一次

准备一个完整管理员账号：有有效订阅，分组设了日限额，当天已有用量（先用该账号的 API Key 调一次模型），邮箱所属组织的次数不为 0。记下订阅 ID `:sid` 和用户 ID `:uid`。

```sql
-- 重置前记录
SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd, daily_window_start, expires_at
  FROM user_subscriptions WHERE id = :sid;
```

在页面上点重置并确认，然后：

```sql
-- quota_date 为服务端配置时区的今天，used_count = 1
SELECT subscription_id, quota_date, used_count, updated_at
  FROM subscription_self_daily_reset_usage WHERE subscription_id = :sid;

-- 日用量为 0，daily_window_start 为今天 00:00（服务端配置时区）；周/月用量和到期时间与重置前一致
SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd, daily_window_start, expires_at
  FROM user_subscriptions WHERE id = :sid;

-- 最新一条为 succeeded、200，有效期 48 小时
SELECT status, response_status, error_reason, expires_at - created_at AS ttl
  FROM idempotency_records WHERE scope = 'subscriptions.self_reset.user:' || :uid
 ORDER BY created_at DESC LIMIT 5;
```

服务端时区是 Asia/Shanghai 时，北京时间 0–8 点之间做这一步，可以顺带确认 `quota_date` 是北京日期而不是 UTC 日期。这一步说明扣次 UPSERT（`$2::date`）、日用量清零和幂等记录在同一事务里提交。

### 第 4 步：缓存失效

重置前让该订阅的日用量达到限额，此时用这个账号的 API Key 调模型会被日额度拦截；重置后立刻再调，应该直接成功，不用等缓存过期。多实例部署时连续调几次（会落到不同实例），都应成功。页面顶部的订阅摘要日用量也应变为 0。

### 第 5 步：重放和并发（验证行锁）

从浏览器开发者工具复制刚才那次 POST 的 `Authorization` 和 `Idempotency-Key`，原样重发：

```bash
curl -i -X POST "$BASE/api/v1/subscriptions/$SID/reset-daily" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY" -d "{\"quota_date\":\"$DATE\"}"
```

期望 200、响应头 `X-Idempotency-Replayed: true`，计数表 `used_count` 仍为 1。

并发：把管理员所在组织的次数临时改为 2，用该账号 API Key 再产生一点用量，然后同时发 5 个不同键的请求：

```bash
for i in 1 2 3 4 5; do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST "$BASE/api/v1/subscriptions/$SID/reset-daily" \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    -H "Idempotency-Key: conc-$i-$(date +%s)" -d "{\"quota_date\":\"$DATE\"}" &
done; wait
```

期望恰好 1 个 200、4 个 409（`SELF_RESET_LIMIT_REACHED`），计数表 `used_count = 2`。做完把次数改回。这一步说明 `FOR UPDATE` 在真实库上把并发请求串行化了。

### 第 6 步：开关，然后放开

- 改为「关闭」：管理员页面的按钮消失，直接调 POST 返回 403 `SELF_RESET_ROLLOUT_DISABLED`。再改回「仅管理员」。
- 以上都通过后改为「全部用户」。

### 放开后的巡检

```sql
-- 每天使用情况
SELECT quota_date, count(*) AS subscriptions, sum(used_count) AS resets
  FROM subscription_self_daily_reset_usage GROUP BY quota_date ORDER BY quota_date DESC LIMIT 7;

-- 超过上限的计数：各组织上限都是 1 时应无结果；上限改过就把 1 换成最大上限
SELECT * FROM subscription_self_daily_reset_usage
 WHERE quota_date >= CURRENT_DATE - 1 AND used_count > 1;

-- 近一天的失败原因：NO_USAGE、LIMIT_REACHED 属正常；出现 SELF_RESET_UNAVAILABLE 要排查
SELECT status, error_reason, count(*) FROM idempotency_records
 WHERE scope LIKE 'subscriptions.self_reset.%' AND created_at > NOW() - INTERVAL '1 day'
 GROUP BY 1, 2;
```

## 2026-09-23 审核后修复

| 审核问题 | 处理 |
| --- | --- |
| 列表加载失败时页面显示“暂无有效订阅”，横幅又误报“自助重置状态暂不可用” | 恢复 `loadSubscriptions` 的 `failedToLoad` 提示。refresh 只在状态请求失败时报状态不可用，列表失败不再丢弃状态 |
| refresh 先清空状态：切回标签页时按钮会闪成“—”，可重试失败后卡片一直停在“重置（—）” | 刷新和提交期间保留上一份状态；关闭已提交过的弹窗后重新读取状态。只有加载失败时才显示“状态暂不可用” |
| 切回标签页一次发 3 个请求，含强制刷新全局 store | 全局 store 只在重置成功后强制刷新 |
| 结果未知的操作用全局待确认横幅锁住所有订阅，存储写入失败就拒绝提交 | 横幅和全局锁删除，存储失败时退回内存、照常提交。原键仍按订阅保留，见下一节二轮复核 |
| 策略读取失败时，弹窗无法保存覆盖 | 读取失败时显示空白输入框，不填默认值；三项都填好后可保存覆盖，也可重试读取 |
| 分组软删除后，状态接口显示 GROUP_DISABLED，POST 却返回 404 | 锁查询不再因为分组为空返回 404，两边统一为 `GROUP_DISABLED`（POST 返回 409 `SELF_RESET_GROUP_DISABLED`） |
| 事务存在性断言在 service 和 repo 里重复了三处 | 只保留 `Reset` 入口一处 |
| Scope 按用户隔离没有测试锁住 | Scope 抽成 `service.SubscriptionSelfResetIdempotencyScope`，handler 和集成测试共用。集成测试新增：另一用户使用同一个键能成功执行，不会回放 |
| 组织按邮箱域名判定，而注册邮箱验证可以关闭 | 不改代码。写入 `llm-wiki/wiki/security-and-reliability.md` 的运维约束 |
| `ProvidePluginManager` 与上游手改的 `wire_gen.go` 分叉 | 写入 `docs/features/sub2api -merage-list.md` 的合并检查项 |

复验结果：

- 前端：自助重置 3 个专项文件共 23 项通过；管理订阅 3 个文件和 i18n 11 个文件共 83 项通过；vue-tsc、ESLint 通过。
- Go：service、handler、middleware、routes 专项通过，`go vet -tags=integration ./internal/repository` 通过。
- **PostgreSQL 集成测试未重跑**（本机没有可用的 PostgreSQL）。新增的跨用户同键断言和软删除分组断言只通过了编译，还没有实际执行，合入前需要在隔离库上跑一次。

### 2026-09-24 二轮复核修复

| 问题 | 处理 |
| --- | --- |
| P1：关闭结果未知的弹窗会删掉原键。组织上限 ≥2 时，原请求其实已提交、之后又有新用量，再次确认会生成新键，重复清零并多扣一次。上一轮认为 NO_USAGE 能兜住，但前提是中间没有新用量；来点重置的用户正在大量使用，这个前提通常不成立。默认 1/1/1 时第二次会得到 LIMIT_REACHED，不受影响 | 按订阅保留未确认的键（内存＋按用户隔离的 sessionStorage，写入失败退回内存）。关闭弹窗、刷新页面都不删除；只在成功或确定性失败时删除；日期变化后作废。再次打开同一订阅时沿用原键，并提示“上一次重置结果尚未确认”。其他订阅不受影响 |
| P2：重置成功后全局订阅状态刷新失败被静默忽略 | 刷新失败时提示“重置成功，状态刷新失败”，下一次 refresh（切回标签页、次日、手动重试）补刷全局状态，成功后不再重复刷新。App 本身每 5 分钟强制轮询，所以之前最多旧 5 分钟 |

复验：自助重置 composable 21 项（含 Codex 场景：上限 3、原请求已提交、新用量入账、关闭后再次确认沿用原键）、用户页 4 项、策略弹窗 3 项、管理订阅 11 项、i18n 11 项通过；vue-tsc、ESLint 通过。本轮只改了前端，后端和 PostgreSQL 集成测试同样未重跑。

### 2026-09-24 灰度开关复核修复

| 问题 | 处理 |
| --- | --- |
| 「仅管理员」阶段，普通用户仍能看到置灰的「重置（1）」和「暂未对当前账号开放」提示 | 原因为 `ROLLOUT_DISABLED`，或状态还没加载成功时，不显示按钮和提示行。状态读取失败只显示可重试的错误横幅 |
| 页面打开期间开放范围被收回，提示成「组织未开放自助重置」 | 单独错误码 `SELF_RESET_ROLLOUT_DISABLED`，提示「自助重置暂未对当前账号开放」 |
| 灰度只测了判断函数 | 新增服务层测试：「仅管理员」阶段普通用户的状态全部为 `ROLLOUT_DISABLED`；重置返回 403，且不加锁、不扣次 |
| 收尾 | 类型补 `ROLLOUT_DISABLED`；删除未使用的文案；去掉测试 mock 里状态接口并不存在的 `rollout` 字段；gofmt 和缩进 |

复验：Go service、handler、handler/admin、middleware、routes 专项通过，`go vet -tags=integration` 通过；前端 6 个文件 50 项通过，vue-tsc、ESLint 通过。PostgreSQL 集成测试仍未执行，上线步骤见「上线与 PostgreSQL 验证」。

## 发布与验证边界

- 业务数据库未迁移，运行中的业务服务未重启；发布时需要正常应用 migration 241 并更新后端及前端，不能只部署按钮。
- 浏览器测试使用模拟 API，数据库测试使用隔离库；未通过真实登录账号提交重置，也未修改生产配置。
- 两个缓存实例在同一个测试进程中，Redis 服务为 miniredis；这验证实际缓存读写/PubSub 路径，不代表生产 Redis 集群、网络断连或多进程部署验收。通知失败仍由既有 TTL 最终收敛。
- 本轮执行与改动相关的专项检查，未跑全仓测试或生产前端构建。wiki 已同步实现约束，知识图谱未重建。
- 本轮创建的隔离页面预览和临时 PostgreSQL 均已停止，未停止或重启用户已有服务。
