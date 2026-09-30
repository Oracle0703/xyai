# 交付报告

## 摘要

| 字段 | 内容 |
|---|---|
| 结果 | 0.1.173 合并候选快照已完成并暂存，可进入 merge commit 前用户审核。 |
| 日期 | 2026-08-09 |
| 状态 | 等待用户审核；尚未 commit 或 push |

## 已交付变更

| 区域 | 变更 |
|---|---|
| Git | 在 `feature/hy/10173_merge_sub2api_173` 合入固定上游 `48eb3766...`，保留未提交 merge。 |
| 冲突 | 9 个文本冲突按语义并集解决，未合并索引清零。 |
| 本地能力 | 23 个 `docs/features/` 文件及对应本地功能边界已确认保留。 |
| 上游能力 | 合入 Channel Monitor V2、Grok Voice/Realtime/Video/Web Search、Redis OAuth session、响应模型审计、邮箱主域额度和新计费配置。 |
| 自动合并适配 | 两个 GroupsView 测试删除重复 `getLiveCapability` mock；测试 stub 仅对齐上游新增接口，未改生产逻辑。 |
| 文档 | 更新六页 llm-wiki、Wiki 图谱、append-only 合并 ledger 和本交付目录。 |
| 审查 | 完成重叠路径、上游风险、冲突语义、Wiki 和最终代码五轮独立只读审查；最终 0 Critical / 0 Important / 0 Minor。 |

## 验证证据

| 检查 | 结果 | 证据 |
|---|---|---|
| Git 基线 | 通过 | `main` 与 `github/main` 同为 `ddbb0426...`。 |
| 冲突状态 | 通过 | 无 unmerged index 或冲突 marker。 |
| Focused Go tests | 通过 | config/routes/cmd server/service。 |
| Backend default / integration | 通过 | 两组全量测试均退出 0。 |
| Backend unit | 仅既有失败 | 仅第一父既有 `/auth/me admin_permissions:null` fixture；不在本次修复。 |
| Backend build / lint | 通过或已归因 | 两种 build 通过；增量 lint 0 issues，全量 28 项为第一父既有。 |
| Frontend | 通过 | lint、typecheck、246/246 Vitest files、1694/1694 tests、production build 均通过。 |
| Wiki 图谱 | 通过 | 33 nodes / 67 edges、49 wikilinks、0 unresolved，source hash 匹配。 |
| 最终 Git 审计 | 通过（含固定上游例外） | 508 个 staged 路径，0 unstaged/untracked/unmerged，23/23 features；排除固定上游文档 3 处尾随空格后 diff check 通过。 |

## 重要文件

| 文件 | 用途 |
|---|---|
| `requirements.md` | 用户目标、边界与成功标准。 |
| `specs.md` | 合并合同和风险处理方式。 |
| `plan.md` | 执行与验证清单。 |
| `test-review.md` | 审查发现和命令证据。 |
| `docs/features/sub2api -merage-list.md` | 已按追加规则写入本轮上游合并记录。 |
| `llm-wiki/wiki/*.md` | 0.1.173 稳定架构、前端、配置、数据和安全事实。 |
| `llm-wiki/.understand-anything/knowledge-graph.json` | 刷新后的 Wiki 导航图谱。 |

## 已知限制

| 限制 / 跳过的检查 | 原因 | 影响 |
|---|---|---|
| merge commit SHA 尚不存在 | 用户要求提交前审核。 | ledger 已写“待审核”；获批并提交后需追加回填最终 SHA。 |
| migration 206/220 | 固定上游存在隐私默认值和旧 checksum 数据恢复风险。 | 本次按要求不修；部署前评估数据并等待上游修复。 |
| 固定上游文档尾随空格 | `docs/channel-monitor-v2-safe-defaults.md` 第 3、4、50 行来自固定上游。 | 保持 blob 一致；全量 diff check 会仅因此非零。 |
| `go mod tidy -diff` | 固定上游 `go.sum` 仍含 5 组 CLI 传递 checksum。 | 按“不修上游 bug”边界原样保留。 |

## 后续动作

| 优先级 | 动作 | 负责人 |
|---|---|---|
| P0 | 审核待提交快照并决定是否创建 merge commit。 | 用户 |
| P1 | 部署前评估上游 migration 206/220 风险。 | 项目维护者 |
