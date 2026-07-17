# sub2api v0.1.156 上游同步执行计划

| 任务 | 状态 | 所有者 | 验证 |
| --- | --- | --- | --- |
| 固定本地基线、分支和上游标签目标 | 已完成 | 交付控制器 | SHA、标签、祖先关系 |
| 并行盘点上游增量、本地功能和验证入口 | 已完成 | 三个只读分析智能体 | 三份发现清单 |
| 执行 `--no-commit --no-ff` 固定 SHA 合并 | 已完成 | 交付控制器 | `MERGE_HEAD=d515c3045...` |
| 逐项解决冲突并审查自动合并 | 已完成 | 交付控制器 | 3 个冲突、29 个交集文件与网关 P2 均完成复核 |
| 收口重复功能并补充必要回归测试 | 已完成 | 交付控制器 | Prompt Risk runtime snapshot 与 UseKeyModal 回归已通过 |
| 更新 wiki、review 与追加型 ledger | 已完成 | 交付控制器 | wiki、review、ledger 与功能映射已收口 |
| 两轮 QA/代码审查与完整验证 | 已完成 | QA 智能体、交付控制器 | 无开放 P0-P3；最终 staged Git 硬门通过 |
| 通知用户进行提交前审核 | 已完成 | 交付控制器 | 保持未提交 merge 状态，等待用户确认 |
| 用户授权后创建 merge commit 并回填文档 | 已完成 | 交付控制器 | merge `b5b54af2129b`，双父顺序正确；未 push/PR/部署 |

## 回滚

用户已审核并批准创建本地 merge commit，因此 `git merge --abort` 不再适用。分支保持在 `feature/hy/10157_同步sub2api主线`；未经额外指示不删除分支、不 push、不创建 PR、不部署。

详细命令、工作目录与硬门见 `docs/superpowers/plans/2026-07-16-sub2api-v0.1.156-sync.md`。
