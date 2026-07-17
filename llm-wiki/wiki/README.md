# Sub2API llm-wiki 基线

更新时间: 2026-07-17

本知识库面向后续 AI 开发前快速读取。进入任务后先读本页, 再按任务类型读取相关页面。若 wiki 与源码冲突, 以源码为准并修正 wiki。

## 项目一句话

Sub2API 是一个 AI API 网关和管理平台, 用 Go + Gin + Ent 提供后端服务, 用 Vue 3 + Vite + Pinia 提供管理与用户前端, 支持 Claude/OpenAI/Gemini/Antigravity/Grok 等上游账号调度, API Key 分发, 用量计费, 支付, 订阅, 运维监控和请求转发。


## 最近同步

- 2026-07-17 在 `feature/hy/10157_同步sub2api主线` 继续合并 `Wei-Shaw/sub2api main@c2c19a7cbe8486ebb5b56834d1a6e07b3f12cffc`, 当前后端版本 `0.1.159`。本轮上游增加异步图片任务与 S3 结果转存、操作审计/会话绑定/敏感操作 step-up 2FA、API Key 计费倍率自省与上游倍率探测、用户批量限额、账号/分组/监控复制、图片输入 token 独立计费及统一客户端 IP 信任策略；冲突解决保留本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk、Token Analysis、组织用量、用户并发 preset、compatible 适配和 quota flusher。上游异步图片 URL 下载 SSRF、进程重启后 processing 任务恢复及批量限额 cache 终态风险只记录等待远端修复，本地未改写上游生产实现。
- 2026-07-16 从本地 `main` 创建 `feature/hy/10157_同步sub2api主线`, 固定合入 `Wei-Shaw/sub2api` `main@d515c3045ce838976ebedab87846aaaf893dbbf6`（包含 `v0.1.156` tag `12f991d` 和紧随其后的 `VERSION` 同步）, 当前后端版本 `0.1.156`。本次上游新增 Agent Identity、管理员安全复制账号、根级 `/models`、native Responses first-output timeout、WS 首消息 timeout、Grok OAuth pool/reconcile、token refresh provider 并发/QPS/熔断; 冲突解决继续保留本地 RequestArchive/RequestIntercept、Prompt Metrics、Token Analysis、组织报表、用户并发、默认 reasoning effort、Prompt Risk/LLM judge 和 quota flusher。
- 2026-07-14 从本地 `main` 创建 `feature/hy/10155_同步sub2api主线`, 合并 `Wei-Shaw/sub2api` `main` 到 `7c717365ef728e53cdcf6d639a4dd68226db03b2`, 当前后端版本 `0.1.155`。本次上游增加管理端 opt-in Server-Timing、OpenAI 账号级长上下文计费、Responses namespace 可逆摊平、图片非流式 keepalive/终态修正、Grok Web SSO 导入与渠道监控、Ops host 筛选、Codex manifest failover 和调度全量重建并发修复; 冲突解决继续保留本地 Redis 7+/Memurai 启动校验、Prompt Metrics、Prompt Risk/LLM judge 与内容审核配置, 并审查了 23 个双方同时修改的自动合并文件。
- 2026-07-13 基于 `feature/hy/10151_同步sub2api主线` 创建 `feature/hy/10153_同步sub2api主线`, 固定同步 `Wei-Shaw/sub2api` 到 `55ed0ab0da367183d97c15659e33ae9e83f6ff90`, 当前后端版本 `0.1.153`; 明确不包含其后的 `7d239d62e`。本次上游增加 OpenAI WS ingress 空闲/API Key 连接上限、Grok API Key 与视频编辑/扩展、alpha search 按次计费、API Key 最新 IP 索引、Responses `additional_tools`/Read 流式参数/stop reason 兼容、嵌入式静态资源长缓存、Apple container、账号 `plan_type` 和 DataTable 小列表非虚拟化等能力; 冲突解决继续保留本地 RequestArchive/RequestIntercept、Responses→Chat options adapter、`ConcurrencyCacheError` 和 OpenAI-compatible provider preset。
- 2026-07-13 同步 `Wei-Shaw/sub2api` `main` 到 `feature/hy/10151_同步sub2api主线`, 当前后端版本 `0.1.151`。本次上游引入 Responses/Chat 的 custom、namespace、tool_search 工具桥, Codex alpha search 与 identity 修复, 用户级 Fast/Flex 策略, Grok Free OAuth prompt cache/Chat bridge/quota recovery, compact keepalive 加固和 Responses/Anthropic cache creation 透传; 冲突解决继续保留 RequestArchive/RequestIntercept、Token Analysis、图片生成、敏感词过滤、用户并发、第三方 Responses->Chat options 过滤和 OpenAI-compatible cache usage。

## 文档地图

- `backend.md`: 后端入口, 路由, Wire 依赖注入, service/repository 分层, 网关路径。
- `frontend.md`: Vue 前端入口, 路由守卫, store, API client, 组件和样式约定。
- `ops.md`: 本地启动, 构建, 测试, CI, 配置和部署入口。
- `data-and-domain.md`: 核心领域对象, Ent schema, SQL migration, 支付/订阅/计费知识。
- `security-and-reliability.md`: 认证, 权限, 限流, 幂等, CSP, URL allowlist, 网关可靠性。
- `ai-workflow.md`: Codex/Copilot 日常如何读取和更新 llm-wiki。

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
