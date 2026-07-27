# 组织用量趋势图 · Codex 审核结论分流

| 项目 | 内容 |
| --- | --- |
| 日期 | 2026-07-27 |
| 修订 | 2026-07-27（第二轮：修正 F2 降级条件、F3 并行时序、F4 最小行为合同、F5 提交顺序、F6 门禁级别，并补最终验收清单） |
| 输入 | `docs/reviews/2026-07-27-organization-usage-trend-chart-code-review.md` |
| 对照实现 | `feature/hy/org-usage-trend-chart` 工作树 + 设计 `docs/features/organization-usage-trend-chart-design-cn.md` |
| 目的 | 判断 Codex 各条 findings **要不要改、改什么、优先级**；作为执行合同，不重复审核全文 |

## 总览

| ID | Codex 定级 | 本分流结论 | 是否改业务代码 | 是否阻塞合并 / 交付 |
| --- | --- | --- | --- | --- |
| **F1** | Blocker | **要处理**（提交纳入 untracked 核心文件） | 否（Git） | **阻塞合并** |
| **F2** | High | **要补 integration，或正式改设计合同后才能降级** | 否（测试；或改设计） | **阻塞合并**（现行设计下） |
| **F3** | High | **要改**（as_of fail-closed，且保留并行 fast path） | **是** | **阻塞合并** |
| **F4** | High | **要补最小行为合同测试**（可裁剪易碎快照，不可裁行为） | 是（测试为主） | **阻塞合并** |
| **F5** | Medium | **要收尾文档**（实现提交与状态提交分离） | 否 | **交付前收尾**（不挡实现 PR 技术正确性，挡“已实现”宣称） |
| **F6** | Medium | **要合规改名**（等负责人给 `10xxx`） | 否 | **阻塞最终交付合规**（不挡功能正确性） |

**一句话**：暂不合并仍成立。F1–F4 在现行设计下均为合并前硬项；F2 **不得**仅靠 triage 口头降级；F3 修复必须保留 Summary/Trend 并行 provisional 路径；F4 可减断言数量但必须覆盖设计核心行为；F5 不能要求同一提交写入自身 SHA；F6 按本轮分支规范阻塞交付合规。

---

## 第二轮对第一版 triage 的修正（摘要）

| 点 | 第一版问题 | 修正后 |
| --- | --- | --- |
| F2 | 写了「负责人可批准 24h 后补 integration」 | **删除软降级**。二选一：补 integration 再合并，**或**负责人正式改设计合同并记风险/负责人/截止条件 |
| F3 | 「每次 Trend 成功都与 summaryCanonical 严格相等」易误读 | **分阶段**：响应必须有非空 `as_of` → Summary 未到时 provisional → Summary 到后比较 → 重拉后严格相等 fail-closed |
| F4 | 图表侧几乎只保留 loading/retry/emit | 可裁剪 options **快照**，但最小集合必须覆盖四系列/双轴零基线/monotone/密集点/maxTicksLimit/tooltip/全零仍画图 |
| F5 | 易被理解成「本提交记本 SHA」 | **两段提交**：先实现，再文档记前序实现 SHA；或先「已实现/待合并」，合入后再补 SHA |
| F6 | 「看团队规范 / 默认不挡」 | 本轮规范已明确 `feature/hy/10xxx_XXX`，**阻塞最终交付合规**；缺的是编号，不是是否遵守 |
| 验收 | 缺最终门禁清单 | 增补 build、干净快照、PG integration、浏览器未来桶、366 天性能抽查 |

---

## 逐条分析

### F1 — 核心新增文件仍 untracked

**事实**  
`OrganizationUsageTrendChart.vue` 与设计文档仍为 `??`；View 已静态 import 组件。`git add -u` 会漏文件，干净 checkout 构建失败。

| 维度 | 结论 |
| --- | --- |
| 要不要改业务逻辑 | **否** |
| 要不要处理 | **是** |
| 阻塞 | **合并 / 提交阻断** |

**动作**：提交 checklist 显式 `git add` 设计文档、TrendChart 及全部 intended 路径；从暂存区或干净 worktree 再跑 typecheck/build。

**分类：需要处理 · 交付纪律 · 阻断**

---

### F2 — 缺少 PostgreSQL Trend integration

**事实**  
设计（约 779 / 830 / 924 行）将 Trend PG integration 标为 **PR1 必做 / 必测**。当前 `organization_usage_repo_integration_test.go` 无 `repo.Trend`；仅有 SQL 字符串合同 + sqlmock，不能证明桶边界与补零。

| 维度 | 结论 |
| --- | --- |
| 要不要改业务 SQL | 默认否（integration 暴露 bug 再改） |
| 要不要补测试 | **是**（现行合同下） |
| 阻塞 | **合并阻断**（现行设计下） |

#### 合法处理只有二选一（不得第三种）

1. **按现有设计补 integration，再合并**  
   至少覆盖设计矩阵：day 空桶、未来桶裁剪、跨月 week partial、month 月末、`2024-01-07..2025-01-06` 54 周上界、org/q、禁用/删除用户、Summary/Trend **同一 canonical as_of** 下 requests/total_tokens 总和一致。  
2. **负责人正式修改设计合同后再降级**  
   必须在 `organization-usage-trend-chart-design-cn.md` 中改写 DoD（取消/延后「PR1 必做」），并写明：  
   - 降级原因与风险  
   - 负责人  
   - 新门禁（例如「合并后 N 日内 / CI 具备 PG 后必须绿」）  
   - 未完成前不得宣称「后端合同完成」  

**仅在 triage 或口头说「可降级」不足以改变 DoD，也不构成合入许可。**

**分类：需要修改 · 测试 / 或正式改设计 · 现行合同下阻断合并**

---

### F3 — canonical 重拉后 `as_of` 缺失仍当成功

**事实**  
`OrganizationUsageView.vue` 成功路径未校验 `response.range.as_of`；重拉终态使用：

```ts
if (trendMeta.value?.range.as_of && trendMeta.value.range.as_of !== summaryCanonical)
```

缺失 `as_of` 时条件为 false，points 仍展示。与 Summary fail-closed 不对称，违反 K6。

| 维度 | 结论 |
| --- | --- |
| 要不要改代码 | **是** |
| 阻塞 | **合并阻断** |

#### 正确修复时序（必须保留并行 fast path）

不得把「每次 Trend 成功都立刻与 summaryCanonical 严格相等」机械套进**首次** Trend 回调——Trend 可能先于 Summary 返回。

| 阶段 | 规则 |
| --- | --- |
| 任意 Trend HTTP 成功 | **`range.as_of` 必须非空**，否则清空 points + 局部 `trendError`，不写成功态 |
| Summary 尚未建立 canonical | 允许将带非空 `as_of` 的结果写入 **provisional** Trend（并行 fast path） |
| Summary 成功且 `range.as_of` 非空 | `snapshotAsOf = canonical`；与当前 Trend（或 `trendRequestedAsOf`）比较 |
| 不一致 | 每 `reportCycleId` **最多重拉一次** Trend（带 Summary canonical） |
| 重拉之后 | **必须** `trend.range.as_of === summaryCanonical`；缺失或不同 → 清空 + 局部错误，**禁止再循环** |
| Summary 缺 `as_of` | 保持页面级 fail-closed（既有） |

回归测试至少：

1. 首次 Trend 响应缺 `as_of` → 局部错误、无 points  
2. provisional 先到、Summary canonical 不同 → 恰好一次重拉后对齐  
3. 重拉后仍缺或仍不同 → 局部错误、请求次数不再增加  

**分类：需要修改 · 业务逻辑 · 阻断 · 修复时保留并行**

---

### F4 — 前端测试矩阵不足

**事实**  
无 TrendChart spec；View mock 掉图表；状态机高风险路径覆盖不足。

#### 可以裁剪

- 完整 Chart.js options **深快照**（易碎、低收益）  
- 运行时主题 MutationObserver 的自动化（可降为**浏览器验收**，不强制单测阻塞）

#### 不可裁剪的最小行为合同（阻塞合并）

**View 状态机**

- candidate 并行双拉 Summary + Trend  
- Summary canonical 缺失 → 页面错误  
- Trend 缺 `as_of` → 局部错误  
- canonical 不同 → 只对齐一次  
- 重拉后仍不同/仍缺 → 局部错误且不循环  
- 粒度切换只打 Trend、不打 Summary  
- 人员 sort/page 不打 Trend  
- filter/完整加载 abort 在途 Trend；旧 cycle 丢弃  
- 图表区 retry；页面 retry-load 新 cycle  

**TrendChart 行为（非全量 options 快照，但是合同断言）**

- loading / error / retry  
- 粒度 emit  
- **四个默认系列**及各自 Y 轴（input/output/cache_read → 左；requests → 右）  
- 两轴 **`beginAtZero: true`**  
- **`cubicInterpolationMode: 'monotone'`**（或等价配置断言）  
- 密集点（点数 > 60）**`pointRadius === 0`**  
- x 轴 **`maxTicksLimit`** 在约定区间（如 12–16）  
- tooltip：`partial` 文案；**最后一桶**带 `range.as_of`  
- **全零数据仍渲染图表**（有 axes / chart 容器，不是 empty-only）  

说明：这是「减少易碎快照、保留设计 §842 核心行为」，不是放弃行为合同。

**分类：需要修改 · 测试 · 最小集合阻断合并**

---

### F5 — 设计 Draft vs wiki 已写落地

**事实**  
设计仍 Draft；wiki 已按落地能力描述。成熟度信号不一致。

| 维度 | 结论 |
| --- | --- |
| 要不要改业务代码 | **否** |
| 要不要处理 | **是（文档）** |

#### 可执行提交顺序（禁止「本提交写本 SHA」）

Git 无法在同一提交内容中预知并写入**自身**最终 SHA。

推荐：

1. **提交 A**：实现 + 测试 + 设计/wiki 可先标「实现中/待验收」（若 wiki 已超前，可先收回「已完成」语气）。  
2. **提交 B（或合并后）**：设计状态 →「已实现/待合并」或「已实现」，**记录提交 A 的 SHA**（或 merge commit SHA）。  

或：实现合入前设计只写「已实现/待合并」不写 SHA；合入主线后再补准确哈希。

**F1–F4 未关闭前，不要把设计改成「已实现」并暗示可上线。**

**分类：需要收尾 · 文档 · 挡「已实现」宣称，不挡修代码本身**

---

### F6 — 分支名不符合 `feature/hy/10xxx_XXX`

**事实**  
当前 `feature/hy/org-usage-trend-chart`。本轮已明确新增功能分支必须为 `feature/hy/10xxx_XXX`。

| 维度 | 结论 |
| --- | --- |
| 要不要改业务代码 | **否** |
| 缺什么 | **负责人提供 `10xxx` 编号**，不是讨论「要不要遵守规范」 |
| 阻塞 | **阻塞最终交付合规**；不阻塞功能正确性与本地修复 |

**动作**：拿到编号后 `git branch -m feature/hy/10xxx_描述`；若已推送远程，按团队流程改远程分支名。

**分类：需要处理 · 流程合规 · 交付门禁**

---

## 明确无需返工的部分

| 项 | 说明 |
| --- | --- |
| 主链路重写 | 独立 Trend API、无 user 维、`data_through`、双轴四系列、翻页不拉 Trend 等已认可 |
| F1 业务逻辑 | 仅提交纳入文件 |
| F4 全量 options 深快照 | 可裁剪；**行为合同不可裁** |
| F6 运行时行为 | 仅分支元数据 |

---

## 建议执行顺序

| 序 | 动作 | 对应 | 完成标准 |
| --- | --- | --- | --- |
| 1 | 修 Trend as_of：非空校验 + provisional + 重拉后严格相等 | **F3** | 并行 fast path 保留；缺/不等 fail-closed；每 cycle ≤1 次重拉 |
| 2 | View 状态机 + TrendChart 最小行为合同测试 | **F4** | 上文「不可裁剪」清单全绿 |
| 3 | 补 PG Trend integration **或** 正式改设计 DoD | **F2** | 二选一落地，禁止口头降级 |
| 4 | 显式 add 全部 intended 文件并做干净快照构建 | **F1** | 无依赖工作区 untracked 才能 build |
| 5 | 实现提交 → 文档状态提交（记前序 SHA） | **F5** | 设计/wiki 与真实进度一致 |
| 6 | 负责人给号后改分支名 | **F6** | `feature/hy/10xxx_*` |
| 7 | 最终验收清单（下节）全过 | Gate | 才允许宣称可合并 |

---

## 最终验收清单（合并前）

| # | 项 | 说明 |
| --- | --- | --- |
| V1 | 后端单测 | `go test` service / repository（非 integration）/ handler / routes，OrganizationUsage 相关全绿 |
| V2 | **真实 PostgreSQL Trend integration** | 除非 F2 选项 2 已正式改设计；否则必须绿 |
| V3 | 前端 Vitest | View 状态机 + TrendChart 最小行为合同 + 既有 organizationUsage 套件 |
| V4 | 前端 typecheck / **build** | `pnpm` typecheck + production build 通过 |
| V5 | **干净提交快照构建** | 仅基于已跟踪/已暂存内容的 worktree 或 `git archive`/干净 clone，确认 TrendChart 与设计文档在树内 |
| V6 | `git diff --check` | 无空白错误 |
| V7 | 浏览器：当前月 / 当前周 | **不出现未来 0 点**；筛选与组织联动正常 |
| V8 | 浏览器：人员翻页 | 趋势不闪、不无故清空 |
| V9 | **366 天 day 性能抽查** | 与设计期望同量级；若异常先查是否误走 user 维 SQL |
| V10 | 分支名合规 | `feature/hy/10xxx_*`（F6） |
| V11 | 文档状态 | 不与实现脱节（F5） |

---

## 与 Codex 最终 Gate 对齐

| 议题 | 结论 |
| --- | --- |
| 暂不合并 | **成立** |
| F1–F4 | 现行合同下均为合并前硬项（F4 指最小行为合同，非全量快照） |
| F2 降级 | **仅**正式改设计后成立 |
| F5 | 两段文档提交；不挡写代码，挡「已实现」话术 |
| F6 | **挡最终交付合规** |
| 主实现推倒重来 | **不需要** |

---

## 结论表（给执行用）

### 必须改代码 / 测试

1. **F3** — as_of 非空 + provisional 并行 + 重拉后严格相等 fail-closed  
2. **F4** — View 状态机 + TrendChart 最小行为合同（见上表）  
3. **F2** — PG integration（或正式改设计 DoD）

### 必须处理、非业务逻辑

4. **F1** — 提交纳入 untracked 核心文件 + 干净快照验证  
5. **F5** — 设计/wiki 状态与提交顺序  
6. **F6** — 负责人编号后改分支名（交付合规门禁）

### 明确不做

7. 主功能重写  
8. 仅 triage 口头降级 F2  
9. 破坏并行的「首次 Trend 必须已有 Summary canonical」  
10. 用全量 Chart.js 深快照代替行为合同  

---

*本文为审核分流与执行合同修订版，不包含业务实现 diff。落地请严格按「执行顺序」与「最终验收清单」推进。*
