# 运维, 配置与验证基线

## 当前版本基线

- 当前合并分支为 `feature/hy/10162_合并1.162版本`, merge commit 是 `ea26f2b0755323dcd750dbdb01cb35991a396be7`, 第一父/本地 `main` 基线是 `e52b5c89d07ac058043de5adb983cad8750cab58`, 第二父是 `Wei-Shaw/sub2api main@e625ce3b3b3b955b7c3afc93221f7c5f0ae55aa8`, `backend/cmd/server/VERSION` 为 `0.1.162`。`v0.1.162` tag `27f094e09` 本身仍写 `0.1.161`, 不可用 tag target 代替后续 version-sync commit。
- `backend/go.mod` 声明 Go `1.26.5`; CI、Dockerfile 和 release workflow 的 Go 版本引用应保持 `go1.26.5`。
- Wire provider 或后台服务签名变动后, 在 Windows 上建议使用仓库内 `GOCACHE`/`GOTMPDIR` 重新生成并测试, 避免默认 Go build cache 权限噪音。`backend/cmd/server/main.go` 的生成指令固定为 `go run -mod=mod github.com/google/wire/cmd/wire`; 干净模块缓存下缺少 `-mod=mod` 会因 Wire 工具传递依赖缺少 `go.sum` 条目而失败。
- 0.1.162 继续保留 `securityaudit.ProviderSet` 的 `PromptAdminService -> *PromptService` binding；`go generate ./cmd/server` 应从合并后的 Wire 源图同时生成上游 Ops/auth-cache/image-storage 生命周期与本地 Prompt Metrics、Token Analysis、并发 preset、quota flusher 链。
- `frontend/src/i18n/__tests__/localesMessageCompile.spec.ts` 使用的 `@intlify/message-compiler@9.14.5` 已由上游补入 `frontend/package.json` 与 lockfile；Windows 完整前端验证使用 `corepack pnpm@9.15.9` 读取 lockfile v9。

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

embed 模式只给 Vite `assets/` 下文件名带 8 字符 fingerprint 的资源设置一年 `immutable` 缓存; unhashed assets、`logo.svg`、`favicon.ico`、HTML 和 SPA fallback 不使用静态长缓存。`deploy/Caddyfile` 只负责 TLS/反向代理, 不重复按路径强制 immutable, fingerprint 判定由后端 `static_cache.go` 统一负责。根级 API `/alpha/search` 和 `/videos/*` 必须由 `shouldBypassEmbeddedFrontend` 旁路, 不能回退为 SPA HTML。更改资源路径、根级 API 或 Vite 文件名策略时要同步 `backend/internal/web/embed_on.go`、`static_cache.go` 与测试。

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

如果同一测试二进制在多个 fresh `GOTMPDIR` 中持续 `Access is denied`, 且连 `Get-Acl` 或只读文件句柄都被拒绝, 应通过 `Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct` 核对第三方安全软件, 不要只查 Windows Defender。未经授权不得关闭杀软或添加白名单; 也不能把 `go list -tags=...` 的文件集合等价、其他 tag 通过或 `go test -c` 编译成功写成被阻断命令的“通过”。交付记录必须分别列出完整命令的通过包、被阻断包、独立重跑和环境证据。

Repository 的纯 PostgreSQL integration 可显式复用外部临时数据库, 不启动 Testcontainers PostgreSQL/Redis。仅在 `-run` 已限定为不依赖 Redis 的数据库测试时设置 `SUB2API_POSTGRES_ONLY_INTEGRATION_DSN`; 默认未设置时仍使用 CI 的 Docker Testcontainers 完整路径。DSN 只通过当前进程环境变量传入, 不写入仓库或测试日志。

```powershell
$env:SUB2API_POSTGRES_ONLY_INTEGRATION_DSN = "host=127.0.0.1 port=55432 user=postgres dbname=postgres sslmode=disable"
go test -tags=integration -p 1 -count=1 ./internal/repository -run OrganizationUsage -v
Remove-Item Env:SUB2API_POSTGRES_ONLY_INTEGRATION_DSN
```

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

`.github/workflows/release.yml` 负责 tag `v*` 发布。管理端版本徽章可查询最近 3 个 GitHub release 并触发在线回退; 服务端实现位于 `internal/service/update_service.go` 和 `internal/repository/github_release_service.go`, 回退属于高风险系统操作, 必须保留管理员校验、目标版本校验和执行日志。可选环境变量 `UPDATE_GITHUB_TOKEN` 只给 `https://api.github.com` release 查询附加 Bearer token, `GITHUB_TOKEN` / `GH_TOKEN` 不复用；release asset 与 checksum 下载保持匿名, API redirect 离开该 host 时删除 Authorization。

## 配置文件

示例配置: `deploy/config.example.yaml`。

主要配置组:

- `server`: host, port, mode, frontend_url, trusted_proxies, h2c, request body 上限; `read_header_timeout=10`、`max_header_bytes=65536`、`idle_timeout=120` 构成 HTTP ingress 基线, 不设置会截断 SSE/WS 的 `WriteTimeout`; `enable_server_timing` 默认 `false`, 也可用精确环境变量 `ENABLE_SERVER_TIMING=true` 开启管理端可观测响应头。
- `run_mode`: `standard` 或 `simple`。
- `cors`: allowed origins 和 credentials。
- `security`: URL allowlist, response headers, CSP, proxy probe, proxy fallback, `trust_forwarded_ip_for_api_key_acl` 与 `forwarded_client_ip_headers`。客户端 IP 兼容开关默认 `true` 以兼容反代/Docker 升级；自定义 header 最多 16 个、按配置顺序优先, 仅在该开关开启时有效。关闭后必须正确配置 `server.trusted_proxies`。
- `gateway`: 上游超时, body size, request archive, request intercept, OpenAI WS, 调度, usage record, connection pool, Codex bridge。`max_body_size` 默认 256 MiB 供多模态/media, `text_max_body_size` 默认 32 MiB 并只用于 embeddings 与 alpha search 等纯文本入口。
- 管理端运行时设置 `enable_client_dateline_normalization` 默认 `true`, 仅影响 Anthropic OAuth/SetupToken 转发, 用于清理客户端 dateline 隐写指纹; 关闭后请求体保持原样透传。
- `gateway.openai_ws.scheduler_score_weights.reset`: 默认 `0.0`, 用于给会话窗口最早重置的 OpenAI 账号加分; 关闭时不改变原调度行为。
- `gateway.openai_ws.scheduler_score_weights.quota_headroom`: 默认 `0.0`, 用于按 OpenAI/Codex 7d 剩余额度健康度给账号加分; 关闭时不改变原调度行为, 小流量灰度可从 `0.3` 起。
- `gateway.openai_scheduler`: OpenAI sticky session 逃逸配置; 默认开启, 可按 TTFT/error rate 跳过劣化 sticky 账号。
- `gateway.openai_compact_model`: OpenAI `/responses/compact` 上游默认模型, 默认 `gpt-5.4`; 可在 compact endpoint 暂未支持新模型时临时降级, 不影响普通 `/v1/responses`。
- `gateway.openai_first_output_timeout_seconds`: 默认 `0` 关闭; 非零必须为 30-600 秒, 否则启动校验失败。只保护 native OpenAI HTTP streaming Responses, deadline 包含响应头等待, 不作用于 passthrough/WS; 首次语义输出前单次 attempt 暂存上限 8 MiB, 超时最多切号一次。原 attempt 可能已产生上游用量, 开启后必须接受重复上游计费风险。
- `gateway.openai_high_effort_first_output_timeout_seconds`: 默认 `0`, 表示 high/xhigh/max 继承标准 first-output timeout; 非零必须为 30-1800 秒, 且只有标准 timeout 已启用时才生效。
- `gateway.image_nonstream_keepalive_interval`: OpenAI 非流式图片 JSON 心跳秒数, 默认 `0` 关闭; 非零只允许 5-60 秒。首个心跳会提交 HTTP 200, 开启前必须确认调用方能接受已提交状态后的错误语义。
- `gateway.scheduling.prefer_soonest_reset`: 默认 `false`, 开启后负载感知调度优先选用会话窗口最早重置账号。
- `gateway.openai_ws`: OpenAI Responses WebSocket v2 和 HTTP bridge 配置; 首包较大时可保持客户端 WS, 改用 HTTP Responses 上游; `ingress_mode_default` 支持 `off|ctx_pool|passthrough|http_bridge`, 旧值 `shared/dedicated` 按 `ctx_pool` 兼容。
- `gateway.openai_ws.client_first_message_timeout_seconds`: 默认 30 秒, 必须为正数, 否则启动校验失败; 覆盖首条客户端 `response.create` 的完整读取和解压。大请求或慢链路可调到 120-300 秒, 但值越大会越久占用 ingress 连接和 lease 资源。
- `gateway.openai_ws.ingress_inter_turn_idle_timeout_seconds`: completed turn 之间的客户端空闲上限, 默认 300 秒, 0 关闭, 负数配置拒绝启动。
- `gateway.openai_ws.max_ingress_connections_per_api_key`: 多实例范围每个 API Key 的存活 ingress 连接上限, 默认 64, 0 关闭; 依赖 Redis lease, 缓存不支持或 lease 丢失时 fail-close。
- `database`: PostgreSQL 连接池。
- `database.user_platform_quota_flusher_*`: user x platform quota 写聚合 flusher 配置; 默认关闭, 开启时必须考虑多实例 leader lock。
- `redis`: Redis 连接池和 TLS。
- `api_key_auth_cache`: L1/L2 TTL、singleflight、`lookup_concurrency=64` 和进程内 invalid-auth abuse limiter；默认每可信客户端 IP（IPv6 按 `/64`）60 秒 120 次无效凭据后阻断 60 秒, capacity 16384。它不是 CDN/WAF 的替代品。
- `ops`: 运维监控开关。
- `jwt`, `totp`: 登录和 2FA 安全配置。
- OAuth: LinuxDo, WeChat, OIDC, DingTalk, GitHub, Google。
- `pricing`: 模型价格远程源, hash 校验和 fallback 文件。
- `billing`: 计费熔断。
- `gemini`: OAuth 和本地 quota 模拟。
- `image_storage`: 异步图片任务总开关与 S3-compatible 结果存储; `enabled=true` 仍要求 bucket/access key/secret 完整。`endpoint`, `region`, `prefix`, `public_base_url`, `presign_expiry_hours`, `max_download_bytes` 控制 R2/S3 兼容上传和 URL 结果下载上限。后台 `/api/v1/admin/backups/image-storage` 保存的数据库配置优先且立即生效, 可复用备份 S3；只有从未保存过后台配置时才使用 YAML/env fallback。`IMAGE_STORAGE_*`、`SERVER_TRUSTED_PROXIES` 和 `SECURITY_FORWARDED_CLIENT_IP_HEADERS` 已有 env reachability guard。当前任务 worker 只在进程内运行, 服务重启不会恢复 Redis 中的 `processing` 任务, 可能保留到默认 24h TTL。

Prompt Audit 是数据库运行时设置, 不在 YAML 中新增独立配置组:

- `settings.prompt_audit_config` 保存配置版本、启用/阻断开关、worker/queue、group 范围、scanner 和 OpenAI-compatible Guard endpoints；节点 token 以密文保存。`risk_control_enabled` 是上层总开关。
- 默认 `enabled=false`, 有效模式为 off / async audit / blocking。配置加载失败只在最近一次可解码的存储意图明确要求 blocking 且 `risk_control_enabled=true` 时 fail-closed；默认关闭或 async intent 不得因 untrusted config 被提升为 blocking。async 原文扫描载荷写 Redis key `sub2api:prompt_audit:payload:<job_id>`, TTL 最长 30 分钟, 完成或非重试失败后主动删除。
- 默认 worker 数 4、queue capacity 32768、节点 timeout 3000 ms、input limit 4000 rune。更新配置时必须带 `expected_config_version`, 多实例通过 PostgreSQL advisory lock 和 Redis version 通知刷新快照。

`token_refresh` 的运行时有效值会对非正数回退默认值、对超过上限的正数封顶:

| 配置 | 默认值 | 运行时上限 | 含义与风险 |
| --- | ---: | ---: | --- |
| `candidate_page_size` | 200 | 1000 | 游标页越大, 单页内存和单周期待处理量越高。 |
| `provider_concurrency` | 4 | 32 | 每 provider 并发 attempt; 过高会放大上游和代理压力。 |
| `provider_qps` | 2 | 100 | 每进程、每 provider QPS; 后台与 admin reconcile 共用 gate。 |
| `provider_failure_threshold` | 3 | 100 | 当前周期连续临时失败熔断阈值; 过高会延迟 provider 隔离。 |
| `attempt_timeout_seconds` | 15 | 300 | 单次刷新 attempt; 还会被分布式刷新锁 lease 的安全余量进一步收紧。 |
| `cycle_timeout_seconds` | 240 | 3600 | 单个后台周期总时限; 到期后保留未完整页游标供后续恢复。 |

候选账号按 ID 游标分页, 各 provider 独立处理和熔断; 一个 provider 失败不阻断其他 provider。调整并发/QPS/超时时要同时评估上游限流、代理容量、周期能否扫完整页和多实例总压力。

修改配置要同步代码默认值, 示例配置, 校验逻辑和文档。

## 部署入口

- Dockerfile: 根目录 `Dockerfile`, `deploy/Dockerfile`。
- Compose: `deploy/docker-compose.yml`, `deploy/docker-compose.local.yml`, `deploy/docker-compose.dev.yml`, `deploy/docker-compose.standalone.yml`。
- systemd: `deploy/sub2api.service`, `deploy/sub2api-datamanagementd.service`。
- 安装脚本: `deploy/install.sh`, `deploy/docker-deploy.sh`, `deploy/install-datamanagementd.sh`。
- Caddy 示例: `deploy/Caddyfile`; 只负责 TLS/反向代理, 不负责静态资源 immutable 分类, 该规则由 embedded backend 按 filename fingerprint 判定。
- Edge 基线见 `deploy/EDGE_SECURITY.md`; bundled Caddyfile 以直连 Caddy 为前提, CDN 前置时必须改用精确 trusted proxy CIDR 与 `{client_ip}`。Dockerfile 使用宿主架构 Go 交叉编译目标镜像, Apple Silicon 构建 amd64 不再依赖 QEMU 执行 Go；`deploy/docker-compose.yml` 的 Redis 多行 command 每行必须保留续行反斜杠, 否则持久化参数不会传给 `redis-server`。

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
