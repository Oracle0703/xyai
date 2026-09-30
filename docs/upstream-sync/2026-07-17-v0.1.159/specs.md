# sub2api v0.1.159 上游同步规格

## 摘要

| 字段 | 内容 |
| --- | --- |
| 本地第一父 | `b5c9726262ca65bddc8ca4bc5a35ed26e1208cb3` |
| 上游第二父 | `c2c19a7cbe8486ebb5b56834d1a6e07b3f12cffc` |
| Merge base | `d515c3045ce838976ebedab87846aaaf893dbbf6` |
| 上游增量 | 114 commits、399 files、`+28829/-1666` |
| 当前状态 | 10 个冲突已解决；未创建 merge commit |

## 目标行为

| ID | 行为 | 影响 |
| --- | --- | --- |
| TB-1 | 上游 Audit、StepUp、异步图片、billing probe、批量限额及 Grok/OpenAI 修复与本地功能并存。 | 吸收上游且不丢本地业务入口。 |
| TB-2 | 10 个冲突文件组合本地功能与上游代码；`wire_gen.go` 从最终 provider 源图重新生成。 | 冲突解决不丢双方入口。 |
| TB-3 | 仅在本地用户并发 preset 测试桩中补齐上游新增的 `BatchUpdateLimits` 接口；不修改上游批量限额生产实现。 | 本地功能继续编译，同时保持远端代码原样。 |
| TB-4 | `concurrency=0` 继续表示不限并发。 | 保持上游与本地 preset 合同。 |
| TB-5 | 测试、审查和文档完成后才通知 commit 审核。 | 保证审核快照可复查。 |

## 合并决策

| 情况 | 决策 |
| --- | --- |
| 仅本地存在 | 保留生产入口、DI、前端入口和测试。 |
| 仅上游新增 | 采用上游并更新 durable wiki。 |
| 双方互补 | 手工组合调用链、中间件顺序和数据合同。 |
| 完整重叠 | 证明上游覆盖后删除重复实现。 |
| Wire 冲突 | 不选 ours/theirs，从合并后源图重新生成。 |

## 数据与接口契约

| 区域 | 必需规则 |
| --- | --- |
| 管理端批量用户限额 | 沿用上游 `POST /api/v1/admin/users/batch-limits` 合同；`all=true` 优先并忽略同时传入的 `user_ids`，0 并发为不限。上游实现风险只记录、不在本分支修补。 |
| API Key / 用户缓存 | 沿用 `MERGE_HEAD` 实现，不增加本地批量删除或错误传播逻辑。 |
| Prompt Metrics routes | 保留本地已有管理入口、payload 和合规 guard，不额外改造上游审计链。 |
| migrations `177..181` | 只新增 migration；Ent schema 与生成代码一致。 |

## 失败模式与验证

| 场景 | 处理 | 证据 |
| --- | --- | --- |
| 上游继续前进 | 收尾前 fetch；SHA 改变则重新评估。 | `MERGE_HEAD` 与 `upstream/main` |
| 发现上游原生缺陷 | 在 review/ledger 中标明远端路径与风险，等待 upstream 修复；不得混入本地 merge patch。 | 相关文件与 `MERGE_HEAD` 一致性检查 |
| Wire 漂移 | 修复 provider 源图并连续生成两次。 | SHA-256 与 Git diff |
| Windows `.test.exe` 锁/360 拒绝 | fresh repo-local cache、fresh `GOTMPDIR`、`-p 1 -count=1`；仍失败则披露。 | 命令与真实退出码 |
| 文档被 ignore | 仅对本次 delivery/review 使用 `git add -f`。 | staged 清单 |
