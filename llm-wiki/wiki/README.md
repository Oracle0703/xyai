# Sub2API llm-wiki 基线

更新时间: 2026-07-27

本知识库面向后续 AI 开发前快速读取。进入任务后先读本页, 再按任务类型读取相关页面。若 wiki 与源码冲突, 以源码为准并修正 wiki。

## 项目一句话

Sub2API 是一个 AI API 网关和管理平台, 用 Go + Gin + Ent 提供后端服务, 用 Vue 3 + Vite + Pinia 提供管理与用户前端, 支持 Claude/OpenAI/Gemini/Antigravity/Grok 等上游账号调度, API Key 分发, 用量计费, 支付, 订阅, 运维监控和请求转发。


## 最近同步

- 2026-07-27 从本地 `main@c819b17fdd024a7a936a45b6d725022f4fd6af6c` 创建 `feature/hy/10166_合并1.166版本`, 固定合入 `Wei-Shaw/sub2api main@59ce11c78000bde5bdd74930b5885753037a5841`, 当前后端版本 `0.1.166`; merge commit 待用户审核后创建。上游增量为 142 个文件、`+7131/-512`, 新增默认开启的 Panel API 分层限流、显式 `CONFIG_FILE`、通用 settings PUT 的省略字段保留、管理用量 request ID 精确筛选、支付看板按币种隔离统计、OpenAI WS 每轮模型映射/计费归因、Antigravity 原生 OAuth 的 Chat/Responses 兼容转发和 Codex++ Responses/Anthropic 工具兼容, 并避免 Caddy 压缩缓冲 SSE。8 个文本冲突按语义并集解决；Prompt Metrics 独立管理路由同步接入上游 Panel 限流。110 个仅上游路径与固定提交逐 blob 一致, 425 个仅本地修改路径未被触碰, 32 个双方修改路径已审查, 23 个 tracked `docs/features/` 文件零删除。本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、子管理员、compatible cache usage、默认 reasoning effort、用户并发 preset 和 quota flusher 继续保留；本分支只解决合并冲突, 不修复上游自身问题。
- 2026-07-26 从本地 `main@e3b10c17b960bcf92d3e5ca340e3d39056f56c13` 创建 `feature/hy/10165_合并1.165版本`, 固定合入 `Wei-Shaw/sub2api main@2730c1c43b29be003925b033f3f9e645e726bb8c`, 当前后端版本 `0.1.165`; merge commit 待用户审核后创建。本轮上游增加 OpenAI Live 网关与 macOS attestation、客户端 session ID 用量归因、注册邮箱别名去重、请求驱动的 Ollama Cloud 用量刷新、公告预览/共享富文本样式、Claude Opus 5 与 PostCSS 安全升级, 并修正多项 OpenAI/Gemini/Grok 转发边界。唯一文本冲突 `backend/internal/config/config.go` 按语义并集保留上游 Live 默认值和本地 RequestArchive/RequestIntercept；35 个双方修改文件已审查, 133 个仅上游路径与固定提交逐 blob 一致, 22 个 tracked `docs/features/` 文件零删除。本地 Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、子管理员、compatible cache usage、默认 reasoning effort、用户并发 preset 和 quota flusher 继续保留；本分支只解决合并冲突, 不修复上游自身问题。
- 2026-07-23 从本地 `main@ddbf5ab414991475e6ad6f81663d5eed5b7d7d3a` 创建 `feature/hy/10164_合并1.164版本`, 固定合入 `Wei-Shaw/sub2api main@cb24522dd53f8f363d008e3afdc8e4baf9788cab`, 当前后端版本 `0.1.164`; merge commit 待用户审核后创建。本轮上游增加 composite group/model route、Ollama Cloud 官方用量自动刷新、移动端支付宝 precreate 深链和 OpenAI 代理流断开进程内隔离。5 个文本冲突按语义并集解决, 继续保留本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、子管理员、compatible cache usage、默认 reasoning effort、用户并发 preset 和 quota flusher；本分支只解决合并冲突, 不修复上游自身问题。
- 2026-07-23 从本地 `main@e52b5c89d07ac058043de5adb983cad8750cab58` 创建 `feature/hy/10163_合并1.163版本`, 固定合入 `Wei-Shaw/sub2api main@60013c5f100be7b4f2e6caee415883d221d33e32`, 上游 merge commit 为 `3e4f4e3f1e987783298c3b28b60f01de80618ac2`, 当前后端版本 `0.1.163`；随后同步 `Oracle0703/xyai main@5cc963c6c4458121769ff4f18a1b53e4b29b523d`, 带入已合并的 0.1.162 记录。本轮上游增加 OpenAI 分组 reasoning effort 精确映射与上限、客户端 IP 自定义转发头和可信代理兼容模式、异步图片对象存储后台热配置、Grok 本地 count-tokens / compact / Codex client tools 兼容，以及 hosted image tool usage 计费提取。文档冲突按语义并集解决, 继续保留本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、子管理员、compatible cache usage、默认 reasoning effort、用户并发 preset 和 quota flusher；本分支只解决合并冲突, 不修复上游自身问题。
- 2026-07-20 从本地 `main@e52b5c89d07ac058043de5adb983cad8750cab58` 创建 `feature/hy/10162_合并1.162版本`, 固定合入 `Wei-Shaw/sub2api main@e625ce3b3b3b955b7c3afc93221f7c5f0ae55aa8`, merge commit 为 `ea26f2b0755323dcd750dbdb01cb35991a396be7`, 后端版本为 `0.1.162`。`v0.1.162` tag `27f094e09` 的 `VERSION` 仍为 `0.1.161`, 因而以随后 version-sync commit 为边界。本轮上游增加客户端 IP 兼容模式/自定义请求头、异步生图对象存储后台热配置、Grok 本地 count-tokens 与客户端工具缓存路由、Prompt Audit 阻断意图 fail-closed、Codex manifest 401 账号隔离、`UPDATE_GITHUB_TOKEN` 和 SVG branding；唯一文本冲突 `backend/internal/server/routes/gateway_test.go` 采用上游 helper 并迁入本地 RequestArchive/RequestIntercept 测试。45 个双方修改文件已做语义复核, 22 个 `docs/features/` 文件及本地 Prompt Metrics/Risk、Token Analysis、组织用量、图片生成、并发 preset、compatible 适配、默认 reasoning effort、quota flusher 和子管理员权限均保留。上游 rollback timeout 测试失配和本地既有 `/auth/me` contract mismatch 只记录, 未在合并分支修复。
- 2026-07-19 从本地 `main@332fdbd0b84619cfb1da6fcb57b65d4d9263b2e9` 创建 `feature/hy/10161_合并1.161版本`, 固定合入 `Wei-Shaw/sub2api main@d4b9797ff72024960a035cf22fdd8f213e149169`, merge commit 为 `e3e6b52da43a5be351cf59089976759eebc28376`, 当前后端版本 `0.1.161`。本轮上游增加 HTTP ingress 有界防护、拒绝聚合、跨实例 API Key 鉴权缓存失效 outbox、安全开关默认关闭、Grok 受保护视频同源代理、模型级临时隔离、OpenAI WS/Responses 流修复和初始 branding 注入；4 个文本冲突做语义并集, 继续保留本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、图片生成/支付、用户并发 preset、compatible 适配、默认 reasoning effort、quota flusher 和子管理员权限。
- 2026-07-17 从本地 `main@5d5f157854b9a88cc57da1600095bb404b78ed45` 创建 `feature/hy/10160_合并1.160版本`, 固定合入 `Wei-Shaw/sub2api main@57914967cbb127ff715719c3879d881c10d75274`, 当前后端版本 `0.1.160`。本轮上游增加 OpenAI-compatible Prompt Audit（默认关闭, 支持异步审计/同步阻断）、审计事件筛选删除、Grok media 账号资格隔离、S3 配置 step-up TOTP 和 locale compiler 直接依赖；冲突解决继续保留本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk 与 LLM judge、Token Analysis、组织用量、用户并发 preset、compatible 适配、默认 reasoning effort 和 quota flusher。上游 Prompt Audit Wire 源图缺少接口绑定的问题只记录, 未在本分支修复。
- 2026-07-15 新增账号级 `sub_admin` 角色与三项固定管理权限。管理接口使用数据库最新用户数据和 HTTP 方法 + Gin 路由模板白名单鉴权，未知路由默认拒绝；前端按服务端权限目录配置账号，并对订阅、使用记录和 Token 分析页面收口写操作。
- 2026-07-17 在 `feature/hy/10157_同步sub2api主线` 继续合并 `Wei-Shaw/sub2api main@c2c19a7cbe8486ebb5b56834d1a6e07b3f12cffc`, 当前后端版本 `0.1.159`。本轮上游增加异步图片任务与 S3 结果转存、操作审计/会话绑定/敏感操作 step-up 2FA、API Key 计费倍率自省与上游倍率探测、用户批量限额、账号/分组/监控复制、图片输入 token 独立计费及统一客户端 IP 信任策略；冲突解决保留本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk、Token Analysis、组织用量、用户并发 preset、compatible 适配和 quota flusher。上游异步图片 URL 下载 SSRF、进程重启后 processing 任务恢复及批量限额 cache 终态风险只记录等待远端修复，本地未改写上游生产实现。
- 2026-07-16 从本地 `main` 创建 `feature/hy/10157_同步sub2api主线`, 固定合入 `Wei-Shaw/sub2api` `main@d515c3045ce838976ebedab87846aaaf893dbbf6`（包含 `v0.1.156` tag `12f991d` 和紧随其后的 `VERSION` 同步）, 当前后端版本 `0.1.156`。本次上游新增 Agent Identity、管理员安全复制账号、根级 `/models`、native Responses first-output timeout、WS 首消息 timeout、Grok OAuth pool/reconcile、token refresh provider 并发/QPS/熔断; 冲突解决继续保留本地 RequestArchive/RequestIntercept、Prompt Metrics、Token Analysis、组织报表、用户并发、默认 reasoning effort、Prompt Risk/LLM judge 和 quota flusher。
- 2026-07-14 从本地 `main` 创建 `feature/hy/10155_同步sub2api主线`, 合并 `Wei-Shaw/sub2api` `main` 到 `7c717365ef728e53cdcf6d639a4dd68226db03b2`, 当前后端版本 `0.1.155`。本次上游增加管理端 opt-in Server-Timing、OpenAI 账号级长上下文计费、Responses namespace 可逆摊平、图片非流式 keepalive/终态修正、Grok Web SSO 导入与渠道监控、Ops host 筛选、Codex manifest failover 和调度全量重建并发修复; 冲突解决继续保留本地 Redis 7+/Memurai 启动校验、Prompt Metrics、Prompt Risk/LLM judge 与内容审核配置, 并审查了 23 个双方同时修改的自动合并文件。
- 2026-07-13 基于 `feature/hy/10151_同步sub2api主线` 创建 `feature/hy/10153_同步sub2api主线`, 固定同步 `Wei-Shaw/sub2api` 到 `55ed0ab0da367183d97c15659e33ae9e83f6ff90`, 当前后端版本 `0.1.153`; 明确不包含其后的 `7d239d62e`。本次上游增加 OpenAI WS ingress 空闲/API Key 连接上限、Grok API Key 与视频编辑/扩展、alpha search 按次计费、API Key 最新 IP 索引、Responses `additional_tools`/Read 流式参数/stop reason 兼容、嵌入式静态资源长缓存、Apple container、账号 `plan_type` 和 DataTable 小列表非虚拟化等能力; 冲突解决继续保留本地 RequestArchive/RequestIntercept、Responses→Chat options adapter、`ConcurrencyCacheError` 和 OpenAI-compatible provider preset。
- 2026-07-13 同步 `Wei-Shaw/sub2api` `main` 到 `feature/hy/10151_同步sub2api主线`, 当前后端版本 `0.1.151`。本次上游引入 Responses/Chat 的 custom、namespace、tool_search 工具桥, Codex alpha search 与 identity 修复, 用户级 Fast/Flex 策略, Grok Free OAuth prompt cache/Chat bridge/quota recovery, compact keepalive 加固和 Responses/Anthropic cache creation 透传; 冲突解决继续保留 RequestArchive/RequestIntercept、Token Analysis、图片生成、敏感词过滤、用户并发、第三方 Responses->Chat options 过滤和 OpenAI-compatible cache usage。

## 文档地图

- [[backend]]
- [[frontend]]
- [[ops]]
- [[data-and-domain]]
- [[security-and-reliability]]
- [[ai-workflow]]

## 文档地图（路径说明）

- `backend.md`: 后端入口, 路由, Wire 依赖注入, service/repository 分层, 网关路径。
- `frontend.md`: Vue 前端入口, 路由守卫, store, API client, 组件和样式约定。
- `ops.md`: 本地启动, 构建, 测试, CI, 配置和部署入口。
- `data-and-domain.md`: 核心领域对象, Ent schema, SQL migration, 支付/订阅/计费知识。
- `security-and-reliability.md`: 认证, 权限, 限流, 幂等, CSP, URL allowlist, 网关可靠性。
- `ai-workflow.md`: Codex/Copilot 日常如何读取和更新 llm-wiki。

## 知识图谱

- Wiki 图谱（可共享）: `llm-wiki/.understand-anything/knowledge-graph.json`
- 代码图谱（本机）: `.understand-anything/knowledge-graph.json`
- 启动: `tools\start-understand-dashboard.cmd`
- 状态: `tools\check-understand-status.cmd`
- 刷新 Wiki 图: `tools\refresh-understand-wiki.cmd`
- 详情见 [[ai-workflow]]

## 快速定位

| 任务 | 优先阅读 |
| --- | --- |
| 改后端接口或路由 | `backend.md`, `security-and-reliability.md` |
| 改网关转发, 模型映射, OpenAI/Claude/Gemini 兼容 | `backend.md`, `ops.md`, `security-and-reliability.md` |
| 改前端页面, store, API 调用 | `frontend.md`, 对应 `frontend/src/components/**/README.md` |
| 改数据库字段或索引 | `data-and-domain.md`, `backend/migrations/README.md` |
| 改支付, 订阅, 余额, 兑换码 | `data-and-domain.md`, `security-and-reliability.md` |
| 改启动, 配置, CI | `ops.md` |

## 高优先级维护约束

- 不修改业务代码来更新 wiki; wiki 只记录知识。
- 修改 `backend/ent/schema` 后必须运行 `go generate ./ent` 和 `go generate ./cmd/server`, 并提交生成代码。
- 新增 SQL migration 只能新增文件, 不修改已应用 migration; `_notx.sql` 只用于并发索引等非事务语句。
- 前端必须使用 pnpm, `package.json` 变更要同步 `pnpm-lock.yaml`。
- 修改 `frontend/src/components/` 下模块时, 同步更新该模块目录下 `README.md`。
- 修复缺陷先定位根因, 再实施和验证。

## 当前基线阅读范围

本基线根据以下项目内容整理:

- 根目录文档和构建文件: `README.md`, `README_CN.md`, `DEV_GUIDE.md`, `LOCAL_STARTUP_NOTES.md`, `Makefile`, `Dockerfile`。
- 后端入口和关键层: `backend/cmd/server`, `backend/internal/server`, `backend/internal/handler`, `backend/internal/service`, `backend/internal/repository`, `backend/internal/config`, `backend/migrations`, `backend/ent/schema`。
- 前端入口和关键层: `frontend/src/main.ts`, `frontend/src/App.vue`, `frontend/src/router`, `frontend/src/api`, `frontend/src/stores`, `frontend/src/components`, `frontend/vite.config.ts`, `frontend/package.json`。
- CI 和部署: `.github/workflows`, `deploy/config.example.yaml`, `deploy/*.yml`, `tools/start-local.ps1`。
