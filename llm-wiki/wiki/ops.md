# 运维, 配置与验证基线

## 本地启动

Windows 本地脚本:

- 启动: `start-local.cmd`
- 停止: `stop-local.cmd`
- PowerShell 启动: `powershell -ExecutionPolicy Bypass -File .\tools\start-local.ps1`
- PowerShell 停止: `powershell -ExecutionPolicy Bypass -File .\tools\stop-local.ps1`

当前本地脚本假设:

- PostgreSQL: `127.0.0.1:5432`
- Redis 或 Memurai: `127.0.0.1:6379`
- 后端监听 `0.0.0.0:8080`
- 本地环境文件: `.localdev/local-env.ps1`
- 日志: `.localdev/logs/backend.log`, `.localdev/logs/frontend.log`
- 后端 data/config: `.localdev/backend-data/config.yaml`

开发模式手动启动:

```bash
cd backend
go run ./cmd/server
```

```bash
cd frontend
pnpm run dev
```

## 构建

根目录:

```bash
make build
```

后端:

```bash
cd backend
make build
```

前端:

```bash
pnpm --dir frontend run build
```

前端构建产物输出到 `backend/internal/web/dist`, 后端使用 embed tag 打包前端。

## 验证命令

后端:

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
```

前端:

```bash
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run
```

根目录 Makefile:

```bash
make test-backend
make test-frontend
make test
```

Windows 没有 make 时, 直接运行 Makefile 内对应原始命令。

## CI

`.github/workflows/backend-ci.yml`:

- 后端单元测试: `make test-unit`
- 后端集成测试: `make test-integration`
- 前端: pnpm 9, Node 20, `pnpm install --frozen-lockfile`, `make test-frontend`
- golangci-lint: `golangci/golangci-lint-action@v9`, version `v2.9`, working-directory `backend`
- Go 版本校验: `go1.26.4`

`.github/workflows/security-scan.yml`:

- 后端: `govulncheck ./...`
- 前端: `pnpm audit --prod --audit-level=high --json > audit.json || true`
- audit 例外检查: `tools/check_pnpm_audit_exceptions.py --audit frontend/audit.json --exceptions .github/audit-exceptions.yml`

`.github/workflows/release.yml` 负责 tag `v*` 发布。

## 配置文件

示例配置: `deploy/config.example.yaml`。

主要配置组:

- `server`: host, port, mode, frontend_url, trusted_proxies, h2c, request body 上限。
- `run_mode`: `standard` 或 `simple`。
- `cors`: allowed origins 和 credentials。
- `security`: URL allowlist, response headers, CSP, proxy probe, proxy fallback。
- `gateway`: 上游超时, body size, request archive, request intercept, OpenAI WS, 调度, usage record, connection pool, Codex bridge。
- `gateway.openai_scheduler`: OpenAI sticky session 逃逸配置; 默认开启, 可按 TTFT/error rate 跳过劣化 sticky 账号。
- `gateway.openai_ws`: OpenAI Responses WebSocket v2 和 HTTP bridge 配置; 首包较大时可保持客户端 WS, 改用 HTTP Responses 上游。
- `database`: PostgreSQL 连接池。
- `database.user_platform_quota_flusher_*`: user x platform quota 写聚合 flusher 配置; 默认关闭, 开启时必须考虑多实例 leader lock。
- `redis`: Redis 连接池和 TLS。
- `ops`: 运维监控开关。
- `jwt`, `totp`: 登录和 2FA 安全配置。
- OAuth: LinuxDo, WeChat, OIDC, DingTalk, GitHub, Google。
- `pricing`: 模型价格远程源, hash 校验和 fallback 文件。
- `billing`: 计费熔断。
- `gemini`: OAuth 和本地 quota 模拟。

修改配置要同步代码默认值, 示例配置, 校验逻辑和文档。

## 部署入口

- Dockerfile: 根目录 `Dockerfile`, `deploy/Dockerfile`。
- Compose: `deploy/docker-compose.yml`, `deploy/docker-compose.local.yml`, `deploy/docker-compose.dev.yml`, `deploy/docker-compose.standalone.yml`。
- systemd: `deploy/sub2api.service`, `deploy/sub2api-datamanagementd.service`。
- 安装脚本: `deploy/install.sh`, `deploy/docker-deploy.sh`, `deploy/install-datamanagementd.sh`。
- Caddy 示例: `deploy/Caddyfile`。

## 常见维护陷阱

- `frontend/package.json` 改依赖后必须更新并提交 `frontend/pnpm-lock.yaml`。
- Ent schema 改动必须生成 Ent 和 Wire。
- 已应用 migration 不可修改, 只能新增 migration。
- 高风险接口新增写操作时要考虑 `Idempotency-Key`。
- 修改网关 body/stream 逻辑要验证流式和非流式两类请求。
- 更改 OpenAI WS 或调度配置要检查 fallback, sticky session 和连接池策略。
- 修改 Wire provider 或后台服务启动/清理逻辑后运行 `cd backend && go generate ./cmd/server` 与 `go test ./cmd/server -run Wire`。
