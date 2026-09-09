# 交付报告

## 摘要

已形成基于 `main@f6bce5db`、第二父固定为 `98d86915becae9fe9491a91ffc6defd5235c8d2b` 的未提交 Sub2API 0.2.4 merge 快照，等待用户审核 commit。

## 已交付变更

| 交付项 | 结果 |
|---|---|
| Git | 创建 `feature/hy/10204_merge_sub2api_204`；未提交 merge 的 `MERGE_HEAD` 精确等于目标 SHA。 |
| 上游能力 | GPT Image 2.5、可配置 OAuth 生图主控、图片输入 token usage、fallback pricing、Compose 透传与前端候选。 |
| 冲突 | 无文本冲突；2 个双方修改路径完成三方语义审查，17 个仅上游路径保持 exact target blob。 |
| 本地功能 | 24/24 `docs/features` 保留，独有业务能力未被 0.2.4 增量触碰。 |
| 文档 | 六页 wiki、Wiki 图谱、append-only merge ledger 与本交付记录已更新。 |

## 验证证据

| 检查 | 结果 |
|---|---|
| 本地 main 与 GitHub main | 均为 `f6bce5db1cda145f838bc8fa67b7c8dd90f6b0bd` |
| 上游目标 | `upstream/main@98d86915becae9fe9491a91ffc6defd5235c8d2b`，`VERSION=0.2.4` |
| 三方预演 | 19 个上游路径、2 个双方修改路径、无文本冲突 |
| 本地 features | 24 个 tracked 文件 |
| Go 专项 / integration | 通过；integration 全量通过 |
| Go build / lint | normal/embed build 通过；golangci-lint 0 issues |
| 前端 | lint、typecheck、focused 16/16、build 1079 modules 通过 |
| 完整套件 | Go default/unit 与 Vitest 仅保留已归因的第一父或环境失败 |
| Wiki 图谱 | 33 nodes / 67 edges，状态 READY |
| 最终 Git 快照 | 34 files / `+755/-27`；0 unstaged、0 untracked、0 unmerged，cached diff check 通过 |

## 已知限制

未修复 3 个 Windows 缺 `sh.exe` 的 Go 测试、1 个第一父 `/auth/me` golden 差异、4 个第一父前端测试问题及基线 `go.sum` tidy 差异；这些均不属于 0.2.4 冲突解决范围。

## 后续动作

用户审核当前 staged merge 快照；明确批准后才创建 merge commit。未授权 push、PR 或部署。
