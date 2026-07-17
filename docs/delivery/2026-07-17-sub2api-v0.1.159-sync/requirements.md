# sub2api v0.1.159 上游同步需求

## 目标

在 `feature/hy/10157_同步sub2api主线` 上合入 `Wei-Shaw/sub2api` 最新 `main`，仔细解决冲突；保留 `docs/features/` 对应的本地独有功能，完整重叠时优先采用上游实现；同步 wiki 和追加型合并记录，完成测试后停在 commit 前供用户审核。

## 成功标准

| ID | 标准 | 证据 |
| --- | --- | --- |
| SC-1 | 第二父为收尾时最新 `upstream/main`，当前为 `c2c19a7cb`。 | 最终 fetch 与 `MERGE_HEAD` 比较 |
| SC-2 | 10 个冲突完成语义组合，无 unmerged index 或 marker。 | 冲突清单、`git ls-files -u`、marker 扫描 |
| SC-3 | 本地独有 features 仍有生产入口和测试锚点；完整重叠才采用上游替代。 | 功能矩阵、定向测试、`docs/features` 删除门禁 |
| SC-4 | 上游原生问题与本地合并回归严格区分；上游缺陷只记录并等待远端修复，不在本分支改写。 | 与 `MERGE_HEAD` / merge-tree 的逐文件对照和 merge review |
| SC-5 | wiki、merge review 和追加型 ledger 反映真实边界、冲突和验证。 | 文档 diff、EOF 追加、LF/BOM/敏感信息检查 |
| SC-6 | 后端、前端、Wire 和生产构建完成；环境失败如实记录。 | `test-review.md` 的命令、退出码与统计 |
| SC-7 | 用户审核前不 commit、push、PR 或部署。 | `MERGE_HEAD`、HEAD 与 status |

## 范围与约束

| 类型 | 内容 |
| --- | --- |
| 包含 | 最新上游增量、冲突/交集审查、本地功能保留或去重、必要的本地接口兼容、wiki/review/delivery/ledger、完整验证。 |
| 不包含 | 未经审核 commit；push、PR、部署；修改安全软件；清理无关历史债务。 |
| Git | 使用 `git merge --no-commit --no-ff`，不得整仓接受 ours/theirs。 |
| 生成代码 | Wire 必须从合并后的 `wire.go`/ProviderSet 生成并连续验证稳定。 |
| 文档 | `docs/features/sub2api -merage-list.md` 只能 EOF 追加；wiki 使用 LF，不写凭据。 |
| 语义 | `concurrency=0` 继续表示不限并发。 |

## 假设

“features 文件”指 `docs/features/` 沉淀的本地能力及其实现。代码版本以 `backend/cmd/server/VERSION=0.1.159` 为准；最近 tag 描述仍为 `v0.1.157-33-gc2c19a7cb`，仅表示该提交尚无 0.1.159 tag。

## 待确认问题

无阻塞性产品问题。唯一固定检查点是完整验证后的 commit 前审核。
