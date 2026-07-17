# sub2api v0.1.156 上游同步规格

## 当前状态

| 项目 | 值 |
| --- | --- |
| 本地基线 | `main@4c456aad38b8f9c0b879c6d852ba87798e718439` |
| 既有上游基线 | `7c717365e`，代码内版本 `0.1.155` |
| 工作分支 | `feature/hy/10157_同步sub2api主线` |
| 上游标签 | `v0.1.156` |
| 标签目标 | `12f991dde8a58e183d4bd16a87ef6fd0df714757` |
| main 版本同步目标 | `d515c3045ce838976ebedab87846aaaf893dbbf6`，仅修改 `VERSION` 为 `0.1.156` |
| 当前上游 HEAD | `eb2b8632d`，明确排除 |

## 目标行为

1. Git 索引进入以 `d515c3045` 为 `MERGE_HEAD` 的无提交合并状态。
2. 冲突处理保留本地独有能力；存在完整上游替代时使用上游代码，避免双实现、双路由或重复配置。
3. 对 Git 自动合并的双方修改文件执行语义审查，不能把“无文本冲突”等同于“无行为冲突”。
4. 依据实际增量更新 backend、frontend、data/domain、security/reliability、ops 中受影响的稳定知识。
5. 测试与构建完成后保持无 unmerged/unstaged 修改，等待用户审核后再提交。

## 合并决策规则

| 情况 | 决策 |
| --- | --- |
| 仅本地存在、无上游等价实现 | 保留本地功能与测试。 |
| 仅上游新增 | 采用上游实现并补充必要 wiki。 |
| 双方功能互补 | 手工组合，并验证入口、数据结构、调用链和终态。 |
| 双方功能等价且上游覆盖完整 | 采用上游实现，移除本地重复实现与失效兼容层。 |
| 上游实现缺少本地安全或业务边界 | 保留本地边界，将上游能力接入该边界。 |

## 失败处理

- 出现冲突时先保存 `git ls-files -u` 和三方差异，再编辑。
- 测试失败先按错误、最近变化和调用链定位根因；环境锁使用 fresh `GOTMPDIR` 重跑，不修改业务代码规避。
- 若固定目标不在 `upstream/main` 历史或版本标签不可验证，停止合并；当前已验证通过。
- 不执行 `git merge --abort`、reset 或覆盖用户分支，除非出现不可恢复阻塞并取得授权。

## 验收证据

| 标准 | 证据 |
| --- | --- |
| AC-1/AC-2 | `git rev-parse ORIG_HEAD MERGE_HEAD`、祖先边界检查、`git log --left-right`。 |
| AC-3/AC-4 | 冲突文件清单、双方修改清单、功能映射审查、定向测试。 |
| AC-5 | wiki diff、ledger 尾部追加校验、LF/敏感信息检查。 |
| AC-6 | Go unit/integration/lint、pnpm lint/typecheck/Vitest/build、embed build 的退出码和统计。 |
| AC-7 | `git status --short --branch`、`MERGE_HEAD` 存在、无 commit。 |
