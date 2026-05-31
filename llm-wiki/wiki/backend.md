# 后端知识基线

## 技术栈与入口

- Go module: `github.com/Wei-Shaw/sub2api`, 当前 `backend/go.mod` 声明 Go `1.26.3`。
- HTTP 框架: Gin。
- ORM: Ent, 生成代码位于 `backend/ent/`, schema 位于 `backend/ent/schema/`。
- 依赖注入: Google Wire, 入口在 `backend/cmd/server/wire.go`, 生成文件 `wire_gen.go`。
- 后端主入口: `backend/cmd/server/main.go`。
- 主要命令:
  - 启动: `cd backend && go run ./cmd/server`
  - 构建: `cd backend && make build`
  - 生成: `cd backend && make generate`

## 启动流程

`backend/cmd/server/main.go`:

1. 初始化 bootstrap logger。
2. 解析 `-setup` 和 `-version`。
3. 若首次运行需要 setup:
   - Docker/环境变量自动 setup 时调用 `setup.AutoSetupFromEnv()`。
   - 否则启动 setup wizard 路由。
4. 正常服务模式:
   - `config.LoadForBootstrap()` 读取配置。
   - 初始化 logger。
   - Wire 构建 `Application`。
   - 启动 HTTP server。
   - 监听 `SIGINT/SIGTERM`, 优雅关闭。

## 依赖注入结构

`backend/cmd/server/wire.go` 的 `initializeApplication` 汇总:

- `config.ProviderSet`
- `repository.ProviderSet`
- `service.ProviderSet`
- `payment.ProviderSet`
- `middleware.ProviderSet`
- `handler.ProviderSet`
- `server.ProviderSet`

很多后台服务在 Provider 中自动 `Start()`, 例如 token refresh, dashboard aggregation, usage cleanup, ops collector, scheduled report, account/subscription expiry, channel monitor runner。新增后台服务时要同时考虑 Wire 注入, 启动时机和 `provideCleanup` 停止逻辑。

## HTTP Server 与路由

入口:

- `backend/internal/server/http.go`: 提供 Gin engine 和 `http.Server`。
- `backend/internal/server/router.go`: 注册全局中间件和业务路由。
- `backend/internal/server/routes/*.go`: 分组注册路由。

全局中间件包含 recovery, request logger, CORS, security headers, embedded frontend。`server.trusted_proxies` 为空时会禁用可信代理解析。

路由主分组:

- `/api/v1/auth`: 注册, 登录, OAuth, refresh, logout, pending auth。
- `/api/v1/user`, `/api/v1/keys`, `/api/v1/usage`, `/api/v1/redeem`, `/api/v1/subscriptions`: 用户侧接口。
- `/api/v1/admin`: 管理端接口, 由 admin auth 保护。
- `/v1`, `/v1beta`, `/responses`, `/chat/completions`, `/images/*`: AI 网关兼容接口。
- `/antigravity/v1`, `/antigravity/v1beta`: Antigravity 专用兼容接口。

## 网关路径

`backend/internal/server/routes/gateway.go` 是网关路由入口。

关键行为:

- `/v1/messages` 根据 API Key 所属 group platform 分流到 OpenAI 或 Claude 兼容处理。
- `/v1/messages/count_tokens` 对 OpenAI group 返回不支持。
- `/v1/responses` 和根级 `/responses` 支持 OpenAI Responses API。
- `/v1/chat/completions` 和根级 `/chat/completions` 支持 OpenAI Chat Completions。
- `/v1/embeddings` 和根级 `/embeddings` 仅 OpenAI platform 支持。
- `/v1/images/generations` 和 `/v1/images/edits` 仅 OpenAI platform 支持。
- `/v1beta/models/*` 提供 Gemini SDK/CLI 兼容。
- `/backend-api/codex/responses` 支持 Codex 直连别名。

OpenAI 上游请求会按官方 endpoint 做字段过滤:

- `/v1/responses` / Responses 透传路径删除 top-level `thinking`, 保留官方 `reasoning`。
- `/v1/chat/completions` raw 直转路径删除 top-level `thinking`, 保留官方 `reasoning_effort`。
- Anthropic/Gemini 等非 OpenAI 协议的 thinking 映射不复用该过滤规则, 需按各自协议能力单独处理。
- OpenAI Responses SSE 终止事件的 usage 可能在顶层 `usage` 或 `response.usage`; Chat Completions 和 Messages 的 buffered/streaming 转换及计费解析必须按实际 JSON 路径保留 `input_tokens_details.cached_tokens`、`cache_read_input_tokens` 等缓存 token 字段。

网关链路常见中间件:

- `RequestBodyLimit`
- `ClientRequestID`
- `OpsErrorLoggerMiddleware`
- `InboundEndpointMiddleware`
- `APIKeyAuth`
- `RequireGroupAssignment`
- `RequestArchive`
- `RequestIntercept`

`RequestArchive`(`backend/internal/server/middleware/request_archive.go`)把网关请求/响应体写入本地 JSONL, 仅用于短期排障:

- 默认关闭: `gateway.request_archive.enabled=false`, `capture_response=false`(与 `backend/internal/config/config.go` 默认一致)。
- 开启后位于请求热路径, 会 `io.ReadAll` 完整请求体并缓存响应体, 高并发/大 body/流式会放大磁盘与尾延迟, 单日文件可达 GB 级。
- 写入为异步有界队列(`gateway.request_archive.queue_size`, 默认 1024): 热路径只入队, 后台单 goroutine 持有当日文件句柄并按日期轮转, 队列满时丢弃记录不阻塞请求。
- 管理后台 `/admin/settings` 的 Gateway 标签页可热切换 `enabled` 和 `capture_response`, 后端接口为 `GET/PUT /api/v1/admin/settings/request-archive`; `dir`, body 截断上限和 `queue_size` 仍由实际加载的 `config.yaml` 控制, 修改后需重启。

端点归一化集中在 `backend/internal/handler/endpoint.go`。新增网关端点时要同步常量, `NormalizeInboundEndpoint`, `DeriveUpstreamEndpoint`, 路由注册和相关 OpenAI/Claude/Gemini 分流测试。

## 分层约定

- `internal/handler`: HTTP 请求绑定, 参数校验, 调用 service, 返回响应。
- `internal/service`: 业务逻辑和跨 repository 编排。
- `internal/repository`: Ent/SQL/Redis/外部 HTTP 访问实现。
- `internal/server/middleware`: 服务端通用 HTTP 中间件。
- `internal/payment`: 支付抽象, provider 注册和负载均衡。
- `internal/pkg`: 可复用基础设施包, 如 logger, errors, openai, gemini, proxyutil。

新增能力时优先按现有层次放置, 不要让 handler 直接承载复杂业务逻辑。

## 配置加载

`backend/internal/config/config.go` 使用 Viper:

- 支持 `config.yaml`, `./config`, `/etc/sub2api` 等路径。
- 支持环境变量, `.` 会映射为 `_`。
- `LoadForBootstrap()` 用于启动前完整配置。
- `GetServerAddress()` 是 setup 前的轻量读取。

配置结构很大, 修改配置必须同步:

- `Config` 结构体。
- 默认值。
- 校验逻辑。
- `deploy/config.example.yaml`。
- 必要时更新前端 settings 类型和 UI。

## 数据访问

`backend/internal/repository/ent.go`:

- `InitEnt` 初始化 PostgreSQL Ent client。
- 启动时自动执行 embedded migrations。

`backend/internal/repository/wire.go`:

- 集中注册 repository, cache, encryptor, external clients。
- `ProvideSQLDB` 从 Ent driver 暴露 `*sql.DB`, 用于复杂 SQL。
- `ProvideRedis` 调用 `InitRedis`。

## 生成代码

修改以下内容后通常要生成:

- `backend/ent/schema/*.go`: 运行 `cd backend && go generate ./ent`。
- Wire provider: 运行 `cd backend && go generate ./cmd/server` 或 `make generate`。

生成代码必须随源码提交, 否则 CI/编译可能失败。
