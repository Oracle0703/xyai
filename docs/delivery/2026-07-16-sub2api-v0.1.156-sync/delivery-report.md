# sub2api v0.1.156 上游同步交付报告

## 总结

已在 `feature/hy/10157_同步sub2api主线` 中，以本地 `main@4c456aad3` 为第一父、`Wei-Shaw/sub2api main@d515c3045` 为固定第二父完成无提交合并。3 个文本冲突与 29 个双方修改交集已审查，10 类本地能力均保留或与上游组合，当前仍保留 `MERGE_HEAD` 等待用户审核。

## 交付内容

| 领域 | 结果 |
| --- | --- |
| 上游 0.1.156 | 合入 Agent Identity、账号复制、first-output guard、Grok credential/reconcile、token refresh、scheduler、compat bridge、静态缓存等增量。 |
| 冲突 | config、config tests、content moderation 均为语义组合，没有整文件接受 ours/theirs。 |
| 本地功能 | RequestArchive/Intercept、Prompt Metrics/Risk、Token Analysis、组织用量、ImageGen/支付、并发 preset、compatible 适配、默认 reasoning effort、quota flusher 均有代码和测试锚点。 |
| 合并修复 | 补 Wire binding、修正 UseKeyModal 测试选择器、保留 failover headers、即时更新风控快照、收紧 `Retry-After` 原始值与时间边界。 |
| 文档 | 更新 6 个 `llm-wiki/wiki/*.md`、3 个组件 README、追加 merge ledger、新增 merge review 与本交付材料。 |

## 验证证据

| 验证 | 结果 |
| --- | --- |
| 后端 unit | 完整重跑退出 0：50 个测试包通过、53 个无测试包、失败 0。 |
| 后端 integration | 完整运行到结尾：44 个测试包通过、57 个无测试包；唯一 `openai_compat.test.exe` 被 360 环境阻断，未伪报为通过。 |
| 后端 lint | 全量 29 条均为第一父既有债务；`--new-from-rev main` 退出 0、0 issues。 |
| 前端 | Vitest 176 files / 1214 tests；lint、typecheck、production build 均退出 0。 |
| 生产构建 | 前端 build 后 embed build 退出 0，产物 147,534,336 bytes。 |
| 生成与格式 | Wire 连续两次稳定生成；gofmt 和三种 diff check 通过。 |
| 最终 Git 硬门 | 274 files / `+32475 -1728`；双父边界正确，无 unstaged/untracked/unmerged、冲突标记、CRLF/BOM、敏感模式或 `docs/features` 删除。 |

详细命令、RED/GREEN 与环境诊断见 `docs/delivery/2026-07-16-sub2api-v0.1.156-sync/test-review.md`。

## 重要文件

| 文件 | 用途 |
| --- | --- |
| `docs/features/sub2api -merage-list.md` | 追加型 0.1.156 合并记录 |
| `docs/reviews/2026-07-16-sub2api-v0.1.156-merge-review.md` | 冲突、交集、本地能力和审查问题证据 |
| `llm-wiki/wiki/*.md` | 0.1.156 稳定架构、配置、数据与安全边界 |
| `backend/internal/service/content_moderation.go` | 上游 runtime snapshot 与本地 Prompt Risk/LLM judge 组合 |
| `backend/internal/service/openai_gateway_passthrough.go` | 统一 `Retry-After` 安全合同 |
| `backend/internal/service/wire.go` | Grok OAuth token service binding |

## 已知限制

- 本机 360 安全软件会持续锁定 integration 版本的 `openai_compat.test.exe`，即使换目录、改文件名和有限重试也无法执行；未更改安全软件配置。
- 全量 lint 的 29 条历史债务和前端构建 warning 未在本次上游同步中扩范围清理。
- merge commit 尚未创建；ledger 中如实记录 `MERGE_HEAD`，用户批准后才能提交。

## 下一步

用户审核 staged diff、merge review、测试证据与环境限制。批准后再创建 merge commit，并回填 ledger 的真实 merge commit SHA；当前不 push、不创建 PR、不部署。
