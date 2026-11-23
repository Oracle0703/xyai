# 0.2.24 上游合并审核

2026-11-23 · `feature/hy/10238_merge_sub2api_224` · 待用户审核后提交

## 1. 范围

| 项目 | 结果 |
| --- | --- |
| 第一父 | 本地 `main@80034d8eb6c63a552ed1aca051ebacde232e53b0`，建分支时与 `github/main` 相同 |
| 第二父 | `Wei-Shaw/sub2api main@328db26f450edbdf9f8d4344a3ea903ac6396842`，VERSION 为 0.2.24 |
| 共同祖先 | `2af3f1e15c98c893a89255a037f551046883b8c0`（0.2.23） |
| 上游增量 | 110 个提交（含合并提交），190 个路径，新增 11853 行、删除 610 行。可用 `git diff --name-status HEAD...MERGE_HEAD` 查看完整清单 |

## 2. 冲突与处理

| 文本冲突文件 | 裁决 |
| --- | --- |
| `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` | 新增上游 Stop 透传，保留本地可选字段过滤与流式 usage 选项 |
| `backend/internal/server/routes/gateway.go` | 合入分组文本大小限制、白名单后的歧义检查。根别名、Codex 直连与分组路由保留归档、拦截和已有子路径守卫顺序。两份上游路由测试断言改为匹配实际注册链，去除旧的示例注释 |
| `backend/internal/service/openai_responses_namespace.go` | 保留具名独立输出 namespace，其他项采用上游字符串级处理 |
| `backend/internal/service/openai_responses_tool_schema.go` | 并入上游工具 schema 预筛与扫描，本地结构化输出 text.format.schema 校正保留 |
| `frontend/src/components/admin/account/AccountActionMenu.vue` | 刷新按钮保留管理权限条件，添加上游提示 |

HTTP Responses 入口还有一处行为重叠。上游 normalizer 已完整覆盖本地 `agent/proposed_plan`、`agent/untrusted_content` 的有效输入，并处理空白计划片段、空字符串来源；删除这两项的本地校验豁免，入口回归改为期望上游的 user message。`browser_tabs`、三种 Guardian 输出、云线程消息及混合 bootstrap 路径保留原用例与实现。

## 3. 本地功能

- 共同祖先至两侧共有 33 个双方修改路径。以行增删多重集对照，30 个文件完整保留双方增量，另 3 个为桥接字段、路由与账号菜单的同一行合并。仓库外 `delta-audit.json` 保存逐路径结果。
- 文档更新前，807 个仅本地路径中 805 个与第一父逐 blob 相同；另两项为上述上下文 helper 和测试。157 个仅上游路径中 155 个与上游逐 blob 相同，另两项为路由断言。
- 27 份 `docs/features/` 与第一父逐 blob 相同。核对范围包括请求归档/拦截、Token Analysis、组织与部门、子管理员、订阅、并发预设、图片生成、请求时区、OpenAI compatible 字段过滤、Grok 自动思考强度。未合入其他工作树分支。

## 4. 上游行为

采用 Exa 搜索、Claude 压缩请求的 5m 缓存、Grok 搜索计费、管理员建用户默认配额、账号窗口成本和刷新提示、HTTP/WS 歧义字段检查、Antigravity 429 冷却范围与端点配额查询、OpenAI 调度与协议转换。该范围没有新增数据库迁移、Ent schema 或前端依赖。管理员创建账号时，仅 `user` 获得默认平台配额；本地 `sub_admin` 仍走默认订阅流程。详细合同见 llm-wiki 对应主题。

## 5. 验证

Go 1.27.4。前三行运行 `go test -json -p 1 -count=1 -timeout 60m ./...`，后两种构建标签分别为 `-tags=unit` 和 `-tags=integration`。三套有重复用例，按套列出。

| 套件 | 顶层通过 | 顶层失败 | 显式跳过（顶层） | 命令退出码 |
| --- | ---: | ---: | ---: | ---: |
| go-default | 8129 | 1 | 14 | 1 |
| go-unit | 13959 | 1 | 41 | 1 |
| go-integration | 7625 | 0 | 18 | 0 |
| pgredis | 875 | 0 | 6 | 0 |
| audit-fresh | 114 | 0 | 0 | 0 |

- 本机标准 integration 因无 Docker 跳过 repository 包。在独立 PostgreSQL 18.3、Redis 8.10.2 上，以仓库外 Go overlay 给 postgres-only 测试入口提供同一隔离库的 DSN 和独立 Redis 客户端，重跑整个 repository 包，无 `-run` 或 `-skip` 筛选。875 个顶层用例通过，7 个显式跳过（含子用例），名称及原因可查原始日志。
- securityaudit 使用独立空库、Redis 和原版测试入口，全包 114 个顶层用例通过。首次复用已迁移的仓储库时，测试夹具用 DEFAULT VALUES 创建用户，撞到现行业务表的 email 非空约束；改用空库后同一套件通过。临时服务已停止。
- 前端 `pnpm run lint:check`、`typecheck`、`test:run`、`build` 全部通过。Vitest 441 个文件、3700/3700 用例通过。Go 嵌入前端的服务构建通过，二进制 `-version` 输出 0.2.24。
- 同一组 8 种真实生产者格式在第一父、固定上游和合并代码运行相同的 HTTP Responses 规范化与校验链。计划、未命名外部来源在第一父原样通过，新 IDE 上下文在第一父拒绝；三者在上游和合并结果均改写为 user message 后通过。浏览器标签、三种 Guardian 输出和云线程消息在第一父与合并结果均原样通过，纯上游均拒绝。实测按名称存在 `context-{main,upstream,merged}.log`。
- Wiki 图谱已刷新，35 个节点、76 条边，wiki 链接无悬空项。暂存差异空白检查通过。所有原始日志、父版本快照与测试 overlay 位于仓库外 `E:/tmp/xyai-merge-224-20261123/`。

### 提交前独立复核

复核者确认 5 个文本冲突的裁决正确，未发现阻塞项。

| 复核范围 | 结果 |
| --- | --- |
| Windows 全量 golangci-lint | 使用 Go 1.27.4 构建的 v2.14，取消同类问题和单个 linter 的报告数量上限，0 issues |
| Wire 生成一致性 | `wire diff ./cmd/server` 无差异，执行中触及的 `go.sum` 已还原 |
| 冲突区回归 | 路由、协议转换、handler 与 service 相关测试复跑通过 |
| 文件与归档 | 仅本地路径 805/807、仅上游路径 155/157 与各自来源一致；本轮仅一份审核记录，台账只追加，旧目录未恢复 |

Linux 目标的全量 lint 已按用户要求停止。

## 6. 遗留问题

| 项目 | 证据与处理 |
| --- | --- |
| WebSocket 首 token 耗时断言 | `TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode` 在默认全量的 semantic 子用例观测到 499 ms，断言下限为 500 ms。相同测试精确复跑 3 次，第一父 2 过 1 败、固定上游 1 过 2 败、合并分支 2 过 1 败。用例、阈值与计时实现沿用合并结果，等待上游处理 |
| Windows 发布工具测试 | 补入 Git Bash 的 PATH 后 10 项中 9 项通过。`test_linux_context_preserves_binary_executable_mode` 在 Windows 得到 0666，期望 Linux 0755。第一父快照同环境得到相同 9/10 与相同断言，保留发布代码 |
| WebSocket 请求强度重复赋值 | HTTP bridge 返回后，上游逻辑会对非空 result 统一回填 RequestedReasoningEffort。先前的本地 Grok 特判可在合并后清理 |
