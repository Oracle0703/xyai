# 请求来源 v2 审核证据

审核结论见 `docs/reviews/2026-10-28-token-origin-v2-implementation-review-cn.md`。

全部用例使用本机合成请求和独立 PostgreSQL 18.3 临时库，无生产正文、图片、凭据或身份数据。代码基线为 `02cf7a07cbebf4c77003f617ec7cb53c6e1150f2` 加未提交变更；被审核文件哈希见 `source-hashes.json`。

| 文件 | 内容 |
| --- | --- |
| `repo-repro.go.txt` | F1 邮箱筛选、F2 原始项目键碰撞的集成复现；复用本次新增的 `newHumanInputRows` fixture |
| `service-repro.go.txt` | F3 工具图片前纯上下文消息的分类复现 |
| `repo-repro.log`、`service-repro.log` | 3 个失败断言的完整输出 |
| `integration.log` | 现有 Token Analysis 56 个顶层 PostgreSQL 测试全部通过、0 跳过 |
| `source-hashes.json` | 审核实现与测试文件的 SHA-256 |

复现方式：将两个 `.go.txt` 分别通过 Go `-overlay` 映射到 `backend/internal/repository/zz_codex_origin_v2_review_integration_test.go` 与 `backend/internal/service/zz_codex_origin_v2_review_test.go`。overlay JSON 的 `Replace` 键和值均使用本机绝对路径。不要覆盖已有文件。

在 `backend/` 中，服务用例运行 `go test -overlay=<overlay.json> -p 1 -count=1 -v -run TestCodexV2Review ./internal/service`；数据库用例仅在空的本机隔离 PostgreSQL 库设置 `SUB2API_POSTGRES_ONLY_INTEGRATION_DSN` 后，运行 `go test -overlay=<overlay.json> -tags=integration -p 1 -count=1 -v -run TestCodexV2Review ./internal/repository`。测试初始化会执行仓库 migration，禁止指向生产或业务库。

原始本机执行脚本与日志在 `E:/tmp/token-origin-v2-review-20261028/`。数据库已停止；若重跑，应创建新的空库或受控重启这个专用审核实例。修复前预期 Go 退出码为 1，修复后应为 0。
