# 设计规格

## 摘要

| 字段 | 内容 |
|---|---|
| 需求来源 | `requirements.md` |
| 交付目标 | 生成一个可审查、可测试、文档完整的 `0.1.155` 本地合并分支 |
| 主要验收标准 | 固定上游 SHA、双边功能保留、文档追加、全套验证 |

## 当前状态

| 区域 | 当前行为 / 证据 |
|---|---|
| 本地主线 | `main` 与 `github/main` 均为 `20e6379f9243f00aaf84b562af40c7b80793d4fc` |
| 上游 | `upstream/main` 为 `7c717365ef728e53cdcf6d639a4dd68226db03b2`，版本 `0.1.155` |
| 共同基线 | `55ed0ab0da367183d97c15659e33ae9e83f6ff90`，即之前固定的 `0.1.153` 上游提交 |
| 变更规模 | 上游 71 个提交、238 个文件、15089 行新增、880 行删除；双方共同修改 23 个文件 |
| 预演冲突 | `backend/internal/repository/redis.go`、`backend/internal/server/router.go`、`backend/internal/service/content_moderation.go` |

## 目标行为

| ID | 行为 | 用户 / 系统影响 |
|---|---|---|
| TB-1 | 以普通 merge 保留双亲历史，固定合入上游 SHA | 后续可准确追溯同步边界 |
| TB-2 | 冲突按语义合并，不采用整文件 `ours` 或 `theirs` | 上游与本地功能同时存在 |
| TB-3 | 对 23 个双边修改文件做逐文件审查和定向验证 | 捕获 Git 自动合并造成的静默回归 |
| TB-4 | Ent、Wire、迁移、配置样例和前端文案保持一致 | 避免生成代码、数据库和 UI 合同漂移 |
| TB-5 | 将稳定架构变化写入 wiki，将本次事实追加到 merge ledger | 后续 AI 和维护者可复用本次结论 |

## 接口

| 接口 | 变更 | 兼容性说明 |
|---|---|---|
| Git 历史 | `main` + `upstream/main` merge commit | 不改写现有历史 |
| 后端 API / 配置 / 数据库 | 接收上游 `0.1.155` 的路由、监控、计费、Grok、Ops、Server-Timing 和迁移变化 | 本地扩展继续保留，实际合同以合并后测试为准 |
| 前端 | 接收上游账号、Grok 监控、Ops 日志和请求封装变化 | 保持现有本地页面与 locale 扩展 |
| 文档 | 追加 merge ledger，更新 wiki 最近同步和受影响领域 | merge ledger 不覆盖旧条目 |

## 数据契约

| 字段 / Payload / 格式 | 必需规则 | 兼容性 / 消费方说明 |
|---|---|---|
| `backend/cmd/server/VERSION` | 最终为 `0.1.155` | 发布和管理端版本展示使用 |
| SQL migrations | 保留新增文件及顺序，不修改既有已应用迁移 | PostgreSQL 启动迁移使用 |
| Ent schema/generated | schema 与生成代码一致 | Repository、DTO 和迁移 diff 使用 |
| 配置样例 | 新字段与 `backend/internal/config` 默认值一致，本地字段不丢失 | 部署与启动使用 |

## 数据流

| 步骤 | 输入 | 处理 | 输出 |
|---|---|---|---|
| 1 | 本地 `main`、固定 `upstream/main` | Git 三方合并 | 带冲突的 index/worktree |
| 2 | base/ours/theirs | 逐块语义对照和交集审查 | 无冲突、双边功能保留的源码 |
| 3 | 合并后源码 | 生成一致性、测试、lint、typecheck、build | 可验证的本地分支 |
| 4 | commit/冲突/验证证据 | 更新 wiki 与 ledger | 可追溯文档 |

## 失败模式

| 场景 | 预期处理 | 需要的证据 |
|---|---|---|
| 显式冲突 | 对照 base/ours/theirs 和调用链逐块解决 | 冲突标记为 0、定向测试通过 |
| 自动合并静默覆盖本地逻辑 | 审查 23 个交集文件及本地关键功能搜索结果 | 交集审查记录 |
| 生成代码漂移 | 运行 Ent/Wire 生成后检查工作树 | 生成命令和 diff |
| 测试环境失败 | 区分代码失败、Docker/网络/Windows 文件锁；代码失败必须修复 | 原始命令、错误和重跑结果 |
| 文档与源码不一致 | 以源码和测试为准并同步修正文档 | 最终 wiki diff |

## 验收标准映射

| 成功标准 | 规格覆盖 | 验证方式 |
|---|---|---|
| SC-1 | TB-1 | 分支和 commit 图检查 |
| SC-2 | TB-1、数据契约 | merge parent、VERSION 检查 |
| SC-3 | TB-2、TB-3 | 冲突/交集审查、定向和全量测试 |
| SC-4 | TB-5 | ledger 追加性和 wiki diff 检查 |
| SC-5 | TB-4、失败模式 | `test-review.md` 完整验证矩阵 |
