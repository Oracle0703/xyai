# sub2api v0.1.159 上游同步交付报告

## 摘要

| 字段 | 内容 |
| --- | --- |
| 结果 | 最新上游已完成未提交 merge、文档、验证与 staged 门禁；等待用户审核 |
| 日期 | 2026-07-17 |
| 状态 | 等待 commit 前审核；尚未 commit、push 或创建 PR |

## 当前变更

| 区域 | 结果 |
| --- | --- |
| Git 边界 | `MERGE_HEAD=upstream/main@c2c19a7cb`，第一父保持 `b5c972626`。 |
| 冲突 | 10 个冲突完成语义组合，unmerged index 为 0。 |
| 本地功能 | RequestArchive/Intercept、Prompt Metrics/Risk、Token Analysis、组织用量、用户并发、quota flusher 等保留。 |
| 范围决定 | 上游原生 SSRF、任务恢复和批量 cache 风险仅记录并等待远端修复；本地主动修复已撤销。 |
| 测试范围决定 | 上游 locale compile 测试缺直接依赖只记录、不在本分支补依赖。 |
| 最新追加 | 0.1.159 客户端 IP 信任统一、OpenAI 账号文案、账号上游站点跳转等最新提交已保留。 |

## 当前验证

| 检查 | 结果 |
| --- | --- |
| Merge 父与冲突清零 | 通过；`MERGE_HEAD=upstream/main@c2c19a7cb`，unmerged 0 |
| 最新 0.1.159 tree 对比 | 通过；114 commits、399 files、`+28829/-1666` |
| 后端 | unit、增量 lint、embed build 通过；integration 的 5 个环境拦截包均经独立执行通过 |
| 前端 | lint、typecheck、build 通过；Vitest 188 files / 1300 tests passed，1 个上游清单 suite 无法收集 |
| Wire | 连续生成稳定，SHA-256 `92D6F616...12A61` |
| 模块 | `go mod tidy -diff` 只报告继承的旧 `go.sum` 校验项；按范围不修改 |
| wiki/review/ledger | 已更新；ledger 仅 EOF 追加 |
| 最新 upstream 复核 | 收尾 fetch 后仍为 `c2c19a7cb`，边界未移动 |
| Staged 门禁 | 416 files / `+29294 -1677`；unmerged、unstaged、untracked、marker、features 删除、未解析敏感模式均为 0 |

## 已知限制与下一步

已知限制均来自 upstream 或本机执行环境：异步图片 SSRF/任务恢复、批量限额 cache/参数上限、locale compile 测试依赖清单，以及 360 对仓库临时测试二进制的启动拦截。本分支没有修复这些问题。当前保持未提交状态，剩余动作仅为用户审核并决定是否允许创建 merge commit。
