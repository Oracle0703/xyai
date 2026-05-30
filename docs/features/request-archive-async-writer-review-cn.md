# Request Archive 异步写入改造 — 深度代码审查报告

> 审查对象: 当前工作区 `gateway.request_archive` 全链路改造（异步写入 + 运行态开关 + 管理后台 UI）。
> 配套设计文档: [request-archive-async-writer-technical-notes-cn.md](./request-archive-async-writer-technical-notes-cn.md)
> 审查日期: 2026-05-30
> 结论摘要: **方案合理、可上线**。核心异步写入与运行态开关链路正确、测试覆盖到位（含新增服务层用例 `request_archive_settings_test.go`，全绿）；存在若干非阻塞性健壮性与最佳实践改进项（无 P0）。设计文档"Gateway 标签页"的描述经核对**与实现一致**。回归中 `internal/service` 的 2 个失败用例经定位**与本特性无关**（既有的定价 codegen 漂移 + OpenAI chat 兼容），详见 §2。另有 1 处与本特性无关的"重置日限"前端变更夹带在同一工作区，建议拆分提交。

---

## 1. 审查范围与方法

全链路覆盖以下变更文件：

| 层 | 文件 | 角色 |
| --- | --- | --- |
| 配置 | `backend/internal/config/config.go` / `config_test.go` | 默认值 + `QueueSize` 字段 |
| 中间件 | `backend/internal/server/middleware/request_archive.go` / `_test.go` | 异步 writer、provider、handler |
| 服务 | `backend/internal/service/setting_service.go` / `settings_view.go` / `domain_constants.go` | 运行态配置读写 + 缓存 |
| Handler | `backend/internal/handler/admin/setting_handler.go` / `dto/settings.go` | GET/PUT API |
| 路由 | `backend/internal/server/routes/admin.go` / `gateway.go` / `gateway_test.go` | 路由注册 + provider 注入 |
| 前端 | `frontend/src/views/admin/SettingsView.vue` / `api/admin/settings.ts` / `i18n/*` | 管理后台开关 UI |
| 文档 | `deploy/config.example.yaml` / `llm-wiki/wiki/backend.md` | 示例与知识库 |

方法：阅读全量 diff + 关键文件全文，比对设计文档逐条断言，运行单元测试回归。

---

## 2. 测试回归结果

执行 `go test -tags=unit ./...`（backend）：

- **直接相关包全部通过**：`internal/config`、`internal/server/middleware`、`internal/server/routes`、`internal/handler/admin` 均 `ok`。
- **新增服务层用例通过**：`internal/service` 的 `request_archive_settings_test.go`（未跟踪文件）三个用例全绿——`TestGetRequestArchiveSettingsDefaultsFromConfig`、`TestGetRequestArchiveSettingsUsesPersistedSwitchesOnly`（验证 DB 只覆盖 `enabled`/`capture_response`，`dir`/`max*`/`queue` 恒取配置文件值）、`TestSetRequestArchiveSettingsUpdatesRuntimeCache`（写库后运行态缓存即时更新）。
- **`internal/service` 报 2 个 FAIL，逐个隔离复跑后确认与本特性无关、属分支既有问题**：
  1. `TestDefaultPricingIncludesCodexAutoReview`（`pricing_service_test.go:127`）：浮点价格断言不符（expected `2.5e-06`，actual `5e-06`），属**定价元数据**问题（与近期提交 `chore(pricing): update model pricing metadata` 相关）。
  2. `TestForwardAsChatCompletions_BufferedTopLevelTerminalUsage`（`openai_gateway_chat_completions_test.go:446`）：`expected 3, actual 0`，属 **OpenAI chat 兼容转发 / 顶层 usage 缓冲**领域（与近期提交 `feat: sanitize OpenAI request bodies ... 'thinking' field` 相关）。
- 旁证：`git diff --stat` 显示本工作区改动**未触及** `pricing_service*`、`openai_gateway_chat_completions*` 等文件；这两个用例用 `-run` 单独执行仍失败，说明非并发/flaky，而是分支上的既有失败。
- 另：`internal/service` 输出里那条 `SubscriptionMaintenance worker panic: boom` 是 `TestSubscriptionMaintenanceQueue_TryEnqueue_PanicDoesNotKillWorker` **故意触发的 panic 断言**，非失败。

> ✅ 结论：**request_archive 全链路未引入任何测试回归**。上述 2 个失败是分支既有问题，建议另行处理（定价用例核对 codex auto-review 单价；chat 兼容用例归因到 thinking 字段清洗提交）。

---

## 3. 设计文档合理性验证

逐条核对 `request-archive-async-writer-technical-notes-cn.md`：

| 文档断言 | 代码核对 | 结论 |
| --- | --- | --- |
| 默认 `enabled=false` / `capture_response=false` | `config.go` `setDefaults` + `config_test.go` 断言 | ✅ 一致 |
| 新增 `queue_size` 默认 1024 | `GatewayRequestArchiveConfig.QueueSize` + 默认值 | ✅ |
| 热路径只做非阻塞 `Enqueue` | `Enqueue` 用 `select{case ch<-: default:}` | ✅ |
| 队列满丢弃 + 每 256 条采样告警 | `dropped%256==1` 触发 `Warn` | ✅ |
| 后台单 writer 持久句柄 + 按日期轮转 | `run()` + `fileForToday()` | ✅ |
| `capture_response` 独立开关，关闭时不包装 Writer | handler `if runtimeCfg.CaptureResponse` | ✅，并有专项测试 |
| 运行态可热切换 `enabled`/`capture_response`，写 settings 表 | `SetRequestArchiveSettings` + 缓存 | ✅ |
| `dir`/`body 上限`/`queue_size` 仍由配置文件控制、改后需重启 | 见 §5.2「细节」 | ✅ 成立（机理描述可微调） |
| 开关位置：`/admin/settings` → **Gateway 标签页** → 请求归档 | `SettingsView.vue` 卡片在 `activeTab === 'gateway'` 区块内 | ✅ 与实现一致 |

总体：设计文档作为"上游升级后快速迁移"的技术留底是**合格且准确的**，方案取舍（drop vs 阻塞、句柄常驻 vs 每条 open/close）论证充分、与代码自洽。仅 §5.2 的一处机理描述可微调，无实质偏差。

---

## 4. 代码健壮性审查（逐项）

按严重度排序。**无 P0 阻断项。**

### P1 — 建议尽快处理

**H1. `Enqueue` 与 `Close` 之间存在"向已关闭 channel 发送"的竞态（latent panic）。**
`Close()` 执行 `close(w.ch)`，而 `Enqueue` 在另一条 goroutine 里对同一 `w.ch` 执行 `select{case w.ch<-record:}`。Go 规范下"多发送者 + 由非发送者关闭 channel"是已知反模式：一旦在请求在途时调用 `Close`，发送分支会 `panic: send on closed channel`。
- 现状为何不爆：生产环境 `Close` 仅文档约定"随进程常驻不调用"；测试里 `t.Cleanup(writer.Close)` 在请求已结束后才跑。所以**当前不会触发**。
- 风险：`CloseRequestArchiveWritersForTest` / 未来接入"优雅停机"时若队列尚有在途 `Enqueue`，即可能 panic。
- 建议：改为"不由发送者侧关闭数据 channel"的范式 —— 增加独立 `quit chan struct{}`，`Close` 只 `close(quit)`；`run()` 用 `select { case rec := <-ch: ...; case <-quit: drain&return }`；`Enqueue` 在发送前 `if w.closing.Load() { return }`（已有 `closing` 原子量，仅需在发送路径加判断 + 一个 `recover` 兜底）。

### P2 — 推荐改进（健壮性/可维护性/可观测性）

**H2. 包级可变单例 `requestArchiveActiveWriters` / `requestArchiveRegisterWriter`。**
每次 `RequestArchiveWithProvider` 都把 writer 注册进包级 slice，仅测试 helper 清理。生产中它常驻持有唯一 writer（无害），但这种全局可变状态使中间件不可重入，也是测试需要"手术式"`removeRequestArchiveWriterForTest` 的根因。更干净的做法：构造函数把 `*asyncRequestArchiveWriter`（或 `io.Closer`）返回给路由注册方，由调用方在 server 生命周期里持有/关闭，去掉全局注册表。

**H3. 配置解析错误被静默吞掉。**
`getRequestArchiveSettingsUncached` 在 `json.Unmarshal` 失败时直接 `return settings(默认/关闭), nil`，不记日志。一行损坏的 settings 记录会**静默关闭归档且无任何告警**。建议在该分支 `logger.L().Warn("request_archive.settings_unmarshal_failed", ...)`。失败安全方向是对的，缺的是可观测性。

**H4. 后台写入未做缓冲/批量，且 `append(line,'\n')` 每条都分配。**
`writeRecord` 对每条记录 `f.Write(append(line,'\n'))`，即"每记录一次 syscall + 一次切片扩容"。高频归档时后台 goroutine 的 syscall 数 = 记录数。建议引入 `bufio.Writer`（按 channel 排空或定时 flush），可将写 syscall 降一个数量级；权衡是崩溃时丢失缓冲窗口内的记录 —— 对"排障辅助数据"完全可接受。这是本特性**性价比最高的性能改进范式**。

**H5. 仅按日期轮转，单日文件仍可无限增长。**
本次改造的诱因正是"单日 1GB"。新方案消除了锁/同步 IO，但**没有解决单日文件大小**：仍可能在开启期写出超大单文件。建议补"按大小切片 + 保留 N 天/N 个文件自动清理"，从根因上更持久地防止磁盘膨胀（当前仅靠"默认关闭 + 人工清理"兜底）。

**H6. `dropped` 计数仅落日志，未进指标体系。**
丢弃量是判断"队列是否需要调大/归档是否过载"的关键信号，目前只能从采样日志里翻。若项目已有 metrics/Prometheus，建议把 `dropped_total` 暴露为指标。

**H7. 缺优雅停机 drain。**
`Close` 被标注"测试用"，生产进程退出时不排空队列 —— 已入队未落盘的记录在重启时丢失。对排障数据可接受，但把 `Close` 接到 server shutdown 钩子是低成本的最佳实践收益（同时需先解决 H1 的关闭竞态）。

### P3 — 细节/可读性

**H8. `mergeRequestArchiveRuntimeConfig` 合并了 `Dir`/`QueueSize`，但二者对 writer 无效。**
异步 writer 的 `dir`/`queueSize` 在 `newAsyncRequestArchiveWriter` 构造时固化；运行态 `runtimeCfg.Dir/QueueSize` 不会改变实际落盘目录与队列容量。当前因运行态值与静态配置同源、取值一致而**无 bug**，但合并这两个字段是"死逻辑/误导"——一旦未来运行态来源分叉就会变成真 bug。建议运行态只合并真正生效的 `Enabled`/`CaptureResponse`/`Max*BodyBytes`，并在注释里写明 writer 的 dir/queue 为构造期固定。

**H9. `RequestArchiveConfigProvider` 的 error 分支实为死代码。**
`NewRequestArchiveConfigProvider` 始终返回 `nil` error（`GetRequestArchiveRuntimeConfig` 内部已吞错并回退默认）。handler 里 `if err != nil { Warn }` 永不触发。无害，但接口签名带来"会失败"的错觉，可简化或保留以备扩展（明确注释）。

**H10. 前端 `requestArchiveForm` 含 `max_request_body_bytes`/`max_response_body_bytes` 字段但 UI 未展示。**
`Object.assign(form, settings)` 会写入这两个值，但模板只渲染 `dir` 与 `queue_size`。非 bug，属冗余字段，建议要么展示（只读）要么删除以减歧义。

### 正确性亮点（值得肯定）

- ✅ **禁用即零成本**：`provider==nil && !cfg.Enabled` 时返回纯透传 handler，不读 body、不包装 Writer、不起 goroutine；运行态禁用时 `shouldArchiveGatewayRequest` 先于 provider 调用短路。新增 `TestRequestArchiveRuntimeDisabledDoesNotReadBody` / `TestRequestArchiveDisabledDoesNotWriteFiles` 精确锁定该行为。
- ✅ **缓存读取范式正确**：`GetRequestArchiveRuntimeConfig` 采用 `atomic.Value` 缓存 + `singleflight` + Do 内二次校验 + `context.WithoutCancel` 隔离请求取消，5s TTL、错误短 TTL 回退旧值，与仓库既有 `openai_quota_auto_pause` 等模式一致。热路径每请求成本仅 1 次原子读 + 1 次时间比较，可忽略。
- ✅ **文件句柄并发安全**：`file`/`fileDate` 仅由单一 `run()` goroutine 访问，`Close` 通过关闭 channel→等待 `done` 让 `run()` 自行收尾关闭句柄，无数据竞争。
- ✅ **持久化最小面**：仅 `Enabled`/`CaptureResponse` 入库（`persistedRequestArchiveSettings`），UI 请求体也只接收这两个字段，从协议层杜绝了从后台篡改"需重启"参数。
- ✅ **权限位**：归档目录 `0o700`、文件 `0o600`，配合 UI 的敏感数据告警，符合最小权限。

---

## 5. UI 操作指引 + 文档/实现偏差

### 5.1 如何在页面上操作（实测自代码）

后端已暴露运行态开关，前端已落地操作卡片：

1. 进入**管理后台 → 系统设置（`/admin/settings`）**。
2. 切到承载该卡片的标签页，找到 **"请求归档 / Request Archive"** 卡片。
3. 操作元素：
   - **启用请求归档**（`enabled`）开关 —— 打开后才会归档网关 POST 请求；
   - 打开后展开 **捕获响应体**（`capture_response`）开关 + 只读的 **归档目录 / 队列容量** + 风险提示；
   - 底部 **保存** 按钮 → 调 `PUT /api/v1/admin/settings/request-archive`，写入 settings 表。
4. 生效方式：中间件经缓存 provider 在请求热路径读取，**保存后 ≤5s 自动生效，无需重启**。
5. 后端接口：`GET/PUT /api/v1/admin/settings/request-archive`（见 `routes/admin.go`）。
6. **仅配置文件可改、改后需重启**的参数：`dir`、`max_request_body_bytes`、`max_response_body_bytes`、`queue_size`（见 `deploy/config.example.yaml` 的 `gateway.request_archive`）。

> ✅ 因此你的诉求"`request_archive` 能在页面上有操作按钮"——**已实现**：是一个带"启用/捕获响应体"两个开关 + 保存按钮的设置卡片，且热生效。

### 5.2 标签页核对结果：文档正确 ✅

经核对 `SettingsView.vue`：归档卡片（`requestArchive.title`，源码约 414 行）落在 `<div v-show="activeTab === 'gateway'">`（起 205 行，下一标签 `security` 起 1477 行）区块内，与同属 Gateway 标签页的 `overloadCooldown`(212)、`rateLimit429Cooldown`(313)、`streamTimeout`(542) 卡片相邻；`settingsTabs` 也含 `{ key: "gateway", icon: "server" }`。**因此技术留底/示例配置/wiki 三处"Gateway 标签页"的表述均准确，无需订正。**（此前一度怀疑落在限流标签页，核对后排除。）

另一处细节：技术留底说 `max_request_body_bytes`/`max_response_body_bytes`"需重启"。实现上它们其实**每请求从配置值读取并应用**（经 `provider→settings→config` 注入 `runtimeCfg`），但由于运行态不接受这两个字段的修改、值恒等于配置文件值，所以"想改它们必须改配置文件并重启"的**结论正确**，只是机理描述（"运行态不读取"）不够精确，可补一句"运行态会读取但取值恒等于配置文件值，UI 不开放修改"。

另一处细节：技术留底说 `max_request_body_bytes`/`max_response_body_bytes`"需重启"。实现上它们其实**每请求从配置值读取并应用**（经 `provider→settings→config` 注入 `runtimeCfg`），但由于运行态不接受这两个字段的修改、值恒等于配置文件值，所以"想改它们必须改配置文件并重启"的**结论正确**，只是机理描述（"运行态不读取"）不够精确，可补一句"运行态会读取但取值恒等于配置文件值，UI 不开放修改"。

---

## 6. 与本特性无关的夹带变更（范围提示）

`frontend/src/views/admin/SubscriptionsView.vue` + 对应 i18n 新增了 **"重置日限 / Reset Daily"** 功能（仅重置每日用量，复用 `resetQuota({daily:true,weekly:false,monthly:false})`，并通过 `closeResetQuotaConfirm` 统一收尾对话框状态）。

- 该变更**与 request_archive 无关**，逻辑本身看起来正确（弹窗标题/文案/确认按钮按 `resetQuotaMode` 切换，状态复位完整），且附带了未跟踪测试 `frontend/src/views/admin/__tests__/SubscriptionsView.spec.ts`。
- 建议：**拆成独立提交**，避免与归档改造混在同一 PR/工作区，便于回滚与代码审查归因。

---

## 7. 推荐的优化与改进范式（带理由 / 亮点 / 优势）

按"投入产出比"排序，便于你决定是否要我落地。预估改动量见括号。

1. **后台写入加 `bufio.Writer` + 排空/定时 flush（H4，小，~30 行）**
   - 为什么：把"每记录一次 write syscall"降为"批量一次"，在高频归档下显著降低后台 goroutine 的内核态开销与磁盘 IOPS。
   - 亮点/优势：直接强化本次改造的核心目标（不拖慢尾延迟、扛住流式洪峰）；崩溃丢失窗口对排障数据可接受，风险可控。

2. **关闭路径改 quit-channel 范式 + `Enqueue` 关闭判断（H1，小，~25 行）**
   - 为什么：消除"向已关闭 channel 发送"的 latent panic，并为 H7 的优雅停机铺路。
   - 亮点/优势：符合 Go "不由发送者关闭 channel"的惯例，让 `Close` 从"测试专用"升级为"生产可安全调用"，是接入 server shutdown 的前置条件。

3. **大小切片 + 保留策略自动清理（H5，中，~60–100 行）**
   - 为什么：从根因上根治"单日文件膨胀到 GB"的原始事故，而不仅靠"默认关闭 + 人工清理"。
   - 亮点/优势：即使运维忘记关开关，磁盘占用也有硬上限；运维心智负担显著下降。

4. **`dropped` 进指标 + 解析错误告警（H6+H3，小，~15 行）**
   - 为什么：补齐可观测性闭环——能量化"队列是否过载/容量是否需调大"，并让损坏配置不再静默。
   - 亮点/优势：低成本拿到"归档是否健康"的运行时信号。

5. **去掉全局 writer 注册表，构造函数返回 closer（H2，中，~40 行 + 测试简化）**
   - 为什么：消除包级可变单例，提升可测性与可重入性。
   - 亮点/优势：测试不再需要 `removeRequestArchiveWriterForTest` 这类手术式清理，生命周期归属清晰。

> 1+2+4 都是小改动且互相独立，建议作为"上线前/紧随上线"的健壮性补强批次；3、5 体量略大，可排到后续迭代。**若你同意，我可以按上述顺序落地并补对应单测**（每项都附测试，跑 `go test -tags=unit ./internal/server/middleware/ ./internal/config/` 验证）。

---

## 8. 上线前必做闭环（Checklist）

- [x] **归因 `internal/service` 的 FAIL** —— 已定位为 2 个既有、与本特性无关的失败（定价 codegen 漂移 + OpenAI chat 兼容），request_archive 自身用例全绿（§2）。
- [x] **确认 UI 卡片所属标签页** —— 已核对为 **Gateway 标签页**，文档表述正确，无需订正（§5.2）。
- [ ] （与本特性无关，建议）修复分支既有 2 个失败：定价用例跑 `go generate ./...` 后复测；chat 兼容用例归因到 thinking 字段清洗提交。
- [ ] （可选，建议）拆分 SubscriptionsView "重置日限"为独立提交（§6）。
- [ ] （可选，建议）采纳 §7 第 1/2/4 项健壮性补强。

---

## 9. 总评

本次改造在**正确性与方向上是高质量的**：默认关闭对齐注释、异步有界队列 + 单 writer 持久句柄 + drop-on-full 是处理"非关键、可丢、怕阻塞"型旁路数据的教科书范式；运行态开关用缓存 + singleflight + 隔离 context 实现，热路径成本可忽略；测试把"禁用零副作用 / 不包装 Writer / 队列满不阻塞 / 关联与截断"等关键不变量都钉住了。

剩余改进集中在**健壮性边角（关闭竞态）、可观测性（丢弃指标/解析告警）与运维持久性（缓冲、大小轮转、优雅停机）**，均为非阻断项。配合 §8 的两个待确认闭环，即可放心上线。

---

## 10. 数据验证 + 修复决策 + 落地记录（2026-05-30 追加）

应"用数据验证问题是否真实、再决策、再落地"的要求，对关键评审项做了可复现的测试/基准验证，结论如下。

### 10.1 用数据验证：问题是否真实存在

| 项 | 验证手段 | 实测数据 | 真实性 |
| --- | --- | --- | --- |
| **H1 关闭竞态 panic** | 确定性用例 + 32 并发 goroutine 压测 | 已关闭 channel 上 `Enqueue` 必 panic（`send on closed channel`）；并发关闭场景下单轮观测到 **34,907 次** panic（生产 `Enqueue` 无 `recover` → 等价进程崩溃） | ✅ **真实** |
| **H8 运行态 Dir 死逻辑** | 构造期 dir=A、运行态 merge dir=B，写一条记录看落盘位置 | 记录落在 A，B 目录为空；`merge` 后 `Dir/QueueSize` 对落盘**无任何影响** | ✅ **真实（但无害）** |
| **H4 后台写入需缓冲？** | `writeRecord` 基准 + 端到端 drain 吞吐实测 | 无缓冲 **12.8µs/op、5158 B/op、3 allocs**；带 `bufio` **1.3µs/op、0 alloc**（快 ~10×）。但单 writer 端到端 drain ≈ **70,000 records/sec**，而生产实测 **1900 条/天 ≈ 0.022 条/秒** | ⚠️ **开销真实，但必要性证伪** |

> H4 关键结论：单 writer 落盘能力比生产实际归档速率高 **约 300 万倍**。缓冲虽能把单条写入提速 10×，但对真实负载毫无瓶颈意义，反而引入"崩溃丢失缓冲窗口数据"的代价。**数据直接否定了缓冲的必要性**——这正是"先验证再动手"的价值：评审初稿曾把缓冲列为第一优先，数据推翻了它。

### 10.2 多方案对比与决策

**H1（关闭竞态）——决定修复。**

| 方案 | 做法 | 取舍 | 结论 |
| --- | --- | --- | --- |
| A. Enqueue 加 `recover` | 每次入队 `defer recover()` | 掩盖问题；热路径每条 defer 开销 | ❌ |
| B. quit-channel 范式 | 不由发送方关闭 `ch`；`Close` 改关 `quit`；`run()` select 收到 quit 后尽力排空再退出；`Enqueue` 因 `ch` 永不关闭而天然安全 | 符合 Go "多发送者不由他人关闭 channel"惯例；热路径零新增开销（`Enqueue` 不变）；`Close` 升级为生产可安全调用，解锁优雅停机 | ✅ **采纳** |
| C. 读写锁包裹 send/close | `RWMutex` 保护 | 热路径引入锁竞争，违背无锁初衷 | ❌ |

**H8（死逻辑）——决定修复（廉价）**：`mergeRequestArchiveRuntimeConfig` 不再合并 `Dir/QueueSize`（显式置为构造期 base 值并加注释），仅保留真正按请求生效的 `CaptureResponse/Max*BodyBytes`。

**H3（静默吞错）——决定修复（一行）**：解析损坏 settings 时补 `slog.Warn`，与本文件既有日志风格一致；回退默认（关闭）行为不变，已有用例覆盖。

**H4（缓冲）——决定不修复**：数据证伪必要性（见 10.1），且增加崩溃丢数据窗口，性价比为负。

**H6（丢弃量进指标）——决定暂缓**：仓库内无 Prometheus/metrics 注册体系（已确认无 `promauto`/`metrics.Counter`），现有"每 256 条采样告警"足够；待引入指标体系后再做。

**H2（去全局 writer 注册表）/ H7（优雅停机 drain）——决定暂缓为下一步**：H1 修复后 `Close` 已生产安全，`cmd/server/main.go:176` 存在 `Server.Shutdown` 钩子，二者宜一并改造（让路由注册方持有 writer/closer 并接入停机），属独立小迭代。

### 10.3 已落地实现（本次提交）

- [backend/internal/server/middleware/request_archive.go](../../backend/internal/server/middleware/request_archive.go)：
  - 新增 `quit chan struct{}`；`run()` 改为 `select{ch, quit}` + 收到 quit 后尽力排空；`Close()` 改 `close(w.quit)`（不再 `close(w.ch)`）。`Enqueue` 保持不变、天然不再 panic。
  - `mergeRequestArchiveRuntimeConfig` 移除 `Dir/QueueSize` 死合并并加注释。
- [backend/internal/service/setting_service.go](../../backend/internal/service/setting_service.go)：损坏 settings 解析补 `slog.Warn`。
- [backend/internal/server/middleware/request_archive_test.go](../../backend/internal/server/middleware/request_archive_test.go)：新增回归用例 `TestAsyncWriterCloseDuringConcurrentEnqueueDoesNotPanic`（并发 Enqueue 期间 `Close` 不得 panic；若有人退回 `close(w.ch)` 即崩溃告警）。

### 10.4 验证结果

- `go build ./...` 通过；`gofmt -l` / `go vet` 改动文件无告警。
- `go test -tags=unit ./internal/server/middleware/`：**全绿**；新回归用例 `-count=20 -race` 连续 20 轮全过。
- `go test -tags=unit -race ./internal/server/middleware/`：**全绿（32s）**，无 DATA RACE。
- `internal/config`、`internal/server/routes`、`internal/handler/admin`、`internal/service` 的 request_archive 用例全绿。
- 仍存在的 2 个失败 `TestForwardAsChatCompletions_BufferedTopLevelTerminalUsage`、`TestDefaultPricingIncludesCodexAutoReview` 与本特性无关（§2）。
- 注：本机 Windows + `-race` 偶发 `fork/exec ... Access is denied`（Defender 拦截临时测试 exe），属环境问题，非代码问题；去掉 `-race` 即稳定通过。

### 10.5 建议的下一步

1. **优雅停机 + 去全局注册表（H2+H7 合并小迭代）**：让 `RegisterGatewayRoutes` 返回/持有 writer 的 `io.Closer`，在 `cmd/server/main.go` 的 `Server.Shutdown` 后调用，停机时 flush 队列；同时删除包级 `requestArchiveActiveWriters` 单例。预估 ~40–60 行。
2. **按大小轮转 + 保留策略（H5）**：单日文件加体积上限切片 + 保留 N 天自动清理，从根因防止"开启期磁盘膨胀"，降低运维心智。预估 ~60–100 行。
3. **修分支既有 2 个失败用例**：定价用例核对 codex auto-review 单价；chat 兼容用例归因到 thinking 字段清洗提交。
4. **拆分无关变更**：把 SubscriptionsView "重置日限"独立成单独提交/PR（§6）。
5. **（可选）敏感数据脱敏**：归档 body 可能含 PII，可复用仓库 `logredact` 在落盘前对已知敏感字段脱敏。
