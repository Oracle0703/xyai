# 技术债看板（深度扫描版）

| 字段 | 值 |
| --- | --- |
| 创建 | 2026-08-12 |
| 深度修订 | 2026-08-12（第二轮全库量化扫描） |
| 基线 | 本地 `main`（后端约 `0.1.173` 合并后） |
| 状态约定 | `open` / `doing` / `blocked` / `mitigated` / `done` / `wontfix`；`mitigated` 表示风险已显著降低但根因仍在，**完成只改状态，不删行** |
| 扫描方法 | 见下文「扫描方法与仓库体量」；关闭项必须附验证证据 |
| 增量审计 | `docs/features/codex-full-repo-technical-debt-audit-cn.md`（`CX-TD-*`；含 2026-08-12 Grok 评审） |
| Fork 设计 | `docs/features/upstream-fork-governance-design-cn.md`（方案 A 五类强类型接缝、迁移与评审闭环） |
| Fork 度量 | `docs/features/upstream-fork-governance-effectiveness-test-standard-cn.md` v1.1（TD-009 验收尺子；含 Grok 评审） |

> 第一版偏「合并记录里写过的只记不修」。本版补齐：**体量量化、god-file、migration 双前缀全表、源码 TODO 全量、性能/计费洞、前端巨型 SFC、CI 覆盖缺口**。
> Codex 增量审计补：**发布供应链、Git 敏感产物、支持矩阵、race 门禁、密钥生命周期、仓库卫生**。两套编号独立；偿还时按下方「与 Codex 审计的合并优先级」插队。

---

## 怎么用

1. **优先级**
   - **P0** 安全、计费绕过、质量信号失效（CI/测试长期红）
   - **P1** 生产数据损坏风险、已知慢查询、合并/运维高摩擦
   - **P2** 复杂度、功能洞、包结构、可维护性
   - **P3** 风格噪音、已文档化设计权衡、低流量遗留
2. **纪律**
   - 上游原生问题：独立修复 PR 或等上游；**禁止塞进版本合并分支**
   - 本地基线红：专项 sprint，目标 `main` 真绿
   - 一项债一个可回滚 commit/PR；合并债与功能债不混
3. **关闭模板**（贴到条目下）

```markdown
### 关闭记录 · TD-00X
- 日期 / 分支 / commit:
- 摘要:
- 验证命令与结果:
- wiki/本看板是否更新:
- 残留风险:
```

---

## 扫描方法与仓库体量（2026-08-12）

### 方法

| 手段 | 范围 | 说明 |
| --- | --- | --- |
| 源码标记 | `backend/**/*.go` 排除 `ent/`、`wire_gen.go` | `TODO`/`FIXME`/`HACK`、显式 `t.Skip` |
| 体量 | `backend/internal`、`frontend/src` | 行数 ≥1000 的非测试 Go；前端 >50KB SFC |
| 迁移 | `backend/migrations/*.sql` | 数字前缀冲突分组 |
| 文档 | `docs/features/sub2api -merage-list.md`、`llm-wiki/wiki/*`、performance/design 文 | 「只记录不修」、已知风险 |
| CI | `.github/workflows/backend-ci.yml`、根/`backend` Makefile | lint 全量、前端 critical-only |
| 定点读码 | `image_storage.go` download、`organization_usage_repo.go` SQL、Live billing、DTO contract | 确认是否仍存在 |

### 体量快照（会漂移，作数量级）

| 指标 | 数值 | 含义 |
| --- | --- | --- |
| SQL migrations | **264** | 演进极长；不可重命名已发布文件 |
| 数字前缀冲突组 | **42 组**（涉及约 92 个「撞号」文件） | 远不止 wiki 写的 151/194/195 |
| `internal/service` 非测试 | **479 文件 / ~6.6 MB** | 单 package 巨石 |
| `internal/service` 测试 | **612 文件 / ~7.5 MB** | 测试比生产还大 |
| `openai_*.go`（service，含测） | **102 文件 / ~1.68 MB** | OpenAI/Grok 兼容域独占 |
| 非测试 Go ≥1000 行 | **72 文件** | god-file 普遍，非个例 |
| 最大单文件 | `config.go` **3665 行**；`account_repo.go` **3549**；`content_moderation.go` **3312** | 配置/账号/审核中枢 |
| `openai_gateway_handler.go` | **3088 行** | HTTP 入口巨石 |
| `wire_gen.go` | **~717 行 / 44KB** | DI 图已很重 |
| `routes/gateway.go` | **~672 行** | 路由与 root 别名密集 |
| 前端最大 SFC | `SettingsView.vue` **~530 KB** | 管理设置单页 |
| 前端 >50KB views | **12** | 管理端巨型页面集中 |
| i18n ts 文件 | **51** | 与巨型 settings/accounts locale 绑定 |
| 后端 CI lint | **全量** `golangci-lint` v2.9，无 only-new | 基线红则 job 常红 |
| 前端 CI | `make test-frontend` → **critical Vitest 子集**，非全量 | 全量红可漏进 main |
| `//nolint`（backend，excl ent） | **31** | 有意豁免点，需定期审计是否过期 |
| 残留 `interface{}` | **2** | 基本已迁 `any`，几乎无债 |
| `recover()` | **36** | 热路径/后台防御性 recover，排查时注意吞 panic |
| `.(map[string]any)` / `.([]any)`（service） | **~728** | JSON 动态结构断言密集，协议 sanitizer 主风格 |
| 最重 `gjson` 热文件（非测试，调用次数） | `openai_gateway_response_handling` 59、`gateway_claude_oauth_body` 45、`openai_images_responses` 43、`openai_gateway_grok` 39… | 请求/响应靠字符串 JSON 路径而非强类型 DTO |
| 本地分叉能力文件足迹（backend 文件名命中） | archive 5 / intercept 8 / prompt_risk 6 / token_analysis **19** / org_usage 9 / concurrency_preset 9 / platform_quota **17** | 与上游合并时的「必保」表面积（TD-009） |

### 源码 `TODO` 全量清单（生产相关，已排除 `context.TODO()`）

| 位置 | 摘要 | 看板 |
| --- | --- | --- |
| `service/image_storage.go` `download` | **无 host/SSRF 校验即 HTTP GET**（合并记录 + 读码确认） | TD-001 |
| `service/openai_live.go` | Live **TotalCost/ActualCost 恒 0**，余额可白嫖长会话 | TD-027 |
| `repository/usage_log_repo_trend.go` | `GetAllGroupUsageSummary` **全表扫 usage_logs** | TD-026 |
| `repository/organization_usage_repo.go` | peak CTE **未 MATERIALIZED**（性能文 11s→0.4s 候选未落地） | TD-025 |
| `service/content_moderation.go` | admin 命中 autoban 时 **不禁 API Key** | TD-034 |
| `service/account_service.go` | Anthropic/OpenAI/Gemini 凭证测试 TODO | TD-016 |
| `service/admin_account.go` | refresh logic TODO | TD-017 |
| `handler/admin/group_handler.go` | group stats 空实现 | TD-018 |
| `service/proxy_service.go` | 代理连接测试未实现 | TD-035 |
| `service/redeem_service.go` | 兑换统计未实现 | TD-036 |
| `service/batch_image_provider_gemini.go` | mime/aspect/size 未接 | TD-037 |
| `handler/auth_dingtalk_client.go` | appToken 未 Redis 多实例共享 | TD-032 |
| `handler/auth_dingtalk_oauth_test.go` | helper 未齐 **永久 Skip** | TD-038 |
| `payment/load_balancer.go` + `payment_config_providers.go` | legacy AES ciphertext fallback | TD-022 |
| `repository/concurrency_cache_integration_test.go` | CurrentConcurrency CI Skip | TD-019 |

---

## 看板总览

| ID | P | 状态 | 标题 | 类别 |
| --- | --- | --- | --- | --- |
| TD-001 | P0 | open | 异步图 URL 下载无 SSRF 防护（`image_storage.download`） | 安全 |
| TD-002 | P0 | open | 异步图 processing 重启不恢复 | 可靠 |
| TD-003 | P0 | open | 用户批量限额 cache/参数上限风险 | 配额边界 |
| TD-004 | P0 | open | 全量 golangci-lint 基线红（~28–40） | CI |
| TD-005 | P0 | open | `/auth/me` × `admin_permissions` contract mismatch | 测试基线 |
| TD-006 | P0 | open | Frontend Vitest 已知基线失败族 | 测试基线 |
| TD-027 | P0 | open | OpenAI Live 会话计费恒 0（可白嫖时长） | 计费 |
| TD-007 | P1 | open | migration 206 隐私 false→true 冲运营配置 | 数据 |
| TD-008 | P1 | open | 旧 migration 220 composite 视频价可能被清 | 数据/计费 |
| TD-009 | P1 | open | 本地 fork × 上游长期分叉与合并成本 | 架构 |
| TD-010 | P1 | open | migration **42 组**双前缀 + checksum 窄兼容 | 迁移工程 |
| TD-011 | P1 | open | Responses path `LastIndex("/responses")` 重锚 | 网关 |
| TD-012 | P1 | open | `go mod tidy -diff` 旧 checksum | 模块 |
| TD-025 | P1 | open | 组织用量 summary-items 病态计划（~11s/90d） | 性能 |
| TD-026 | P1 | open | 分组用量汇总全表扫 `usage_logs` | 性能 |
| TD-031 | P1 | open | first-output timeout 切号 → 重复上游计费 | 计费/设计 |
| TD-039 | P1 | open | CI 前端仅 critical Vitest，全量可红进 main | CI |
| TD-013 | P2 | open | 网关/兼容层 god-file 与协议矩阵 | 复杂度 |
| TD-014 | P2 | open | 内容审核三轨并存（legacy / Risk / Audit） | 架构 |
| TD-015 | P2 | open | 审核 proxy inactive 仍用 + 无 invalidation hook | 一致 |
| TD-016 | P2 | open | 账号平台凭证测试 TODO×3 | 功能洞 |
| TD-017 | P2 | open | 账号 refresh logic TODO | 功能洞 |
| TD-018 | P2 | open | group stats 空实现 | 功能洞 |
| TD-019 | P2 | open | concurrency cache 集成测试 Skip | 测试 |
| TD-020 | P2 | open | Windows `*.test.exe` 文件锁 | 环境 |
| TD-028 | P2 | open | `config.go` + Settings 前后端巨型配置面 | 复杂度 |
| TD-029 | P2 | open | `setting_handler_update` / account 管理巨石 | 复杂度 |
| TD-030 | P2 | open | `account_repo` 3500+ 行仓储巨石 | 复杂度 |
| TD-032 | P2 | open | 钉钉 appToken 进程内缓存（多实例） | 多实例 |
| TD-034 | P2 | open | 审核 autoban 跳过 admin，不禁触发 Key | 安全策略 |
| TD-035 | P2 | open | 代理连接测试未实现 | 功能洞 |
| TD-036 | P2 | open | 兑换码统计未实现 | 功能洞 |
| TD-037 | P2 | open | batch Gemini 图片参数未接全 | 功能洞 |
| TD-038 | P2 | open | 钉钉 OAuth 测试 sentinel Skip | 测试 |
| TD-040 | P2 | open | 网关 root 别名 / 多入口重复挂载 | 路由复杂度 |
| TD-041 | P2 | open | `service` 单包 479 文件巨石 package | 包结构 |
| TD-042 | P2 | open | 前端巨型 SFC（Settings 530KB 等） | 前端 |
| TD-021 | P3 | open | lint errcheck 假阳性（Builder/Close） | Lint |
| TD-022 | P3 | open | 支付 legacy AES ciphertext fallback | 遗留 |
| TD-023 | P3 | open | gateway TOCTOU soft-limit（有意权衡） | 设计 |
| TD-024 | P3 | open | Prompt Audit 生产 blocking 运营证据未齐 | 运营 |
| TD-033 | P3 | open | 历史 ASYNC_IMAGE 仅 env 配置被静默丢弃 | 运维史 |
| TD-043 | P3 | open | 大量 env/集成测试依赖 Skip（非债但掩盖覆盖） | 测试覆盖 |

**条目数：40**（TD-001–043，编号保留历史 ID，中间有意不连续以便稳定引用）

---

## P0 — 安全、计费绕过、质量信号

### TD-001 · 异步图 URL 下载无 SSRF 防护

| | |
| --- | --- |
| 状态 | open |
| 证据（读码） | `backend/internal/service/image_storage.go`：`download()` 对 `rawURL` 直接 `http.NewRequest` + `httpClient.Do`，**无** private IP / scheme / allowlist 校验；仅有下载字节上限 |
| 文档 | merge-list ~v0.1.159「异步图片 URL 下载缺 SSRF」 |
| 影响 | 若上游返回内网 URL 或攻击者可控 URL 进入转存链路，可打 metadata/内网 |
| 建议 | 复用 `security.url_allowlist` 或独立出站策略：仅 https、禁私网/链路本地、可选 host allowlist；单测覆盖 |
| 验证 | 私网、localhost、file、非 https → 拒绝；合法公网 → 成功 |
| 关闭 | 生产默认防 SSRF；wiki 安全页同步 |

### TD-002 · 异步图 processing 重启不恢复

| | |
| --- | --- |
| 状态 | open |
| 证据 | merge-list；`image_task.go` 状态机 processing/completed/failed，未见启动时 stuck 扫描（以当前 service 为准，修前再确认 worker 入口） |
| 影响 | 进程重启后任务永久 processing |
| 建议 | 启动/leader 扫描超时 processing → failed 或重入队；多实例加锁 |
| 关闭 | 重启演练任务可收敛 |

### TD-003 · 用户批量限额 cache/参数上限

| | |
| --- | --- |
| 状态 | open |
| 证据 | merge-list ~v0.1.159；路由 `POST .../users/batch-limits` |
| 影响 | 脏缓存终态或超大 batch 导致限额错误/DoS |
| 建议 | 核对 cache 失效；硬上限 + 集成测试 |
| 关闭 | 无已知绕过；测试锁住边界 |

### TD-004 · 全量 golangci-lint 基线红

| | |
| --- | --- |
| 状态 | open |
| 证据 | `docs/features/golangci-lint-debt-cleanup-plan-cn.md`（曾 40–44）；merge-list 持续 **~28–29** 基线；CI job **全量**扫描无 only-new |
| 影响 | lint job 对任意 PR 可红，**失去存量拦截**；团队只靠 `--new-from-rev` 心理安慰 |
| 建议 | chore 清零 **或** CI 正式 only-new + 本看板定期还债；二者必须选一并写进 ops |
| 验证 | `golangci-lint run --timeout=30m --max-same-issues=0 --max-issues-per-linter=0` → 0 |
| 关联 | TD-021 |

### TD-005 · `/auth/me` contract mismatch

| | |
| --- | --- |
| 状态 | open |
| 证据 | `handler/dto/types.go`：`AdminPermissions []string \`json:"admin_permissions"\`` **无 omitempty** → 空切片序列化为 `null` 或 `[]` 与 fixture 易冲突；merge-list 多轮「只记录」 |
| 影响 | 完整 unit 需排除该合同；掩盖回归 |
| 建议 | 固定合同（始终数组或 omitempty + 测试双端对齐） |
| 关闭 | 默认 `make test-unit` 不再因该字段失败 |

### TD-006 · Frontend Vitest 基线失败族

| | |
| --- | --- |
| 状态 | open |
| 子项 | **006a** `admin.system.rollback.spec.ts` 未接受 15min timeout；**006b** `GroupsView` 缺 `getLiveCapability` mock → unhandled rejection；**006c** image usage tooltip（`known-red-image-usage-vitest`） |
| 证据 | merge-list 多轮 Frontend 边界 |
| 关联 | TD-039（CI 不跑全量时更易漏） |
| 关闭 | 全量 Vitest 无已知红 |

### TD-027 · OpenAI Live 计费恒 0

| | |
| --- | --- |
| 状态 | open |
| 证据 | `service/openai_live.go` 明确 TODO：`TotalCost/ActualCost` 恒 0，绕过 `recordUsageCore/applyUsageBilling`；低余额可反复开最长 `liveMaxSessionDuration` 会话 |
| 影响 | **计费漏洞**（若 Live 应对用户收费）或未文档化的免费能力 |
| 建议 | 产品二选一：接入时长/用量计费；或文档+测试锁定「有意免费」并删模糊 TODO |
| 关闭 | 计费或正式免费合同 + 测试 |

---

## P1 — 数据、性能、合并、CI 诚实性

### TD-007 · migration 206 隐私设置

见 `llm-wiki/wiki/data-and-domain.md`。升级后核对 `channel_monitor_hide_throughput`；补偿只能**新 migration**。

### TD-008 · 旧 migration 220 composite 视频价

同上 wiki。按 checksum + backup 表判断，不假设自动恢复。

### TD-009 · fork 合并成本（结构性）

| | |
| --- | --- |
| 本地热点能力 | RequestArchive/RequestIntercept；Prompt Metrics/Risk/judge；Token Analysis；组织用量；子管理员；compatible cache usage；默认 reasoning effort；并发 preset；quota flusher |
| 证据 | `llm-wiki/wiki/README.md` 每轮同步；`docs/upstream-merge-playbook.md`；merge-list |
| 影响 | 每轮 Wire/gateway 顺序/settings 省略字段语义复核；人日高 |
| 缓解 | 冲突热点清单；本地能力模块边界；合并 PR 零还债（已有纪律） |
| 方案设计 | `docs/features/upstream-fork-governance-design-cn.md`；五类编译期强类型接缝，当前待用户评审，不代表已授权实施 |
| 效果测试 | `docs/features/upstream-fork-governance-effectiveness-test-standard-cn.md` v1.1；用代表性同 SHA A/B、契约零回归和连续三轮指标判定 |
| 当前进度 | **仍为 `open`**。阶段 0 已在 `github/main@8784a408...` 达到 `14/14` 合同、`97/97` 顶层测试和 `CPR=100%`；仅解除 Gateway Extension RED Gate，尚未实施方案 A、未做同 SHA A/B |
| 评审 | Grok 意见已纳入并校准：取消绝对 `SDP<=3` 与 `SCF` 硬降幅；先契约包、后结构迁移；`improving` 不降低 mitigated 门槛 |
| 关闭 | 不追求 done；合并成本显著下降且满足效果标准硬门槛时可标 mitigated |

### TD-010 · migration 双前缀（系统性问题）

| | |
| --- | --- |
| 状态 | open |
| **量化** | **42 个数字前缀**存在 ≥2 文件；远不止 151/194/195 |
| 样例 | `006`×2，`028`×3，`101`×4，`136`×4，`174`×3，`194`/`195`×2 … |
| 机制 | runner 按**完整文件名**排序 + `filename` 去重；数字前缀不唯一 |
| 风险 | 新人重命名/「整理编号」= 事故；checksum 窄兼容（195/218–220）≠ 数据正确 |
| 建议 | onboarding 强调；禁止整理历史编号；新文件严格避免再撞号（可用更高前缀或时间戳策略） |
| 关闭 | 约定写入 ops/onboarding；无新撞号 |

### TD-011 · Responses path 重锚

`rawOpenAIResponsesRequestPathSuffix` + `LastIndex("/responses")`（v0.1.170 审查）。独立修 + path 表测试。

### TD-012 · go.sum 旧 checksum

`go mod tidy -diff` 多轮非空。一次 chore 清空。

### TD-025 · 组织用量 SQL 病态计划

| | |
| --- | --- |
| 状态 | open |
| 证据 | `docs/features/organization-usage-report-performance-cn.md`：90 天 summary-items **~11s**，peak CTE 嵌套循环；`MATERIALIZED` 候选 **~418ms** |
| 生产代码 | `organization_usage_repo.go` 仍为 `day_peak AS (` **无** `MATERIALIZED`；explain 测试里另有 candidate query |
| 影响 | 管理报表超时/DB 打满；加索引无效 |
| 建议 | 落地物化 peak 或一次聚合 per-user peak；重跑 30/90/366 基线 |
| 关闭 | 生产 SQL 消除 600× 循环；基线文档更新数字 |

### TD-026 · GetAllGroupUsageSummary 全表扫

| | |
| --- | --- |
| 状态 | open |
| 证据 | `usage_log_repo_trend.go` TODO(perf)：`LEFT JOIN usage_logs` 无时间下界聚合 **全部历史** |
| 影响 | usage_logs >~1M 行时管理/dashboard 拖垮 DB |
| 建议 | 短缓存 / 预聚合表 / 限制时间窗 |
| 关闭 | 有上限或缓存；负载测试可接受 |

### TD-031 · first-output timeout 重复上游计费

| | |
| --- | --- |
| 状态 | open（设计风险，默认关） |
| 证据 | wiki backend/security/ops：超时切号重放，**原 attempt 上游用量不可撤销** |
| 建议 | 开启前成本评估；长期探索 attempt 幂等或客户端可见计费说明 |
| 关闭 | 产品签署风险 **wontfix**，或实现缓解 |

### TD-039 · CI 前端 critical-only

| | |
| --- | --- |
| 状态 | open |
| 证据 | 根 `Makefile`：`FRONTEND_CRITICAL_VITEST`；`backend-ci.yml` frontend job 跑 critical 非全量 |
| 影响 | TD-006 全量红可以长期存在而 CI 绿 |
| 建议 | CI 加全量 Vitest 夜间任务；或 main 门禁全量 |
| 关闭 | main 门禁与「全量绿」定义一致 |

---

## P2 — 复杂度、包结构、功能洞

### TD-013 · 网关/兼容层复杂度

| 量化线索 | |
| --- | --- |
| service `openai_*` | ~102 文件 / 1.7MB |
| handler | `openai_gateway_handler` 3088 行；`gateway_handler` 2267 行 |
| apicompat | `chatcompletions_responses_bridge` 1716 行等 |
| 协议面 | Responses↔Chat↔Anthropic、OAuth/APIKey/WS/compact/Grok/Gemini… |
| 建议 | 按 transport 拆包；契约矩阵测试；改动必读 security wiki |

### TD-014 · 审核三轨

legacy 内容审核 + 本地 Prompt Risk/judge + 上游 Prompt Audit（off/async/blocking）。配置与 fail 策略不一致风险。需产品级收敛说明。

### TD-015 · 审核 proxy 缓存

wiki：inactive 仍用；代理变更无 hook，≤1min TTL。建议 save/update/delete 清缓存。

### TD-016–018 · 功能洞 TODO

凭证测试 / refresh / group stats — 见源码表。实现或显式 501/删除死入口。

### TD-019 · concurrency 集成 Skip

`CurrentConcurrency returns 0 in CI` — 修实现或环境假设。

### TD-020 · Windows 测试锁

`ops.md` fresh GOTMPDIR；脚本化。CI 以 Linux 为准。

### TD-028 · 配置面巨石

| | |
| --- | --- |
| 后端 | `config/config.go` **3665 行**；`setting_parse.go` 1199+ 行 |
| 前端 | `SettingsView.vue` **~530 KB**；settings i18n en/zh 各 ~86 KB |
| 影响 | 任意 settings 改动冲突率极高、review 不可能通读 |
| 建议 | 按域拆 config 文件与 Settings 子路由/子表单 |

### TD-029 · 管理端 handler 巨石

`setting_handler_update.go` 2347 行；`account_handler.go` 2746 行。按资源拆文件。

### TD-030 · account_repo 巨石

`account_repo.go` **3549 行**。调度/凭据/查询拆 repository 或内部文件。

### TD-032 · 钉钉 appToken 多实例

`auth_dingtalk_client.go` TODO：进程内 token，多实例重复打钉钉。

### TD-034 · admin autoban 不禁 Key

`content_moderation.go`：admin 跳过封号，TODO 应禁触发 API Key 但 mutation 未接入 → **admin 名下 Key 可继续打**。

### TD-035–037 · proxy 测试 / redeem 统计 / batch Gemini 参数

见 TODO 表。

### TD-038 · 钉钉 OAuth 测试 Skip

`auth_dingtalk_oauth_test.go` sentinel Skip 等 Task 1.10 helper — 可能已过期仍 Skip。

### TD-040 · 网关多入口别名

`/v1` + 根级 + `/backend-api/codex/*` 重复注册；本地 Archive/Intercept 必须每条 root 显式挂载（易漏）。建议生成式注册表 + 单测覆盖「每别名中间件集合」。

### TD-041 · service 巨石 package

单 `package service` **479** 生产文件。编译/IDE/循环依赖风险；长期按域拆 `service/openai`、`service/billing` 等（成本极高，需渐进）。

### TD-042 · 前端巨型 SFC

| 文件 | ~KB |
| --- | --- |
| `SettingsView.vue` | 530 |
| `CreateAccountModal.vue` | 255 |
| `GroupsView.vue` | 253 |
| `EditAccountModal.vue` | 205 |
| `RiskControlView.vue` | 116 |
| … | 另有 Accounts/Users/Proxies 等 |

建议组合式拆分 + 单测按子模块。

---

## P3 — 风格、遗留、权衡、覆盖

### TD-021 · lint 假阳性

与 TD-004 一并 bulk fix 或调整 exclusion。

### TD-022 · 支付 legacy AES

`payment/load_balancer.go`、`payment_config_providers.go` deprecated-legacy-ciphertext。确认无密文后删。

### TD-023 · TOCTOU soft-limit

`gateway_handler` 有意与 WindowCost 一致。默认 **wontfix** 除非强一致需求。

### TD-024 · Prompt Audit blocking 门禁

openspec verification 运营 TODO。未齐前勿开 blocking。

### TD-033 · 历史 IMAGE_STORAGE 仅 env

`docs/ASYNC_IMAGE_TASKS.md`：v0.1.161 前 viper 无 default 时 env 静默丢弃。旧部署 workaround 文档化即可；新版本确认已修可 wontfix。

### TD-043 · 依赖环境的 Skip 海洋

E2E/Redis/Postgres/TLS/外部 Key 大量 Skip — **不是缺陷**，但让「集成全绿」含义变弱。建议 CI 矩阵标明必跑子集与可选子集。

---

## 与 Codex 审计的合并优先级

完整条目与抽检证据见 `docs/features/codex-full-repo-technical-debt-audit-cn.md`（第十一节为 Grok 评审）。此处只列**插队关系**，避免两份清单分叉遗忘。

| 序 | 来源 | 项 | 说明 |
| --- | --- | --- | --- |
| 1 | CX | **CX-TD-001** | Git 内 `dump.rdb`：按**安全事件**处理，不是普通 chore |
| 2 | CX | **CX-TD-002 / 003 / 011** | Release 同 SHA、verify 门禁、VERSION 自洽 |
| 3 | TD | **TD-001 / TD-027** 等本看板 P0 | 异步图 SSRF、Live 零计费等业务面；与 CX **并行**，不降级 |
| 4 | CX | **CX-TD-008** | 执行已有 cleanup-plan（diff/死代码/numstat） |
| 5 | 混合 | 本看板 Sprint A 余项 + CX-TD-004/005/009/010 | 质量门禁、PG 矩阵、race、DEV_GUIDE、govulncheck 钉版本 |
| 6 | 混合 | Sprint C 性能/数据 + CX-TD-006/007 | SQL、migration 核对、forwarded IP 分阶段、密钥生命周期 |
| 7 | TD | Sprint D–F + TD-009 设计 | fork 设计评审批准 + 契约包后，再按 v1.1 用代表性 exact SHA 验收 |

**关闭 CX 项时**：在 Codex 文档改状态并留证据，并在本表对应行备注 commit（可只改本节表格，不必复制全文）。

---

## 推荐还债顺序

```text
Sprint 0 — 供应链与仓库卫生（Codex 增量，评审后插入）
  CX-TD-001 (dump.rdb 事件) → CX-TD-002+003+011 (release) → CX-TD-008 (cleanup-plan)

Sprint A — 计费与安全真问题（本看板 P0，不降级）
  TD-027 (Live 计费) → TD-001 (SSRF) → TD-002 → TD-003 → TD-034

Sprint B — 质量信号还魂
  TD-005 → TD-006 → TD-039 → TD-004(+021) → TD-012
  并行：CX-TD-004 / 005 / 009 / 010

Sprint C — 数据与性能
  TD-025 (组织用量 SQL) → TD-026 (分组汇总扫表) → TD-007/008 运维核对 → TD-010 文档固化
  并行设计：CX-TD-006 / 007

Sprint D — 网关与合并摩擦
  TD-011 → TD-015 → TD-040 注册表
  TD-009：先契约包 + 实现设计，再用 fork 效果标准 A/B（非每次 merge）

Sprint E — 巨石拆分（长期、可并行小步）
  TD-028 Settings 子页 → TD-042 大 SFC → TD-029/030 文件拆分 → TD-041 包拆分（最后）

Sprint F — 功能洞与杂项
  TD-016..019, 032, 035..038, 022, 024, 031, 033, 043, CX-TD-012
```

**并行约束（再次强调）**

- 版本合并 PR：**零**上表功能修复与 CX 清理
- P0 / CX-TD-001 可插队，仍单独 PR、单独回滚
- 禁止用 fork A/B 或拆 god-file 代替 Sprint 0 / Sprint A

---

## 相关文档

| 文档 | 用途 |
| --- | --- |
| `docs/features/codex-full-repo-technical-debt-audit-cn.md` | Codex 增量审计 `CX-TD-*` + Grok 评审 |
| `docs/features/upstream-fork-governance-design-cn.md` | TD-009 方案 A 架构、迁移、回滚和评审闭环 |
| `docs/features/upstream-fork-governance-effectiveness-test-standard-cn.md` | TD-009 v1.1 效果验收 + Grok 评审 |
| `docs/features/sub2api -merage-list.md` | 上游合并与只记不修 |
| `docs/features/golangci-lint-debt-cleanup-plan-cn.md` | lint 明细 |
| `docs/features/organization-usage-report-performance-cn.md` | TD-025 基线 |
| `cleanup-plan-2026-07-08.md` | CX-TD-008 待执行清理清单 |
| `docs/ASYNC_IMAGE_TASKS.md` | 异步图/TD-033 |
| `docs/upstream-merge-playbook.md` | 合并纪律 |
| `llm-wiki/wiki/data-and-domain.md` | migration 风险 |
| `llm-wiki/wiki/security-and-reliability.md` | 网关/审核/first-output |
| `llm-wiki/wiki/ops.md` | 验证命令、Windows 锁、CI 相关 |

---

## 变更日志

| 日期 | 说明 |
| --- | --- |
| 2026-08-12 | 首版 TD-001–024（合并记录导向） |
| 2026-08-12 | **深度扫描修订**：体量快照；migration 42 组双前缀；TODO 全表；读码确认 SSRF/Live 零计费/组织用量未物化；新增 TD-025–043；条目共 40；Sprint 重排 |
| 2026-08-12 | 补全后台扫描：nolint/interface{}/recover/类型断言/gjson 热文件/本地能力文件足迹 |
| 2026-08-12 | 接入 Codex 审计与 Grok 评审：合并优先级、Sprint 0、TD-009 评审指针、相关文档链 |
| 2026-08-12 | TD-009 提升：新增方案 A 正式设计；效果标准升 v1.1；债务状态补 `mitigated` 并与效果等级分离 |
