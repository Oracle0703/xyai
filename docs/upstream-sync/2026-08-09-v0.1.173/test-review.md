# 测试与审查

## 测试矩阵

| 验收标准 | 验证方式 | 结果 | 证据 |
|---|---|---|---|
| SC-1 | 本地/GitHub refs 与 remote URL | 已通过 | `main` 与 `github/main` 均为 `ddbb0426...`；当前 feature 尚无 GitHub 同名分支。 |
| SC-2 | merge/index/冲突审查 | 已通过 | `MERGE_HEAD=48eb3766...`；`git ls-files -u` 为空；9 个冲突文件无 marker。 |
| SC-3 | 三方 diff、features 清单、focused tests | 已通过 | 23/23 features 保留；Wire/Ent 无漂移；focused 与全量验证完成。 |
| SC-4 | 额外编辑与上游风险归因 | 已通过 | 仅有必要测试 stub 适配及两处自动合并重复 mock 去重；未改上游生产缺陷。 |
| SC-5 | wiki、ledger、图谱 | 已通过 | 六页 Wiki、append-only ledger 和 Wiki 图谱均已更新并校验。 |
| SC-6 | 后端/前端全量矩阵 | 已通过（含已归因例外） | 默认/integration/build 与前端全量通过；unit、全量 lint 和 tidy 仅报告已归因的第一父/固定上游问题。 |
| SC-7 | Git 状态 | 已通过 | 未 commit、未 push、未建 PR。 |

## 规格符合性审查

| 严重程度 | 问题 | 证据 | 状态 |
|---|---|---|---|
| 无阻断 | 9 个冲突文件均保留本地与上游必需语义。 | 独立冲突语义审查。 | 已通过 |
| 无阻断 | 70 个自动合并的双方修改路径已由专项合同、全量测试和最终只读审查覆盖。 | 三方重叠审计、最终代码审查。 | 已通过 |

## 代码质量审查

| 严重程度 | 问题 | 文件 / 行号 | 状态 |
|---|---|---|---|
| P1 上游风险 | migration 206 无法区分默认 false 与管理员主动 false。 | `backend/migrations/206_channel_monitor_v2_privacy_defaults.sql` | 记录，不在本分支修复 |
| P1 上游风险 | migration 220 会清空非 Grok、非 composite 视频价格，旧 checksum 库不自动恢复早期 composite 数据。 | `backend/migrations/220_clear_non_grok_video_generation_config.sql` | 记录，不在本分支修复 |
| P2 上游质量 | 上游新增 Channel Monitor 文档含 3 处 trailing whitespace。 | `docs/channel-monitor-v2-safe-defaults.md` | 已确认与固定上游 blob 一致，记录不修 |
| 无 | 最终独立审查未发现本次冲突解决引入的 Critical、Important 或 Minor 问题。 | 9 个冲突文件、Wire/路由、stub、GroupsView tests | 已通过 |

## 验证日志

| 命令 / 检查 | 结果 | 备注 |
|---|---|---|
| 合并前 `go test -p 1 -count=1 ./...` | 通过 | 本地 main 后端默认全量基线。 |
| 合并前前端 lint/typecheck/build | 通过 | 本地 main 前端基线。 |
| 合并前全量 Vitest | 1624/1626 通过 | 两条 rollback timeout 断言为既有失败。 |
| 合并后 `go generate ./cmd/server` | 通过 | Wire 已从合并源图生成，待最终复核。 |
| 合并后 config/routes/cmd server focused tests | 通过 | 覆盖关键冲突路径。 |
| 合并后 `go test ./internal/service` | 通过 | 首轮发现本地测试 stub 接口失配，做最小签名适配后通过。 |
| `git diff --cached --check -- <9 conflict files>` | 通过 | 冲突范围无 whitespace 问题。 |
| `go generate ./ent`; `go generate ./cmd/server` | 通过 | Ent/Wire 生成成功且无生成物漂移。 |
| `go test -p 1 -count=1 ./...` | 通过 | Backend 默认 tag 全量通过。 |
| `go test -tags=integration -p 1 -count=1 ./...` | 通过 | Backend integration tag 全量通过。 |
| `go test -tags=unit -p 1 -count=1 ./...` | 仅既有失败 | 仅 `/auth/me` fixture 未接受实际响应中的 `admin_permissions:null`，与第一父一致；其余通过，按边界不修。 |
| `go build ./...`; `go build -tags embed` | 通过 | embed 产物 160,223,744 bytes。 |
| 全量 / 增量 `golangci-lint` | 已归因 | 全量 28 项均为第一父既有；`--new-from-rev=ddbb0426...` 为 0 issues。 |
| `go mod tidy -diff` | 固定上游非 tidy | 退出 1，仅建议删除固定上游的 5 组 CLI 传递 checksum；未改 `go.sum`。 |
| 前端 lint / typecheck | 通过 | 均退出 0。 |
| 两个 GroupsView focused Vitest | 通过 | 2 files / 10 tests；验证重复 mock 去重。 |
| 完整 Vitest | 通过 | 246/246 files、1694/1694 tests。 |
| 前端 production build | 通过 | Vite 处理 1052 modules。 |
| Wiki 图谱刷新 / 状态检查 | 通过 / 预期 PARTIAL | 33 nodes / 67 edges、49 wikilinks、0 unresolved；source hash `ef937051fe5e` 匹配，PARTIAL 仅因 8 个待提交 Wiki/图谱路径。 |
| `git diff --name-only`; `git ls-files --others --exclude-standard` | 通过 | 均为空；候选快照无 unstaged 或 untracked 文件。 |
| `git ls-files -u`; index 冲突标记扫描 | 通过 | 无 unmerged index；无 `<<<<<<<` / `>>>>>>>` 残留。 |
| `git diff --cached --check -- . ':(exclude)docs/channel-monitor-v2-safe-defaults.md'` | 通过 | 除固定上游文档外，508 个 staged 路径无 whitespace/marker 问题。 |
| 固定上游文档 diff check / blob 对照 | 预期非零 / 通过 | 仅第 3、4、50 行 trailing whitespace；当前文件与 `48eb3766...` blob 完全一致，按边界不修。 |
| features 保留检查 | 通过 | 第一父与候选 index 均为 23 个 tracked 路径；无删除。 |
| Git refs / 远端复核 | 通过 | `HEAD=github/main=ddbb0426...`、领先/落后 0/0；`MERGE_HEAD=upstream/main=48eb3766...`；GitHub 无同名 feature 分支。 |

## 残余风险

| 风险 | 影响 | 缓解 / 后续动作 |
|---|---|---|
| 上游 migration 206/220 行为风险 | 升级时可能改变设置或清理视频价格。 | 不在本分支修复；部署前核验生产数据并等待上游修复。 |
| YesCaptcha key 未进入部署示例 | 开启 Grok 密码登录时可能缺 key。 | 启用前显式配置并做端到端验证。 |
| 固定上游文档 3 处 trailing whitespace | 全量 `git diff --cached --check` 会退出非零。 | 固定上游 blob 原样保留；排除该文件后检查必须通过，并在最终审核摘要明确说明。 |
