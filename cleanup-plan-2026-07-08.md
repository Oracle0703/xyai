# 仓库代码整洁整改方案

- 日期：2026-07-08
- 背景：审查 `upstream/main` 合并过程中，发现仓库存在历史遗留的调试产物、死代码、未过滤的编译缓存，影响仓库整洁度并增加误提交风险。
- 本次整改**不修改业务逻辑**，仅清理冗余、修正 `.gitignore`。

---

## 一、问题总览

| # | 类型 | 路径 | 大小 | 根因 |
|---|---|---|---|---|
| 1 | 调试 diff 文件 | `_conflict_oai.diff` / `_local_oai.diff` / `_merged_oai.diff` / `_merged_test.diff` / `_upstream_oai.diff` | 5 个，共 ~182KB | codex 合并当天（commit `b116c7f27`）误提交 |
| 2 | 死代码 Go 文件 | `backend/_local_compat.go` / `backend/_upstream_fallback.go` | 2 个，共 ~38KB | codex 合并脚手架，函数已迁入正式文件，下划线开头导致从未被编译 |
| 3 | Go 编译缓存未被 gitignore | `backend/.go-test-cache/` / `backend/.gocache-test/` / `backend/.gotmp-test/` | **~2.2GB** | `.gitignore` 未覆盖这些目录名 |
| 4 | `AGENTS.md` | 仓库根目录 | ~3KB | MERGE_HEAD 带来的文件，与项目自有的 `DEV_GUIDE.md` 定位重叠 |
| 5 | `LOCAL_STARTUP_NOTES.md` | 仓库根目录 | ~1.5KB | 个人化启动笔记，不适合入库 |

---

## 二、逐项分析

### 问题 1：codex 调试 diff 文件（_*.diff）

**证据**：

```bash
$ git log HEAD -1 -- _conflict_oai.diff
b116c7f27 feat: add compatibility layer for OpenAI responses
    Date: Thu May 21 01:43:50 2026 +0800
```

这 5 个 `.diff` 文件是 2026-05-21 codex 处理 `upstream/main` 合并时生成的中间产物（冲突对比、本地版本、上游版本、合并结果、测试差异），被作为 `feat` 提交进仓库。它们对最终代码零贡献，且极易与真实补丁混淆。

**验证**：文件创建时间全部为 `May 21 00:38~00:41`，与合并操作时间完全吻合。

**整改动作**：从 git 历史中移除。

```bash
git rm _conflict_oai.diff _local_oai.diff _merged_oai.diff _merged_test.diff _upstream_oai.diff
git commit -m "chore: remove codex merge debug diff artifacts"
```

⚠️ 注意：本清理需在**本次 merge 提交完成后**再单独做，避免干扰当前合并冲突状态。

---

### 问题 2：死代码 Go 文件（backend/_local_compat.go / backend/_upstream_fallback.go）

**证据**：

- 文件以 `_` 开头 → Go 工具链在编译时**自动忽略**（跨平台行为一致，见 [Gocmd-Ignored](https://pkg.go.dev/cmd/go#hdr-Ignored_and_missing_packages)）
- 文件中的导出函数（`forwardResponsesViaRawChatCompletions` / `bufferResponsesViaRawChatCompletions` / `streamResponsesViaRawChatCompletions` / `openAICompatibleResponsesToChatOptions` 等）已完整迁入 `internal/service/openai_gateway_responses_chat_fallback.go`（正式文件，本次合并保留）
- 验证实验：删除这两个文件后 `go build ./internal/service/` 仍通过（exit 0），证实其为冗余
- 删除实验也验证了不存在隐藏的 import 引用

```bash
$ grep -rn "_local_compat\|local_compat" backend/ --include="*.go" | grep -v "_local_compat.go:"
# 空。无其他文件引用
```

**整改动作**：从 git 历史中移除。

```bash
git rm backend/_local_compat.go backend/_upstream_fallback.go
git commit -m "chore: remove codex merge scaffolding dead code"
```

---

### 问题 3：Go 编译缓存未被 gitignore

**证据**：

```bash
$ du -sh backend/.go-test-cache/ backend/.gocache-test/ backend/.gotmp-test/
1.1G    backend/.gocache-test/
1.1G    backend/.gotmp-test/
```

这三个目录是 Go 编译/测试产生的本地缓存（含 `gopsutil`、`golang.org/toolchain`、`modernc.org/libc` 等模块临时构建产物），总量 **~2.2GB**。它们：

- 已通过 `git status` 确认为 untracked（不会被提交）
- 但**根本不该存在于工作树**中（应该在系统临时目录，或在仓库外）
- 当前 `.gitignore` 仅覆盖 `.gocache/`，未覆盖 `.go-test-cache/`、`.gocache-test/`、`.gotmp-test/`

**整改动作**：

**（A）补充 .gitignore**（立即做，无需等 merge 完成）：

```bash
# 追加到 .gitignore
cat >> .gitignore << 'EOF'

# Go 测试/编译缓存目录（不应放在仓库工作树）
backend/.go-test-cache/
backend/.gocache-test/
backend/.gotmp-test/
backend/.gotmp-review-*/
backend/.gotmp-*/
.gotmp-review-*/
.gotmp-*/
EOF
```

**（B）物理删除已有的缓存目录**（释放 ~2.2GB 空间）：

```bash
rm -rf backend/.go-test-cache/ backend/.gocache-test/ backend/.gotmp-test/
```

**（C）根本原因修复**：这些缓存目录被创建到仓库目录内，通常是因为 `GOCACHE` / `GOTMPDIR` 环境变量在 Windows 上指向了仓库路径。建议在项目 `README.md` 或 `Makefile` 中加入引导：

```makefile
# Makefile 中加入（Windows 开发者）
export GOCACHE := $(shell mktemp -d)
export GOTMPDIR := $(shell mktemp -d)
```

或在项目 `.env.example` 中声明期望值。

---

### 问题 4：AGENTS.md（MERGE_HEAD 带来的外部文件）

**证据**：

```bash
$ git log HEAD -1 -- AGENTS.md
6b90b3cb6 docs: record upstream merge process
    Date: Tue May 26 17:11:42 2026
```

`AGENTS.md` 由 ours 分支引入（非 upstream），官方说明是"为 AI coding agent 提供仓库指引"。其功能与项目自有的 `DEV_GUIDE.md`（开发指南）重叠，且内容落后于后者。

**建议**：如 `AGENTS.md` 暂无独立存在价值，合并入 `DEV_GUIDE.md` 后删除；或保留但纳入 `.gitignore` 的个人本地化版本。本方案暂不强制删除，仅记录待你决策。

---

### 问题 5：LOCAL_STARTUP_NOTES.md（个人化启动笔记）

**证据**：

```bash
$ git log HEAD -1 -- LOCAL_STARTUP_NOTES.md
0285e765f chore: add local startup scripts and usage fallback
    Date: Thu May 21 00:08:35 2026 +0800
```

这是 2026-05-21 在合并当天添加的个人本地启动脚本说明，内容不适合进入共享仓库。

**整改动作**：移除并加入 .gitignore 防止再次误提交。

```bash
git rm LOCAL_STARTUP_NOTES.md
echo "LOCAL_STARTUP_NOTES.md" >> .gitignore
git commit -m "chore: remove local startup notes from repo"
```

---

## 三、整改执行计划

### 立即执行（不干扰当前 merge）

| 步骤 | 命令 | 说明 |
|---|---|---|
| 1 | 编辑 `.gitignore` 追加上述缓存目录规则 | 防止将来误提交 |
| 2 | `rm -rf backend/.go-test-cache/ backend/.gocache-test/ backend/.gotmp-test/` | 释放 ~2.2GB |

### 待本次 merge 提交完成后（单独 commit）

| 步骤 | commit message | 内容 |
|---|---|---|
| 3 | `chore: remove codex merge debug diff artifacts` | `git rm` 5 个 `.diff` 文件 |
| 4 | `chore: remove codex merge scaffolding dead code` | `git rm` 2 个 `_*.go` 死代码文件 |
| 5 | `chore: remove local startup notes from repo` | 可选。视 AGENTS.md / LOCAL_STARTUP_NOTES.md 处理决策 |

---

## 四、验证清单

执行整改后，应确认：

- [ ] `go build ./...` 仍通过
- [ ] `git status` 干净（仅含预期中的未跟踪文件）
- [ ] `_*.diff` / `backend/_local_compat.go` / `backend/_upstream_fallback.go` 在 git tree 中不存在：
  ```bash
  git ls-files -- '_*.diff' 'backend/_*.go' | wc -l   # 应返回 0
  ```
- [ ] `.gitignore` 新规则生效：
  ```bash
  git check-ignore backend/.go-test-cache/             # 应输出匹配路径
  ```
- [ ] 仓库体积无明显异常（清理后应释放 ~2.2GB 磁盘空间，git 对象库不受影响）

---

## 五、避免再次引入的长期措施

1. **在 `.gitignore` 中以更通用的模式匹配 Go 缓存目录**：
   ```
   # 匹配任意 .go- 或 .gotmp 开头目录
   .go-*/
   .gotmp-*/
   backend/.go-*/
   backend/.gotmp-*/
   ```
2. **在 CI / pre-commit hook 中扫描**：禁止以 `_` 开头或 `.diff` 结尾的文件被 `git add`。
3. **Windows 开发者指南**：在 `Makefile` 或 `README.md` 中明确 `GOCACHE` 应指向仓库外，避免再次生成到工作树。
