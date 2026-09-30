# sub2api v0.1.159 测试与审查

## 测试矩阵

| 验收标准 | 结果 | 当前证据 |
| --- | --- | --- |
| SC-1 最新边界 | 当前通过，收尾待复核 | `MERGE_HEAD=c2c19a7cb` |
| SC-2 冲突清零 | 当前通过，最终待复核 | 10 个冲突已解决；`git ls-files -u` 为空 |
| SC-3/SC-4 功能与范围 | 当前通过，最终待复核 | 本地功能组合保留；主动上游修复已撤销 |
| SC-5 文档 | 已完成，最终门禁待复核 | wiki/review/delivery 已更新；merge ledger 已 EOF 追加 |
| SC-6 全量验证 | 已完成并披露上游失败 | backend unit/lint/build 通过；integration 环境拦截包已独立通过；frontend lint/typecheck/build 通过，完整 Vitest 有 1 个上游清单 suite 无法收集 |
| SC-7 commit gate | 当前通过，最终待复核 | 尚未 commit/push/PR |

## 审查状态

| 审查 | 状态 | 说明 |
| --- | --- | --- |
| 规格符合性 | 已完成 | 10 个冲突文件保留双方入口；本地 features 无已知删除。 |
| 代码质量 | 已完成 | 上游原生问题与本地 merge 变更分离；按用户决策不在本分支修复。 |
| 最新增量 | 已完成 | 已重做到 `c2c19a7cb`，最新 16 文件增量与上游一致。 |

## 范围与一致性证据

| 检查 | 结果 | 备注 |
| --- | --- | --- |
| 异步图片上游代码 | 通过 | `image_storage.go` / test 与 `MERGE_HEAD` 一致，本地 SSRF 试验补丁已完全撤销。 |
| 批量用户限额上游代码 | 通过 | handler/service/repository/cache 文件与最新 merge-tree 一致，本地主动修复已完全撤销。 |
| 用户并发 preset 测试桩 | 待最终 unit | 仅补齐上游新增 `BatchUpdateLimits` 接口，保留本地 preset 编译能力。 |
| 最新上游 tree 对比 | 通过 | 相对上一版纯 merge tree正好是远端 16 文件 `+262/-33`。 |
| `git diff --check` / cached check | 通过 | 重做最新 merge 后均无输出。 |

## 上游原生问题（等待远端修复）

| 级别 | 问题 | 上游证据 | 本地处理 |
| --- | --- | --- | --- |
| P1 | 异步图片结果 URL 使用默认 HTTP client 下载，未限制私网/metadata/DNS rebinding/redirect。 | `backend/internal/service/image_storage.go` | 不修改；在 merge review 中披露并等待 upstream。 |
| P2 | 异步图片任务只在进程内 goroutine 执行，重启后 Redis `processing` 记录可能悬挂到 24h TTL。 | `backend/internal/service/image_task.go`、`image_task_handler.go` | 不修改；记录恢复语义限制。 |
| P2 | 批量用户限额的 cache 删除错误被底层吞掉；全量用户 ID 展开查询还存在 PostgreSQL 参数上限风险。 | `api_key_auth_cache_impl.go`、`api_key_repo.go`、`admin_user.go` | 不修改；等待 upstream。 |
| P2 | locale compile 测试直接导入未声明的 `@intlify/message-compiler`。 | `frontend/src/i18n/__tests__/localesMessageCompile.spec.ts`、`frontend/package.json` | 单独和全量均稳定复现；不补本地依赖，等待 upstream。 |

## 验证结果

| 命令 / 检查 | 结果 |
| --- | --- |
| Wire 连续生成 | 通过；两次 SHA-256 均为 `92D6F6165A21074C0AEAA39EB0FEA95C1B7EA7B86A66B96B66C34312DA512A61`。 |
| `go test -tags=unit -p 1 -count=1 ./...` | 通过；退出码 0。 |
| `go test -tags=integration -p 1 -count=1 ./...` | 运行到结尾；5 个 `.test.exe` 被本机 360 在启动前拒绝。`tlsfingerprint`、`urlvalidator` 换 fresh `GOTMPDIR` 后通过；`anthropicfp`、`antigravity`、`logredact` 编译到系统临时目录并直接执行，全部通过。 |
| `golangci-lint run --new-from-rev HEAD ./...` | 通过；`0 issues`。 |
| `go mod tidy -diff` | 非零；只建议删除 12 行继承的旧 `go.sum` 校验项，未修改模块文件。 |
| `pnpm run lint:check` | 通过。 |
| `pnpm run typecheck` | 通过。 |
| `pnpm exec vitest run` | 188 files / 1300 tests passed；1 suite 因上游未声明直接依赖而无法收集，退出码 1。 |
| `pnpm run build` | 通过；961 modules transformed，只有既有 chunk/dynamic import 警告。 |
| `go build -tags embed -trimpath` | 通过；临时产物 148,425,728 bytes。 |
| 收尾 `git fetch upstream main` | 通过；`upstream/main` 未前进，与 `MERGE_HEAD` 相同。 |
| 最终 staged 门禁 | 通过；416 files / `+29294 -1677`，无 unmerged/unstaged/untracked/marker/features 删除/未解析敏感模式。唯一 token-like 命中是上游审计脱敏单测的合成 fixture。 |

## 残余风险

| 风险 | 处置 |
| --- | --- |
| 长会话内上游继续前进 | 收尾前再次 fetch，不以旧 SHA 宣称最新。 |
| Windows `.test.exe` 锁或 360 拒绝 | fresh 本地缓存串行重跑；仍失败则明确披露。 |
| 上游 locale compile suite 无法收集 | 已确认不是冲突解决引入；本分支不补依赖，等待 upstream 修复。 |
