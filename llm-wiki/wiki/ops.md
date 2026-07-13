# 运维, 配置与验证基线

## 当前版本基线

- 当前合并后的 `backend/cmd/server/VERSION` 为 `0.1.153`; 对应固定上游提交 `55ed0ab0da367183d97c15659e33ae9e83f6ff90`, 不包含其后的 `7d239d62e`。
- `backend/go.mod` 声明 Go `1.26.5`; CI、Dockerfile 和 release workflow 的 Go 版本引用应保持 `go1.26.5`。
- Wire provider 或后台服务签名变动后, 在 Windows 上建议使用仓库内 `GOCACHE`/`GOTMPDIR` 重新生成并测试, 避免默认 Go build cache 权限噪音。

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

embed 模式会给 Vite `assets/`、`logo.png` 和 `favicon.ico` 设置一年 `immutable` 缓存, HTML/SPA fallback 仍为 no-cache。根级 API `/alpha/search` 和 `/videos/*` 必须由 `shouldBypassEmbeddedFrontend` 旁路, 不能回退为 SPA HTML。更改资源路径、根级 API 或 Vite 文件名策略时要同步 `backend/internal/web/embed_on.go`、`static_cache.go` 与测试。

## Apple container

Apple 芯片 Mac + macOS 26 可使用 Apple `container` 1.1.0+ 运行本地 Sub2API/PostgreSQL/Redis。入口为 `deploy/apple-container.sh`, 完整限制、持久化和升级说明在 `deploy/APPLE_CONTAINER.md`:

```bash
cd deploy
./apple-container.sh init
./apple-container.sh up
./apple-container.sh status
```

该脚本面向本地开发和人工运维, 不提供生产级持续重启监管。shell 语法与 fixture 测试由 `.github/workflows/backend-ci.yml` 的 macOS `shell` job 执行 `/bin/bash -n deploy/apple-container.sh` 和 `/bin/bash deploy/tests/apple-container-test.sh`; Windows 本机没有 bash 时可依赖该 CI 关卡, 不要用 PowerShell 解释脚本。

## 生产源码部署

生产服务器源码部署基线:

- 代码目录: `/opt/sub2api`
- 部署方式: 源码 + systemd, 不是 Docker
- 应用服务: `sub2api`
- systemd 启动二进制: `/opt/sub2api/backend/bin/server`
- systemd 工作目录: `/opt/sub2api/backend`
- 禁止操作: 不重启 PostgreSQL, Redis, Nginx; 不修改服务器 Codex 登录或认证配置。

上线前只读检查:

```bash
cd /opt/sub2api
git status --short --branch
git fetch origin
git log --oneline -n 3 origin/main
```

如果工作树不干净, 立即停止上线。确认干净后再切换和拉取目标分支, 例如部署 `origin/main`:

```bash
cd /opt/sub2api
git checkout main
git pull --ff-only origin main
```

生产构建顺序必须先前端、后后端:

```bash
cd /opt/sub2api
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run build

cd /opt/sub2api/backend
CGO_ENABLED=0 go build -tags embed -ldflags="-s -w -X main.Version=$(tr -d '\r\n' < ./cmd/server/VERSION)" -trimpath -o bin/server ./cmd/server
chown sub2api:sub2api /opt/sub2api/backend/bin/server
chmod 0750 /opt/sub2api/backend/bin/server
```

关键原因:

- 前端必须先构建到 `backend/internal/web/dist`, 否则后端二进制不会嵌入最新前端产物。
- 后端必须使用 `-tags embed`, 否则会编入 `backend/internal/web/embed_off.go`, 页面可能返回 `Frontend not embedded. Build with -tags embed to include frontend.`
- root 构建后要修正 `/opt/sub2api/backend/bin/server` 的 owner 和 mode, 否则 systemd 以 `sub2api` 用户启动时可能报 `status=203/EXEC` 和 `Permission denied`。

仅重启应用服务:

```bash
systemctl restart sub2api
```

重启后检查:

```bash
systemctl status sub2api --no-pager
journalctl -u sub2api -n 120 --no-pager
ss -lntp | grep sub2api || ss -lntp | grep -E '8080|5299'
curl -fsS http://127.0.0.1:52997/ | head -5
```

`curl` 返回 `<!doctype html>` 和 `<html lang="zh-CN">` 说明前端已正确嵌入。`systemctl restart sub2api` 会进入应用启动流程, 应用启动会自动执行 `backend/migrations` 中未执行的 SQL migration; 不要手动处理数据库。若启动失败, 先看 `journalctl -u sub2api -n 120 --no-pager`。

## 验证命令

后端:

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
```

Windows 本地 Go 验证固定入口:

- 不直接用默认 `go env`; 本机默认 `GOMODCACHE` 可能指向 `D:\project\pkg\mod`, 默认 `GOCACHE` 可能指向用户级 `go-build`, 容易出现 `Access is denied` 或 `.test.exe` 文件锁。
- 固定复用 `backend/.gocache/review-cache` 作为 `GOCACHE`, `backend/.gocache/review-gopath/pkg/mod` 作为 `GOMODCACHE`; 这样第二次以后不需要重新下载 toolchain 和模块。
- 每一轮测试必须使用新的 `GOTMPDIR`(`backend/.gocache/run-tmp-*`), 不复用上一次的临时目录。Windows 可能短暂占用 `*.test.exe`; 如果复用同一个 `GOTMPDIR`, 下一次容易继续失败。
- 串行运行 `-p 1 -count=1`; 全包很慢时先跑 smoke 包, 再按风险面追加包。
- `backend/.gocache` 里若存在历史 `00` 到 `ff` 分片目录、`review-tmp` 或固定 `run-tmp`, 视为旧实验缓存; 稳定入口只依赖 `review-cache`, `review-gopath` 和每轮新建的 `run-tmp-*`。

```powershell
$ErrorActionPreference = "Stop"
$repo = (git -C . rev-parse --show-toplevel).Trim()
$backend = Join-Path $repo "backend"
$cacheRoot = Join-Path $backend ".gocache"
$env:GOCACHE = Join-Path $cacheRoot "review-cache"
$env:GOPATH = Join-Path $cacheRoot "review-gopath"
$env:GOMODCACHE = Join-Path $env:GOPATH "pkg\mod"

$resolvedRepo = (Resolve-Path -LiteralPath $repo).Path
foreach ($path in @($env:GOCACHE, $env:GOPATH, $env:GOMODCACHE)) {
  $parent = Split-Path -Parent $path
  if (-not (Test-Path -LiteralPath $parent)) {
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
  }
  $candidate = if (Test-Path -LiteralPath $path) { (Resolve-Path -LiteralPath $path).Path } else { $path }
  if (-not $candidate.StartsWith($resolvedRepo + [System.IO.Path]::DirectorySeparatorChar)) {
    throw "Refusing path outside workspace: $candidate"
  }
}
New-Item -ItemType Directory -Force -Path $env:GOCACHE,$env:GOMODCACHE | Out-Null

# 只清理临时执行目录; 不清 review-cache / review-gopath, 否则下次会重新下载和编译。
Get-ChildItem -LiteralPath $cacheRoot -Directory -Filter "run-tmp-*" -ErrorAction SilentlyContinue |
  Remove-Item -Recurse -Force

Set-Location $backend
$packages = @(
  "./internal/config",
  "./internal/server/routes",
  "./internal/handler"
)

for ($attempt = 1; $attempt -le 3; $attempt++) {
  $env:GOTMPDIR = Join-Path $cacheRoot ("run-tmp-{0}-{1}" -f (Get-Date -Format "yyyyMMddHHmmss"), $attempt)
  New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
  Write-Host "attempt=$attempt GOTMPDIR=$env:GOTMPDIR"

  go test -tags=unit -p 1 -count=1 @packages
  if ($LASTEXITCODE -eq 0) {
    break
  }

  Start-Sleep -Seconds 2
}

if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}
```

常用包组合:

| 场景 | 包 |
| --- | --- |
| 后端 smoke / 合并后路由和 handler 基线 | `./internal/config ./internal/server/routes ./internal/handler` |
| 网关/service 高风险改动 | `./internal/service` |
| repository 或 SQL 相关改动 | `./internal/repository ./internal/config` |
| Wire/provider 变更 | `./cmd/server ./internal/handler ./internal/server/routes` |

如果出现 `fork/exec ... *.test.exe: The process cannot access the file because it is being used by another process.`, 不要改业务代码, 也不要切回默认 Go cache。确认没有残留 `go.exe` / `*.test.exe` 进程后, 用上面的固定入口重跑; 它会换新的 `GOTMPDIR`。只有怀疑缓存损坏时才删除 `backend/.gocache/review-cache` 或 `backend/.gocache/review-gopath`, 删除后首次运行会重新下载 Go toolchain 和模块。

```powershell
Get-CimInstance Win32_Process |
  Where-Object { $_.Name -match "^(go|compile|link|vet|.*\.test)\.exe$" } |
  Select-Object ProcessId,ParentProcessId,Name,CommandLine
```

不要在已经 `cd backend` 后再写 `Join-Path (Get-Location) "backend\..."`, 这会生成 `backend/backend` 缓存目录; 该目录可能被 Go/gopls 暂时锁住, 清理会反复失败并污染后续状态判断。

2026-07-09 重启后本机复测: 上述固定入口已验证 `go test -tags=unit -p 1 -count=1 ./internal/config ./internal/server/routes ./internal/handler` 通过。复测过程中固定复用同一个 `GOTMPDIR` 曾触发 `config.test.exe` 文件占用; 改为 fresh `run-tmp-*` 后通过。前端同步验证 `cmd.exe /c pnpm --dir frontend run typecheck` 通过。全包后端测试日志若混入预期 error 日志, 只以 `^--- FAIL:` / `^FAIL` 判定真实失败。
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

- Apple container shell: macOS 15, 检查脚本语法并运行 fixture test。
- 后端单元测试: `make test-unit`
- 后端集成测试: `make test-integration`
- 前端: pnpm 9, Node 20, `pnpm install --frozen-lockfile`, `make test-frontend`
- golangci-lint: `golangci/golangci-lint-action@v9`, version `v2.9`, working-directory `backend`
- Go 版本校验: `go1.26.5`

`.github/workflows/security-scan.yml`:

- 后端: `govulncheck ./...`
- 前端: `pnpm audit --prod --audit-level=high --json > audit.json || true`
- audit 例外检查: `tools/check_pnpm_audit_exceptions.py --audit frontend/audit.json --exceptions .github/audit-exceptions.yml`

`.github/workflows/release.yml` 负责 tag `v*` 发布。管理端版本徽章可查询最近 3 个 GitHub release 并触发在线回退; 服务端实现位于 `internal/service/update_service.go` 和 `internal/repository/github_release_service.go`, 回退属于高风险系统操作, 必须保留管理员校验、目标版本校验和执行日志。

## 配置文件

示例配置: `deploy/config.example.yaml`。

主要配置组:

- `server`: host, port, mode, frontend_url, trusted_proxies, h2c, request body 上限。
- `run_mode`: `standard` 或 `simple`。
- `cors`: allowed origins 和 credentials。
- `security`: URL allowlist, response headers, CSP, proxy probe, proxy fallback。
- `gateway`: 上游超时, body size, request archive, request intercept, OpenAI WS, 调度, usage record, connection pool, Codex bridge。
- 管理端运行时设置 `enable_client_dateline_normalization` 默认 `true`, 仅影响 Anthropic OAuth/SetupToken 转发, 用于清理客户端 dateline 隐写指纹; 关闭后请求体保持原样透传。
- `gateway.openai_ws.scheduler_score_weights.reset`: 默认 `0.0`, 用于给会话窗口最早重置的 OpenAI 账号加分; 关闭时不改变原调度行为。
- `gateway.openai_ws.scheduler_score_weights.quota_headroom`: 默认 `0.0`, 用于按 OpenAI/Codex 7d 剩余额度健康度给账号加分; 关闭时不改变原调度行为, 小流量灰度可从 `0.3` 起。
- `gateway.openai_scheduler`: OpenAI sticky session 逃逸配置; 默认开启, 可按 TTFT/error rate 跳过劣化 sticky 账号。
- `gateway.openai_compact_model`: OpenAI `/responses/compact` 上游默认模型, 默认 `gpt-5.4`; 可在 compact endpoint 暂未支持新模型时临时降级, 不影响普通 `/v1/responses`。
- `gateway.scheduling.prefer_soonest_reset`: 默认 `false`, 开启后负载感知调度优先选用会话窗口最早重置账号。
- `gateway.openai_ws`: OpenAI Responses WebSocket v2 和 HTTP bridge 配置; 首包较大时可保持客户端 WS, 改用 HTTP Responses 上游; `ingress_mode_default` 支持 `off|ctx_pool|passthrough|http_bridge`, 旧值 `shared/dedicated` 按 `ctx_pool` 兼容。
- `gateway.openai_ws.ingress_inter_turn_idle_timeout_seconds`: completed turn 之间的客户端空闲上限, 默认 300 秒, 0 关闭, 负数配置拒绝启动。
- `gateway.openai_ws.max_ingress_connections_per_api_key`: 多实例范围每个 API Key 的存活 ingress 连接上限, 默认 64, 0 关闭; 依赖 Redis lease, 缓存不支持或 lease 丢失时 fail-close。
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

## 上游历史分支合并注意事项

- `upstream/revert-114-feature/atomic-scheduling` 是 Wei-Shaw/sub2api 在 2026-01-01 创建的旧分支, 单提交 `30326cf2671a` 用于撤销早期 `#114` 负载感知账号调度优化。
- 当前分支历史已包含后续主线的同内容 revert `c5c12d4c8`, 也包含后续 reapply `7568dc850`; 当前 `GatewaySchedulingConfig`, `SelectAccountWithLoadAwareness`, `ConcurrencyService.GetAccountsLoadBatch`, scheduler snapshot/outbox 与 wait plan 代码是后续演进后的稳定实现。
- 以后若再次合并该元旦分支或等价历史分支, 冲突处理不应整块接受该旧 revert 的删除侧, 否则会回退当前网关调度、OpenAI/Gemini/Grok 路由选择和并发缓存能力。应先核对 `git log --grep "Reapply.*负载感知"` 与当前调用点, 再决定是否只补齐 merge 拓扑。
