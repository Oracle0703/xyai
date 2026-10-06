# Sub2API 0.2.13 固定提交合并审核

## 边界

| 项目 | 结果 |
| --- | --- |
| 日期 | 2026-10-03 |
| 工作分支 | `feature/hy/10215_merge_sub2api_213` |
| 第一父 / 本地 main | `244b058680aa56b613a7cb0862e262ed028661ef` |
| 上游分支 / 固定目标 | `Wei-Shaw/sub2api main@b8dece9000c68815a5b867ca5a1e6f236e173905` |
| 共同祖先 | `42bc7f6cffe24bcb471608e48e66b4a0afa1f882` |
| 上游版本 / 增量 | `0.2.13`；39 commits / 157 paths / `+6027/-314` |
| 合并提交 | 尚未创建；保留 `MERGE_HEAD`，等待人工审核 |
| 范围 | 仅解决冲突；本地独有功能保留，重叠行为采用上游，不修上游 bug |

## 冲突裁决

| 文件 | 裁决 |
| --- | --- |
| `backend/internal/handler/auth_oauth_pending_flow_test.go` | 保留本地 `GetByIDWithAdminAccess` 测试适配，并加入上游 EmailCache 原子尝试/密码重置 token 桩方法。 |
| `backend/internal/handler/user_handler_test.go` | 保留本地 `GetByIDWithAdminAccess` 测试适配，并加入上游 EmailCache 原子尝试/密码重置 token 桩方法。 |
| `backend/internal/service/auth_service_email_bind_test.go` | 保留本地 `GetByIDWithAdminAccess` 测试适配，并加入上游 EmailCache 原子尝试/密码重置 token 桩方法。 |

没有改动业务实现来“修复”冲突之外的问题。TypeSafe System One、充值优惠阶梯、公共订单校验限流、邮箱验证码原子计数等重叠能力采用固定上游实现；本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk、Token Analysis、组织/部门、子管理员、并发预设和 quota flusher 等独有能力保留。

## 三方审计

- 上游独有 124 个路径的结果 blob 与固定目标逐项一致。
- 文档更新前，本地独有 682 个路径的结果 blob 与第一父逐项一致；收尾仅 wiki/图谱/台账发生本轮文档更新。
- 33 个双方修改路径已检查；关键范围包括 Ent payment order 生成物、TypeSafe 端点与平台枚举、content moderation、user service 验证码、网关路由、前端 settings/payment 和 README。逐文件比较无上下文 diff、排除行号和 blob 标识后，33/33 的“第一父→合并结果”增量等于“共同祖先→上游目标”，没有额外业务修补。
- `docs/features/` 现有 29 个 tracked 文件相对第一父无删除或改写。
- `git diff --cached --check` 已通过；未解决冲突为 0。

## 验证

| 命令 | 结果 |
| --- | --- |
| `go generate ./ent`；`go generate ./cmd/server`（连续生成） | 通过；生成物无漂移，工具额外写入的 go.sum 校验项已回退。 |
| `go test -p 1 -count=1 -v ./...` | 通过，54 个包 ok；15 个显式 skip。 |
| `go test -tags=unit -p 1 -count=1 -v ./...` | 退出 1；59 个包 ok，15 个显式 skip；`internal/service` 被第一父已有 `payment_config_plans_validation_test.go:137` 与 `gpt_quota_display_test.go:287` 的 `ptrFloat` 重复声明阻塞，未修改。 |
| `go test -tags=integration -p 1 -count=1 -v ./...` | 退出 0，54 个包 ok；19 个显式 skip，repository 包因 Docker 不可用跳过，Prompt Audit PostgreSQL DSN 未设置。 |
| `go mod tidy -diff` | 通过，无依赖漂移。 |
| normal / embed `go build` | 均通过。 |
| `pnpm.cmd run lint:check` / `typecheck` | 均通过。 |
| `pnpm.cmd install --frozen-lockfile` | 通过；将本机 Axios 1.18.1 更新为锁文件中的 1.20.0，锁文件无漂移。 |
| `pnpm.cmd run test:run` | 安装锁定依赖后全量重跑：376 files / 2897 tests；375 files / 2894 tests 通过。唯一失败为 `src/api/__tests__/settings.authSourceDefaults.spec.ts` 的 3 个断言仍要求 5 个平台，而实现含 TypeSafe 6 个。 |
| `pnpm.cmd run build` | 安装锁定依赖后重跑通过（含 typecheck）；保留 Browserslist 与 chunk size 提示。 |
| golangci-lint 2.13 `run --new-from-rev=HEAD ./...` | 通过，0 issues。初次全局旧工具因 Go 版本不匹配退出，随后使用仓库缓存 `.gocache/tools/golangci-lint.exe`（Go 1.27.0）完成。 |
| Wiki 图谱刷新 / `check-understand-status.cmd -AllowDirtyWiki` | 35 nodes / 76 edges，59 wikilinks / 0 unresolved；当前正文 hash 匹配，返回 READY。此项只确认文档导航，不替代集成验收。 |

Vitest 失败归属：该测试文件在第一父、指定上游和结果中的 blob 一致；相关平台列表与 normalize/sanitize/build helpers 和指定上游一致（本地差异仅 append fallback 与 RequestArchive API），故是目标上游增加平台后未同步测试断言。本轮未改断言、未删除用例；未另建完整上游 checkout 复跑。Go unit 两个冲突 helper 文件也逐 blob 等于第一父。

Docker、真实 PostgreSQL/Redis、外部上游和生产浏览器验收不因本地测试通过而默认视为覆盖。Go 日志与最终前端日志位于 `backend/.gocache/merge212-*.log`；首次前端日志位于仓库外 `E:/allsite/merge212-frontend-*.log`，均为本机验证产物。日志前缀沿用版本核实前的名称，实际目标及版本以本报告固定 SHA 为准。

## 复核补充（2026-10-04）

- 独立复核：3 个冲突文件及 33 个双方修改路径的增量与上游一致；124 个上游独有路径 blob 一致；Ent/Wire 重新生成无漂移；增量 golangci-lint 0 issues。
- unit：以临时 `-overlay` 绕过第一父 `ptrFloat` 重复声明补跑，service 包仅第一父已有的 `TestApplyDefaultOpenAIReasoningEffort/config_none_normalizes_to_empty_->_disabled` 失败，上游新增充值赠送/System One/支付 unit 用例全部通过。CI `make test-unit` 在修复 `ptrFloat` 前仍为红。
- 嵌入式 PG16 postgres-only 集成：Migration/Payment/UserPlatformQuota/Composite/APIKey/Department 相关 90 pass、2 skip；Redis、PG18、Docker 全量集成未覆盖。
- 语义缺口：本地 Prompt Risk、RequestArchive/Token Analysis、RequestIntercept、Prompt Metrics 未覆盖 `/v1/systemone`；订阅管理与部门用量平台白名单不含 `typesafe`（订阅管理页下拉已含 TypeSafe，选中返回 400）。TypeSafe 短期不启用，留待后续适配；详见 `llm-wiki/wiki/security-and-reliability.md`「0.2.13 安全与可靠性合同」。

## 交付状态

当前未 commit、未 push、未创建 PR、未合回 `main`、未部署；等待用户审核后再决定是否创建 merge commit。

最终暂存范围为上游/冲突代码与上述必需文档，共 167 个路径；`HEAD=main@244b058680aa56b613a7cb0862e262ed028661ef`，`MERGE_HEAD=b8dece9000c68815a5b867ca5a1e6f236e173905`。合并台账历史字节前缀保持不变；所有已知失败按冲突限定范围原样保留。
