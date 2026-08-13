# 本地 Fork 治理（方案 A）效果测试与验收标准

| 字段 | 值 |
| --- | --- |
| 版本 | `1.1`（采纳并校准 2026-08-12 外部评审；首次演练同时保留 v1.0 对照结果） |
| 日期 | 2026-08-12 |
| 状态 | 正式效果标准，待方案 A 设计批准和首次代表性 A/B 演练 |
| 适用债务 | `TD-009`：本地 fork 与 `Wei-Shaw/sub2api` 长期分叉 |
| 治理目标 | 在不丢失本地能力的前提下，降低每次上游同步的人工语义判断和验证成本 |
| 非目标 | 不以减少 Git 提示的冲突数代替真实治理；不要求一次性消灭 fork；不在合并分支顺手修复上游或既有基线问题 |
| 外部评审 | 2026-08-12 Grok；见「十一、外部评审（Grok）」 |
| 实现设计 | `docs/features/upstream-fork-governance-design-cn.md`；设计批准前不得据本标准开工重构 |
| 首次执行 | `docs/features/upstream-fork-governance-validation-report-cn.md`；初测 `11/14`、补齐后 `14/14`，当前 Gate 为 Gateway Extension RED |

> 核心判据：方案 A 只有同时满足“本地能力零回归”和“人工语义决策显著下降”才算有效。测试全绿只能证明没有明显破坏，不能单独证明合并成本下降。

---

## 一、适用范围

本标准用于评价以下一类改造是否有效：把本地能力从上游高频变更文件中收口到稳定扩展边界，并用契约测试保护中间件顺序、设置更新语义、依赖注入生命周期、权限和兼容层行为。

当前重点能力包括：

| 能力域 | 典型风险边界 |
| --- | --- |
| RequestArchive / RequestIntercept | Gateway 路由别名、中间件顺序、上游 path guard |
| Prompt Metrics / Prompt Risk / LLM judge | Content Moderation、Prompt Audit、fail-open/block 语义 |
| Token Analysis / 组织用量 / 子管理员 | Admin 路由、权限矩阵、DTO 和前端入口 |
| OpenAI-compatible cache usage / 默认 reasoning effort | Responses、Chat bridge、usage 解析和计费 |
| 用户并发 preset / quota flusher | Wire provider、启动、cleanup、后台任务生命周期 |

不属于本标准效果分的事项仍需记录，但不能混入方案 A 的收益：

- 上游自身 bug、migration 存量数据问题和原有基线红；
- 依赖下载、Windows `.test.exe` 锁等环境等待；
- 纯文档整理、格式化和无语义的生成文件变化；
- 与本轮上游同步无关的功能开发或技术债修复。

---

## 二、历史基线 B0

### 2.1 已确认的量化基线

基线取 `docs/features/sub2api -merage-list.md` 中 `v0.1.159` 至 `v0.1.173` 的 12 轮记录，回填条目不重复计算。

| 指标 | B0 | 说明 |
| --- | ---: | --- |
| 合并轮数 | 12 | `v0.1.159`、`160`、`161`、`162`、`163`、`164`、`165`、`166`、`168`、`170`、`171`、`173` |
| 原始冲突文件项 | 65 | 平均 `5.42` 个/轮 |
| 生产源码冲突 | 44 | 不含测试、生成文件和文档 |
| 测试冲突 | 8 | `_test.go`、`*.spec.ts`、`__tests__` |
| 生成文件冲突 | 7 | 均为 `backend/cmd/server/wire_gen.go` |
| 文档冲突 | 6 | `docs/`、`llm-wiki/`、README |

高频热点：

| 路径 | 12 轮出现次数 | 当前解释 |
| --- | ---: | --- |
| `backend/cmd/server/wire_gen.go` | 7 | 生成图反复承受本地 provider/cleanup 与上游 provider 的并集 |
| `backend/internal/server/routes/gateway.go` | 6 | 本地中间件直接插入上游路由组装入口 |
| `backend/internal/server/routes/gateway_test.go` | 5 | 本地顺序契约和上游新增路由共享同一测试文件 |
| `backend/internal/server/router.go` | 4 | 本地管理能力和上游路由注册共享入口 |
| `backend/internal/server/http.go` | 3 | 后台任务和服务启动依赖持续叠加 |

### 2.2 基线限制

历史账本记录了冲突文件，但没有统一记录“有效语义决策点”和“人工处理分钟数”。实施任何边界改造前，必须从最近三轮代表性合并记录中回填一次语义基线：

| 样本 | 选择原因 |
| --- | --- |
| `v0.1.166` | 覆盖 settings 省略字段、Admin 路由和前端设置合同 |
| `v0.1.170` | 覆盖 Gateway guard、Content Moderation、Wire 和兼容桥 |
| `v0.1.173` | 覆盖 Wire/cleanup、Gateway、settings 和 OpenAI usage/reasoning |

回填只能使用当时的 merge ledger、review、冲突文件和提交证据，不得凭记忆估算工时。无法证明的工时写 `unknown`，不能填零。

---

## 三、指标定义

### 3.1 核心指标

| 缩写 | 指标 | 计算方式 | 用途 |
| --- | --- | --- | --- |
| `RCF` | 原始冲突文件数 | merge 后首次执行 `git diff --name-only --diff-filter=U` 的路径数 | 保留 Git 层原始事实 |
| `SCF` | 生产源码冲突数 | `RCF` 排除测试、文档和生成文件 | 衡量需要处理的生产代码表面积 |
| `GCF` | 生成文件冲突数 | `RCF` 中可由正式生成命令重建的文件数 | 识别应机械处理的噪声 |
| `UCP` | 上游变更路径数 | merge base 到 exact upstream SHA 的 changed paths | 说明上游增量总体体量 |
| `OP` | 双方重叠路径数 | merge base 到本地、merge base 到上游两组 changed paths 的交集 | 校正不同上游版本体量差异 |
| `HDA` | 热点域触及数 | 本轮上游生产源码触及 3.4 定义的治理热点域数量 | 判断样本是否真正考验方案 A |
| `SDP` | 有效语义决策点 | 按 3.2 规则人工登记并复核 | **方案 A 的第一核心效果指标** |
| `RSD` | 重复语义决策点 | 本轮 `SDP` 与历史记录具有相同合同 ID 和相同原因 | 衡量是否真正消除了反复判断 |
| `ARM` | 人工有效处理分钟数 | 从开始读冲突到语义解决完成的主动工作时间 | **方案 A 的第二核心效果指标** |
| `CPR` | 契约通过率 | 通过契约数 / 应执行契约数 | 证明本地能力未丢失 |
| `PRF` | 合并后恢复文件数 | 首次完整验证后，为找回本地旧能力而额外修改的文件数 | 捕获“Git 自动合并但语义丢失” |

辅助归一化指标：

- 语义决策率：`SDR = SDP / max(OP, 1)`；
- 重复决策率：`RSR = RSD / max(SDP, 1)`；
- 人工处理率：`AMR = ARM / max(OP, 1)`；
- 冲突源码率：`SCR = SCF / max(OP, 1)`。

不能只展示百分比；每次报告必须同时展示分子、分母和绝对值。

### 3.2 一个 SDP 的认定规则

满足以下条件时计为一个有效语义决策点：

1. 不能仅采用完整 upstream blob、运行生成命令或机械格式化完成；
2. 必须理解本地合同与上游新语义，才能决定顺序、调用、字段映射、生命周期或权限；
3. 决策结果可能改变运行时行为、公开 API、数据合同、安全边界或后台任务可靠性；
4. 有唯一决策 ID、关联合同 ID、文件/符号和简短理由。

计数示例：

| 场景 | 是否计 SDP | 原因 |
| --- | --- | --- |
| 决定 `guardResponsesSubpath` 必须位于 `RequestIntercept` 前 | 是 | 属于安全边界和中间件顺序 |
| Wire 冲突后删除生成文件并运行 `go generate ./cmd/server` | 否 | 机械生成，不需要业务判断 |
| 决定新增 runner 是否必须加入 cleanup 并按何顺序停止 | 是 | 影响后台任务生命周期和数据完整性 |
| 合并 ledger 两段文字 | 否 | 文档编辑，不影响运行时合同 |
| 上游接口加参数，本地测试 stub 机械补参数 | 否 | 只有编译适配，没有业务选择 |
| compatible cache helper 应填缺失值还是覆盖上游值 | 是 | 影响 usage 和计费合同 |

同一个语义决定同时修改多个文件，原则上只计一个 SDP；同一文件中有两个互不依赖的业务决定，则计两个。无法给出合同 ID 和理由的条目不得计入或排除，必须标记为待复核。

### 3.3 ARM 计时规则

`ARM` 只统计主动分析和编辑时间：

- 开始：首次查看冲突或自动合并后的高风险 overlap；
- 结束：所有 SDP 已登记，冲突已解决，定向契约测试可开始执行；
- 排除：依赖下载、编译/测试等待、环境故障等待、用户审批等待和无关工作；
- 同时记录 wall-clock，避免通过遗漏等待掩盖流程问题；
- 控制组与实验组必须由同一人按同一范围执行，或交换顺序后再做第二次演练，降低熟悉度偏差。

### 3.4 样本规模与代表性

上游 commit 数只作元数据，不单独决定样本大小。merge commit、拆分习惯和提交粒度都会使 commit 数失真。样本分层使用 `UCP`、`OP` 和 `HDA`：

| 热点域 | 典型路径/合同 |
| --- | --- |
| Gateway ingress | `routes/gateway.go`、Archive/Guard/Intercept、`GW-*` |
| Settings | 通用 settings、namespaced settings、partial patch、`SET-*` |
| Wire / Runtime | ProviderSet、`provideCleanup`、runner/flusher、`LIFE-*` |
| Admin / Frontend | admin routes、权限矩阵、router/sidebar、`AUTH-*`、`EXT-*` |
| Compat / Safety | usage、reasoning、Prompt 安全链、`COMP-*`、`SAFE-*` |

阶段一回填第 2.2 节的三个代表性 B0 样本时，固定其 `OP` 中位数和 P75。之后在看到实验结果前按以下规则分层：

| 层级 | 规则 |
| --- | --- |
| `S1 单域` | `HDA = 1` 且 `OP` 不高于 B0 中位数 |
| `S2 多域` | 不满足 S1/S3 的其余有效样本 |
| `S3 大跨度` | `HDA >= 4`，或 `HDA >= 2` 且 `OP` 高于 B0 P75 |

只有满足以下任一条件的 exact SHA 才是代表性 A/B 样本：

1. `HDA >= 2`；
2. `HDA = 1`，但触发了 B0 中同一合同、同一原因的重复语义决策。

`HDA = 0` 只能证明安全未回归。首次回填若无法可靠恢复某轮 `OP`，该轮不参与中位数/P75 计算并标记 `unknown`，不能填零。

---

## 四、不可回归契约

方案 A 改造前必须先把下列合同变成可重复执行的测试清单。已有测试可以复用，缺口必须先补测试再重构。

| 合同 ID | 合同 | 最低断言 |
| --- | --- | --- |
| `GW-ARCHIVE-01` | RequestArchive 覆盖主路由和别名 | `/v1/responses`、root/backend alias 等目标入口均生成预期归档事件 |
| `GW-ORDER-01` | Archive 位于 Intercept 前 | 被本地拦截的请求仍能形成完整归档摘要 |
| `GW-GUARD-01` | 安全 guard 位于 Intercept 前 | 非法 Responses/Gemini 子路径不能被拦截规则短路为成功响应 |
| `GW-ROUTES-01` | 本地与上游路由并存 | 核心 OpenAI、Gemini、Grok 及本地 alias 路由不意外 404 |
| `SET-PATCH-01` | settings 省略字段保持原值 | partial payload 不把未发送字段写成零值 |
| `SET-EMPTY-01` | 显式空值语义保留 | 合同允许清空的字段在显式发送空值时确实清空 |
| `LIFE-START-01` | 本地 runner/flusher 被创建 | preset runner、quota flusher 等扩展在启用时进入运行态 |
| `LIFE-STOP-01` | cleanup 完整且幂等 | Stop/Flush 被调用，nil-safe，不在 Redis/Ent 关闭后访问基础设施 |
| `SAFE-ORDER-01` | Prompt 安全链顺序稳定 | Prompt Risk、judge、Content Moderation、Prompt Audit 的允许/拦截边界不漂移 |
| `SAFE-FAIL-01` | judge 故障固定 fail-open | 超时、非 2xx、解析失败不会扩大为生产拦截 |
| `AUTH-MATRIX-01` | 管理权限矩阵稳定 | admin、sub_admin、user 对核心 API、菜单和敏感操作的允许/拒绝一致 |
| `COMP-USAGE-01` | cache usage 为填缺失而非破坏 | cache read/write 字段及计费桶不被兼容 helper 错误覆盖 |
| `COMP-REASON-01` | reasoning effort 默认与显式值稳定 | Responses、Chat fallback、OAuth/API Key 路径保持既定优先级 |
| `EXT-ADMIN-01` | 本地管理能力入口存在 | Token Analysis、组织用量、Request Intercept、并发 preset 等路由/DTO 可用 |

契约执行规则：

1. 控制组和实验组使用同一份外部测试清单、相同 fixtures 和相同命令；
2. 不能只调用实验组新抽象的内部函数，应尽量从 HTTP、公开 service 接口或启动/cleanup 边界验证；
3. 每个合同必须能单独运行，并能映射到失败的能力域；
4. `CPR` 必须为 `100%`。环境导致未执行时结果是 `unknown`，不是通过；
5. 全量既有测试仍照常执行，契约测试不能替代项目原有 unit、integration、frontend 和 build 门禁。

### 4.1 2026-08-12 首次执行与 2026-08-13 补齐结果

阶段 0 固定在 `github/main@8784a4084268b532ab653774c0dc3999e24ff7c9`。首次实测为 `11 pass / 3 partial`、`CPR=78.57%`；补齐三个合同并修复 RED 暴露的生命周期缺陷后，权威矩阵为 `97/97` 顶层测试通过、`14 pass / 0 partial`、`CPR=100%`。

| 合同 | 初测 | 补齐后证据 |
| --- | --- | --- |
| `GW-ARCHIVE-01` | `partial` | `pass`：三个 Responses 入口均生成配对归档并归一 endpoint |
| `LIFE-START-01` | `partial` | `pass`：四类当前 Provider/公开入口覆盖启用、禁用、重复 Start、Stop 后拒绝新任务及 quota 并发启停 |
| `LIFE-STOP-01` | `partial` | `pass`：最终 flush 一次，disabled 状态仍 flush，发生在 Redis/Ent Close 前；重复 cleanup 无副作用，调度注册/取消无遗留并等待在途 tick |

详细命令、97 个已执行测试、RED/GREEN 证据和逐合同映射见 `docs/features/upstream-fork-governance-validation-report-cn.md`。当前只允许开始第一个独立结构阶段 Gateway Extension；这次结果仍只是 `baseline`，不代表方案 A 已产生效果，也不满足 TD-009 `mitigated` 条件。

---

## 五、测试方法

### 5.1 阶段一：改造前等价基线

在方案 A 代码改造前冻结：

- 本地基线 SHA；
- 精确 upstream SHA 和 merge base；
- 不可回归契约清单及通过结果；
- 本地能力清单；
- 最近三轮历史 `SDP` / `RSD` 回填结果；
- Git、Go、Node/pnpm 版本和测试命令。

如果改造前合同本身失败，应先区分已有基线红和测试缺陷。不得为了得到全绿而改变业务合同。

### 5.2 阶段二：同一上游 SHA 的控制 A/B 演练

方案 A 各结构阶段完成后，等待第一个符合 3.4 的代表性、尚未合入的 upstream exact SHA，并建立两个隔离 worktree：

| 分组 | 本地起点 | 用途 |
| --- | --- | --- |
| 控制组 C | 方案 A 改造前冻结的本地基线 | 测量原结构处理同一上游增量的成本 |
| 实验组 E | 同一业务基线 + 方案 A 边界改造 | 测量新结构处理相同增量的成本 |

两组必须满足：

1. 上游目标固定为同一个 exact SHA，fetch 后不再移动边界；
2. merge 前契约均为 `100%`，除方案 A 的结构改造外业务行为等价；
3. 使用 `git merge --no-commit --no-ff <exact-sha>`；
4. 关闭或清空 `rerere` 对本轮的影响，避免历史自动解法污染对比；
5. merge 刚停止时立即保存 `RCF`、分类和 `OP`，解决后不能倒填；
6. 两组采用相同的“保留本地能力、吸收上游有效变化、不修无关问题”范围；
7. 分别记录 `SDP`、`RSD`、`ARM`、`PRF` 和完整验证结果；
8. 控制组只用于测量，不合入 main，不产生发布物。

A/B 不是每次上游同步的固定动作。完成一次有效的代表性 A/B 后，只有出现以下情况才重做：接缝合同发生结构性变化、上次实验被污染/无效，或团队主动校准新一代治理方案。其他真实同步直接进入趋势观察。

### 5.3 阶段三：连续三轮真实同步

A/B 演练达到 `effective` 后，继续观察实验结构的三次连续真实上游同步。不能挑选冲突少的版本跳过统计；若一次同步跨多个 upstream 版本，仍按一次固定 SHA 合并记录，同时注明跨度、commit 数、changed paths 和 `OP`。

三轮期内每次都必须执行完整契约和项目既有验证，并把指标追加到同一趋势表。第三轮完成后才能把 `TD-009` 从 `open/doing` 评为 `mitigated`。

---

## 六、通过标准

### 6.1 硬门槛：任一失败即不通过

| ID | 门槛 |
| --- | --- |
| `G-01` | 改造前后、合并前后 `CPR = 100%`；无 `unknown` |
| `G-02` | 本地能力清单零非预期删除，`PRF = 0` |
| `G-03` | 生成文件只通过正式命令生成；`wire_gen.go` 人工语义编辑次数为 0 |
| `G-04` | 无未解决文件、无冲突标记，`git diff --check` 通过 |
| `G-05` | 上游/既有基线问题单独归因，不借合并分支修复或掩盖 |
| `G-06` | 控制组与实验组使用相同 exact upstream SHA、合同清单和验证范围 |

### 6.2 首次 A/B 效果门槛

实验组相对控制组必须同时满足：

| 指标 | 通过线 |
| --- | ---: |
| `SDP` | 至少下降 `50%`；取消不随样本规模变化的绝对 `<=3` 限制 |
| `SDR` | 至少下降 `40%`，并同时展示 `SDP / OP` 分子分母 |
| Gateway / settings / Wire-cleanup 热点 `SDP` | 合计至少下降 `60%` |
| `RSD` | 至少下降 `60%` |
| `ARM` | 至少下降 `40%` |
| 生成文件人工处理 | `0` |
| `CPR` / `PRF` | `100%` / `0` |

`SCF`、`SCR` 继续完整记录但不设硬降幅。Git 可能因相邻行仍报文本冲突；只要处理已机械化且 `SDP`、`RSD`、`ARM` 达标，不因 `SCF` 单项判失败。

效果评价与债务状态分开：

| 效果等级 | 判定 |
| --- | --- |
| `baseline` | 样本不具代表性，或仅证明契约未回归 |
| `improving` | 6.1 全部通过；`SDP` 至少下降 30%、`SDR` 至少下降 20%、`ARM` 至少下降 20%，但未满足 6.2 完整效果线 |
| `effective` | 6.1 与 6.2 全部通过 |

`improving` 只表示方向有效，不能进入 `mitigated` 判定。若上游增量没有触及治理热点，本轮为 `baseline`，需等待下一个代表性 exact SHA。

### 6.3 连续三轮最终门槛

| 指标 | 三轮判定 |
| --- | --- |
| 契约与恢复 | 三轮均 `CPR = 100%` 且 `PRF = 0` |
| 语义决策 | 三轮同时展示绝对 `SDP`；`SDR` 中位数不高于 A/B 控制组 `SDR_C * 60%`，且不高于实验组 `SDR_E * 120%` |
| 重复判断 | A/B 控制组与 B0 已知的同合同、同原因重复决策在三轮中不得再次出现；新出现的 RSD 不得连续两轮重复；同时展示每轮 `RSD` 与 `RSR` |
| 热点 | Gateway、settings、Wire-cleanup 至少两类不再需要原有重复语义决策 |
| 人工时间 | 三轮 `ARM` 无 `unknown`；`AMR` 中位数不高于 A/B 控制组 `AMR_C * 60%`，且不高于实验组 `AMR_E * 120%`；单轮 2 小时只作运营目标 |
| 生成代码 | 三轮均不手改 `wire_gen.go`，生成一致性检查通过 |
| 验证 | 三轮均完成规定的定向与完整验证；环境跳过不算通过 |

只有代表性 A/B 已为 `effective`，且随后连续三轮真实同步满足硬门槛和三轮最终门槛，债务状态才改为 `mitigated`，不是 `done`。只有本地 fork 被取消、能力完全上游化，或上游长期不再作为合并来源时，才重新讨论 `done`。

---

## 七、失败归因

| 现象 | 归因 | 是否判方案 A 失败 |
| --- | --- | --- |
| 控制组和实验组同一测试同样失败 | 上游或既有基线问题 | 不直接判失败，但该门禁仍是未完成 |
| 只有实验组合同失败 | 边界改造回归 | 是 |
| Git 冲突数未降，但全部可由生成/注册表工具机械解决 | 文本布局仍重叠 | 看 `SDP`、`ARM`；不能仅凭 RCF 判失败 |
| Git 自动合并成功，完整验证后发现本地能力丢失 | 隐性语义回归 | 是，且 `PRF > 0` |
| 上游没有触及治理热点 | 样本无代表性 | 本轮只计安全观察，不计效果验收 |
| Windows 文件锁或网络导致测试未运行 | 环境问题 | 结果为 `unknown`，修复环境后重跑 |
| 实验组顺手修了控制组没有的无关 bug | 试验污染 | 本轮 A/B 无效，重新建立等价样本 |

---

## 八、每轮证据模板

### 8.1 运行元数据

| 字段 | 控制组 C | 实验组 E |
| --- | --- | --- |
| local SHA |  |  |
| upstream SHA / VERSION |  |  |
| merge base |  |  |
| upstream commits / changed paths |  |  |
| `UCP` |  |  |
| `OP` |  |  |
| `HDA` / 样本层级 |  |  |
| Git / Go / Node / pnpm |  |  |
| 操作者与日期 |  |  |

### 8.2 指标结果

| 指标 | 控制组 C | 实验组 E | 变化 | 是否达标 |
| --- | ---: | ---: | ---: | --- |
| `RCF` |  |  |  |  |
| `SCF` |  |  |  |  |
| `SCR` |  |  |  |  |
| `GCF` |  |  |  |  |
| `SDP` |  |  |  |  |
| `SDR` |  |  |  |  |
| `RSD` |  |  |  |  |
| `ARM` |  |  |  |  |
| `AMR` |  |  |  |  |
| `CPR` |  |  |  |  |
| `PRF` |  |  |  |  |

### 8.3 语义决策记录

| 决策 ID | 合同 ID | 文件/符号 | 控制/实验 | 原因 | 是否历史重复 | 处理结论 |
| --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |

### 8.4 验证结果

| 验证层 | 命令或动作 | 结果 | 失败归因 |
| --- | --- | --- | --- |
| 合并完整性 | unresolved scan、marker scan、`git diff --check` |  |  |
| 生成一致性 | Wire/Ent 正式生成命令后工作树检查 |  |  |
| 本地契约 | 按合同 ID 执行 |  |  |
| 后端 | 合并范围定向测试 + 项目规定完整测试 |  |  |
| 前端 | typecheck、Vitest、build |  |  |
| 其他 | migration、脚本或平台专项验证 |  |  |

---

## 九、防止指标失真

以下做法禁止用于宣称方案 A 有效：

1. 通过把本地代码整体搬进一个大文件，只降低文本冲突但不降低耦合；
2. 只统计 `RCF`，不登记自动合并后的语义复核与 `PRF`；
3. 把生成文件、文档冲突从原始数字中删除而不保留分类前总数；
4. 在实验组减少测试范围、关闭功能或放宽断言；
5. 选择对实验组有利的上游 SHA，或控制组与实验组使用不同边界；
6. 把上游 bug 修复、无关重构带来的收益计入方案 A；
7. 环境未执行、测试跳过或人工未复核时填写 PASS；
8. 只做一次低冲突合并就把 `TD-009` 标记为 mitigated。

---

## 十、标准维护

- 指标定义或阈值变更必须在新一轮测试开始前完成，不能看到结果后改通过线；
- 首次 A/B 演练必须同时输出 `1.0` 和 `1.1` 口径，作为阈值升级的可审计对照；不得看到结果后再改变 v1.1 通过线；
- 每次真实上游合并仍按项目规则追加 `docs/features/sub2api -merage-list.md`；
- 本标准只定义效果验证。具体扩展边界、目录、接口和迁移顺序应另写设计文档，经评审后实施。

---

## 十一、外部评审（Grok，2026-08-12）

| 字段 | 内容 |
| --- | --- |
| 评审者 | Grok |
| 日期 | 2026-08-12 |
| 对象 | 本文（效果测试与验收标准 v1.0），非方案 A 实现设计 |
| 结论 | **保留为 TD-009 的度量/验收附件**；降级为「先证明有效的尺子」，**不能**代替设计文档或安全/发布止血 |

### 11.1 优点（建议保留）

1. 核心指标正确：`SDP`（语义决策）+ `ARM`（有效人工分钟），避免只数 Git 冲突（`RCF`）
2. 反作弊条款完整（搬大文件降冲突、缩测试、挑软 SHA、实验组修无关 bug 等）
3. 硬门槛与项目合并纪律一致：`CPR=100%`、`PRF=0`、不手改 `wire_gen`
4. B0 用 merge-list 12 轮量化热点（`wire_gen`×7、`gateway.go`×6 等）有账本依据
5. 合同 ID（`GW-ORDER-01` 等）可直接长成契约测试清单骨架

### 11.2 对 v1.0 的问题与风险（历史评审原意）

| 问题 | 说明 |
| --- | --- |
| 缺方案 A 本体 | v1.0 当时只有「怎么证明有效」，没有「改成什么样」；v1.1 已关联独立设计候选，但仍须用户批准后才能进入实施计划 |
| 通过线可能过严 | 首次 A/B：`SDP` 降 50% 且 ≤3、热点降 60%、`ARM` 降 40%。大跨度 upstream（数十～上百 commit）可能客观做不到 `SDP≤3`，导致永远无法 `mitigated` |
| A/B 成本极高 | 同人、双 worktree、关 rerere、完整契约——科学但贵；须约定「每 N 次 upstream 或代表性 SHA 做一次」，非每次合并 |
| 契约测试多半未齐 | 改造前要求 `CPR=100%`；补齐 14 类合同本身是中等工程，应**单独立项**，勿藏在「治理改造」里 |
| 历史 SDP 回填 | ledger 无统一 SDP/工时；回填易事后叙事——坚持无法证明写 `unknown`，禁止填零 |

### 11.3 v1.1 采纳决议

| 评审意见 | 决议 | v1.1 落点 |
| --- | --- | --- |
| 增加中间等级 | 部分采纳 | 新增 `baseline / improving / effective` 效果等级；不降低 `mitigated` 债务状态门槛 |
| 取消绝对 `SDP<=3` | 采纳 | 以 `SDP`、`SDR` 相对降幅为主 |
| 按上游跨度分桶 | 校准后采纳 | 不只数 commit；改用 `UCP + OP + HDA` 和 B0 分位数分层 |
| 先契约包、后重构 | 采纳 | 14 类合同是阶段 0 独立交付 |
| 另写实现设计 | 采纳 | `upstream-fork-governance-design-cn.md` |
| 限制 A/B 频率 | 采纳 | 仅代表性 SHA 首次验证；接缝结构变化或试验无效时重做 |
| `SCF` 降 50% | 校准后取消硬门槛 | `SCF/SCR` 保留为解释指标，核心看 `SDP/RSD/ARM` |

### 11.4 与全局还债的关系

- **不要**用本标准的 A/B 排期挤占 `CX-TD-001`（RDB）、`CX-TD-002` 族（发布）或 Grok `TD-001`/`TD-027`（业务 P0）
- TD-009 标 `mitigated` 必须同时满足：代表性 A/B 为 `effective` + 本标准硬门槛 + 连续三轮观察门槛 + 设计已落地且契约包存在
- 详细交叉决议见 `docs/features/codex-full-repo-technical-debt-audit-cn.md` 第十一节

### 11.5 评审变更记录

| 日期 | 说明 |
| --- | --- |
| 2026-08-12 | Grok 评审入库；定位为度量附件；给出 v1.1 建议但未改 1.0 通过线 |
| 2026-08-12 | 升级 v1.1：取消绝对 SDP 与 SCF 硬线；新增 UCP/HDA 分层和效果等级；锁定代表性 A/B 触发条件；关联正式方案 A 设计 |
