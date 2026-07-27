# 组织用量趋势图 · 实现自审

| 项目 | 内容 |
| --- | --- |
| 日期 | 2026-07-27 |
| 分支 | `feature/hy/org-usage-trend-chart` |
| 对照 | 设计合同、Codex 审核、分流 triage、Summary-first 漏洞修复要求 |
| 范围 | 工作树未提交实现（含 F3 最新修复） |
| 验证 | 前端 org-usage 相关 53 tests 绿；后端 OrganizationUsage 单测绿；**PG integration 本机未跑** |

## 总体结论

| 维度 | 结论 |
| --- | --- |
| 功能主链 | **基本到位**：独立 Trend API、补零 + `data_through`、双轴图、页面双请求与 as_of 协调 |
| Summary-first 漏洞 | **已按最新要求修复**，并用 deferred Promise 控序测试锁住 |
| 相对设计 DoD | **仍未完全关门**：PG integration 未实跑、分支名不合规、设计状态 Draft、提交未纳入 untracked 核心文件 |
| 是否建议现在合并 | **否**。可进入「修完残余项后提交 review」阶段，不宜宣称可合并上线 |

**一句话**：逻辑与单测质量已明显好于初版；最大剩余风险是 **integration 未实证** 与 **交付合规项（F1/F5/F6）**，另有一处 provisional 展示残留（中低）。

---

## 已通过的自检（对照分流 F1–F6）

### F3 Summary-first / 响应 canonical（重点）

当前 `loadTrend` 成功路径：

1. `response.range.as_of` 非空，否则 fail-closed  
2. 若已有 Summary canonical 且 `responseAsOf !== summaryCanonical`：  
   - **不写入**该响应  
   - 未重拉过 → 用 Summary canonical 重拉一次  
   - 已重拉过 → fail-closed  
3. 仅通过检查后才写 `trendPoints` / `trendMeta`  
4. `trendRequestedAsOf` 只用于在途 abort（`maybeReconcileTrend` 在 loading 且请求参数 ≠ canonical 时提前重拉）

测试（deferred Promise，显式 Summary 先完成）：

- request=C、Summary=C、Trend body=D → 二次请求 as_of=C；D 不展示；二次回 C 则展示  
- 二次仍 D / 缺失 → 局部错误，请求总数 2，不循环  
- Trend-first provisional 再 Summary 不对齐 → 只对齐一次  

**判定**：针对已描述的 Summary-first 漏洞，**当前实现与测试匹配要求，可过关。**

### F4 最小行为合同

| 项 | 状态 |
| --- | --- |
| View 状态机 / reconciliation | 有专项测试 |
| TrendChart 四系列 + Y 轴 | 有 |
| beginAtZero / monotone / dense radius / maxTicksLimit | 有 |
| partial + 末桶 as_of tooltip | 有 |
| 全零仍画图 | 有 |
| 主题 MutationObserver 单测 | 无（分流允许降为浏览器验收） |

**判定**：最小集合 **基本满足**；主题切换仍靠手工。

### F2 Integration 代码

`organization_usage_repo_integration_test.go` 已加：

- day 补零、`data_through` 裁剪、org 过滤  
- 与 Summary 同范围 tokens/requests 总和  
- week/month partial  
- 54 周上界  

**判定**：用例 **已写**；本自审环境 **未执行** `-tags=integration`。在未绿之前，设计 DoD 仍视为 **未关闭**。

### 主链与其它

- 独立 `GET .../trend`，无 user 维 SQL 合同单测  
- `fetchAll` 不调 trend  
- 粒度推断含首尾自然日 + 单测  
- 翻页/排序不拉 trend + 单测  
- i18n / README / wiki 有增量  
- `.gomodcache` 已 ignore，status 可读  

---

## 仍开放的问题

### R1 — Medium：Trend-first provisional 在 Summary 不对齐时，重拉前仍短暂展示旧曲线

**场景**

1. Trend 先返回 provisional `as_of=D` 并写入 points  
2. Summary 后返回 canonical `C`  
3. `maybeReconcileTrend` 发起重拉，但 **未先清空** 已展示的 D 曲线  

**对照**  
「Summary 已存在时不得保留不匹配响应」对 **响应到达瞬间** 已遵守；对 **先 provisional、后发现不匹配** 的展示窗口未清空。

**建议**  
在 `maybeReconcileTrend` 因 `storedAsOf !== summaryCanonical` 而重拉前：`clearTrendSuccessState()`（或 `failTrendLocally` 不设 error、仅清 points），避免闪一下错误快照。

**是否挡合并**：建议修，属正确性/体验；不修则需在验收中接受短暂错误曲线。

---

### R2 — High（门禁）/ 环境：PostgreSQL Trend integration 未实跑

设计仍写 PR1 必做。代码在，**证据链缺最后一环**。

**建议**：有 DSN/Docker 时跑：

```text
go test -tags=integration -p 1 -count=1 ./internal/repository -run OrganizationUsageRepositoryIntegration_Trend -v
```

失败则修 SQL/断言；通过再勾 DoD。

---

### R3 — Medium（交付）：F1 untracked 核心文件

仍可能漏提交：

- `OrganizationUsageTrendChart.vue` + spec  
- 设计文档  
- 审核/自审文档（若要入库）  

提交必须用显式路径，禁止只靠 `git add -u`。

---

### R4 — Low/Medium（交付）：F5 设计 Draft vs wiki「已描述落地」

wiki 已写 Trend 能力，设计状态仍 Draft。应按两段提交：实现后再改状态并记实现 SHA（或「已实现/待合并」）。

---

### R5 — 交付合规：F6 分支名

仍为 `feature/hy/org-usage-trend-chart`，本轮规范要求 `feature/hy/10xxx_XXX`。  
**挡最终交付合规**，不挡本地正确性。缺负责人编号。

---

### R6 — Nit

- Summary 空 `as_of` 检查未 `.trim()`，与 Trend 路径不对称。  
- `scheduleTrendReconcileIfNeeded` 在网络错误后也会重拉一次：合理恢复，但若后端持续 500 只会一次，OK。  
- Chart 测试依赖 `defineExpose`：可接受；重构时注意别拆掉。  

---

## 验证记录（本自审执行）

| 项 | 结果 |
| --- | --- |
| FE View + TrendChart + API + date helpers | **53 passed** |
| BE service/repo(unit)/handler/routes OrganizationUsage | **passed** |
| PG integration Trend | **未跑** |
| frontend production build | **未跑** |
| 干净 worktree 构建 | **未跑** |
| 浏览器当前月/周未来桶 | **未跑** |
| 366 天性能抽查 | **未跑** |

---

## 风险排序（修 / 验优先级）

| 序 | 项 | 动作 |
| --- | --- | --- |
| 1 | R2 integration 实跑 | 有环境则立刻跑并修红 |
| 2 | R1 provisional 清空 | 小补丁，建议合入前做 |
| 3 | 提交清单 F1 | 显式 add 全部 intended |
| 4 | build + 干净快照 | 合并前 |
| 5 | 浏览器 V7/V8 | 合并前 |
| 6 | F5/F6 | 文档状态 + 分支改名 |

---

## 最终 Gate（自审版）

| 问题 | 回答 |
| --- | --- |
| Summary-first 漏洞是否已按最新要求关闭？ | **是**（代码 + 控序测试） |
| 是否达到设计全文 DoD？ | **否**（integration 未实证等） |
| 是否建议现在 merge？ | **否** |
| 是否建议现在 commit 供人审？ | **可以**，在修 R1（建议）并理清 add 列表后；commit message 勿宣称 integration 已绿 |

---

## 给后续执行的最小补丁清单

1. **（建议）** `maybeReconcileTrend`：stored 与 canonical 不一致准备重拉时先 `clearTrendSuccessState()`。  
2. **（必须门禁）** 跑通 Trend PG integration。  
3. **（必须交付）** 显式 add 组件/设计/测试；忽略 cache。  
4. **（合规）** 等 `10xxx` 改分支名；实现提交后再改设计状态。  

---

*本自审不修改业务代码；仅记录结论。*
