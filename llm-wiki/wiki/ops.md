# 运维, 配置与验证基线

## 当前版本基线

- 当前正在 `feature/hy/10173_merge_sub2api_173` 合并上游 `Wei-Shaw/sub2api main@48eb3766d2da817b171b45bb3036d42575e42b8f`: 本地第一父/合并前 HEAD 为 `ddbb0426bfaa5623e31d588977004a9f62bb4772`, 与上游的 merge base 为 `aac53afe0ef1ae850e2f18b5d2814ac67c835e7e`, `backend/cmd/server/VERSION` 为 `0.1.173`。当前保留 `MERGE_HEAD`, merge commit 待用户审核；不要用标签、远程最新 HEAD 或其他中间提交替代这些精确边界。
- 当前集成分支为 `feature/hy/10171_merge_sub2api_171`: 合入已含 v0.1.170 的本地 `main@7a537cffbbb6455ce2f777e70f369a3912738ebd`, 第二父/功能分支 tip 为 `3547702ff54d324cff2f83ec75bd8d9a501ea68f`, 固定上游边界为 `Wei-Shaw/sub2api main@aac53afe0ef1ae850e2f18b5d2814ac67c835e7e`, 与 main 的 merge base 为上游 `7e2e9ba05026b7126318aa0754c1afa0ac00bc58`, `backend/cmd/server/VERSION` 为 `0.1.171`。合并保持 `MERGE_HEAD` 待用户审核；不要用版本标签、当前 upstream HEAD 或其他中间提交替代该精确提交。
- `feature/hy/10170_merge_upstream_v170` 已通过 PR #33 合入 `main@7a537cff`, 固定上游提交 `7e2e9ba05026b7126318aa0754c1afa0ac00bc58`, 后端版本 `0.1.170`。
- `feature/hy/10168_同步sub2api主线` 的固定上游提交为 `5a6143097db142b72a6fc848c214e97214470bdd`, 后端版本为 `0.1.168`。
- `feature/hy/10161_合并1.161版本@e3e6b52da43a5be351cf59089976759eebc28376` 的 `backend/cmd/server/VERSION` 为 `0.1.161`; 对应固定上游提交 `d4b9797ff72024960a035cf22fdd8f213e149169`。
- `backend/go.mod` 声明 Go `1.26.5`; CI、Dockerfile 和 release workflow 的 Go 版本引用应保持 `go1.26.5`。
- Wire provider 或后台服务签名变动后, 在 Windows 上建议使用仓库内 `GOCACHE`/`GOTMPDIR` 重新生成并测试, 避免默认 Go build cache 权限噪音。`backend/cmd/server/main.go` 的生成指令固定为 `go run -mod=mod github.com/google/wire/cmd/wire`; 干净模块缓存下缺少 `-mod=mod` 会因 Wire 工具传递依赖缺少 `go.sum` 条目而失败。
- 0.1.163 继续保留 `securityaudit.ProviderSet` 的 `PromptAdminService -> *PromptService` binding；`go generate ./cmd/server` 应从合并后的 Wire 源图同时生成上游 Ops/auth-cache/image-storage 生命周期与本地 Prompt Metrics、Token Analysis、并发 preset、quota flusher 链。
- 0.1.168 的 Wire 图还必须包含 `NewPasskeySessionStore`、`NewOptionalJWTAuthMiddleware` 与 Passkey service/handler；冲突处理应修改 provider source 后重新生成, 不直接手改 `wire_gen.go`。`deploy/config.example.yaml` 新增默认关闭的 `webauthn` 配置, 生产启用时必须显式提供 RP ID 和 HTTPS origins。
- 0.1.170 的 Content Moderation service 新增 `ProxyRepository` 注入；Wire 必须从 `backend/internal/service/wire.go` 生成该依赖, 本地测试桩可传 `nil` 表示不启用代理。不要直接手改 `wire_gen.go` 固化构造签名。
- 0.1.171 的 Wire 图还必须包含 Tencent/Aliyun captcha service、`ProvideContentModerationService(..., proxyRepo, ...)`、`ProvideOpenAICodexVersionSyncService` 与 `OpenAICodexVersionSyncService` cleanup。Provider source 改完后运行 `cd backend && go generate ./cmd/server`, 不手工编辑 `wire_gen.go`。
- `frontend/src/i18n/__tests__/localesMessageCompile.spec.ts` 使用的 `@intlify/message-compiler@9.14.5` 已由上游补入 `frontend/package.json` 与 lockfile；Windows 完整前端验证使用 `corepack pnpm@9.15.9` 读取 lockfile v9。
- 前端 `pnpm.overrides` 强制 `postcss@<8.5.18` 升到安全版本, lockfile 当前解析为 `8.5.23`; 修改 override 必须用 pnpm 9 重建 lockfile 并复跑 frontend security audit。仓库本地已移除 `vite-plugin-checker` 及其 Vite 插件配置, 合并上游时依赖清单与 `vite.config.ts` 必须保持一致。
- OpenAI Live 的服务端 attestation 仅在 Apple Silicon macOS 且已安装官方 ChatGPT App 时可用；其他平台正常构建但 capability 返回不可用。`gateway.live.max_session_duration_seconds` 默认 3600, 非正值在配置校验时回落该默认值。

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

bundled `deploy/Caddyfile` 的 `encode` 使用显式 content-type allowlist, 不匹配 `text/event-stream`; 不要改回 `text/*` 或 `encode gzip zstd`, 否则 SSE 可能被压缩并缓冲到流结束。`flush_interval` 保持未设置, 让 Caddy 自动 flush SSE 且客户端断开能继续向上游传播；`deploy/test-caddyfile-cache.sh` 同时守卫唯一 encode block、非 SSE 压缩列表和该 flush 合同。Nginx 同样要把 `text/event-stream` 排除在 `gzip_types` 外。

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

Ent schema 或 Wire provider/lifecycle 变化后必须从生成源刷新产物:

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

从仓库根目录可等价运行 `make -C backend generate`；生成失败时修复 schema/provider source 或工具依赖, 不直接手改 `backend/ent/` 与 `backend/cmd/server/wire_gen.go`。

Windows 本地 Go 验证固定入口:

- 不直接用默认 `go env`; 本机默认 `GOMODCACHE` 可能指向 `D:\project\pkg\mod`, 默认 `GOCACHE` 可能指向用户级 `go-build`, 容易出现 `Access is denied` 或 `.test.exe` 文件锁。
- 固定复用 `backend/.gocache/review-cache` 作为 `GOCACHE`, `backend/.gocache/review-gopath/pkg/mod` 作为 `GOMODCACHE`; 这样第二次以后不需要重新下载 toolchain 和模块。
- 每一轮测试必须使用新的 `GOTMPDIR`(`backend/.gocache/run-tmp-*`), 不复用上一次的临时目录。Windows 可能短暂占用 `*.test.exe`; 如果复用同一个 `GOTMPDIR`, 下一次容易继续失败。
- 串行运行 `-p 1 -count=1`; 全包很慢时先跑 smoke 包, 再按风险面追加包。
- `backend/.gocache` 里若存在历史 `00` 到 `ff` 分片目录、`review-tmp` 或固定 `run-tmp`, 视为旧实验缓存; 稳定入口只依赖 `review-cache`, `review-gopath` 和每轮新建的 `run-tmp-*`。
- 仓库内打包/编译缓存目录由根 `.gitignore` 排除, 不计入提交: `.gocache/`、`.gomodcache/`、`.gotmp/`、`backend/.gocache/`、`backend/.gomodcache/`、`backend/.gotmp/` 及 `backend/.go-test-cache/`、`backend/.gocache-test/`、`backend/.gotmp-*`。根目录或 `backend` 下若用 `GOCACHE`/`GOMODCACHE` 指向这些路径, 仅本机使用。

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

0.1.172-0.1.173 专项回归入口（后端命令在 `backend/` 目录运行）:

| 风险面 | 命令 |
| --- | --- |
| migration checksum / `_notx` / invalid index 恢复 | `go test -tags=unit -p 1 -count=1 ./internal/repository -run 'Migration|Migrations'`；需要真实 PostgreSQL schema 时再运行 `go test -tags=integration -p 1 -count=1 ./internal/repository -run 'Migration|Migrations'` |
| Channel Monitor V1/V2 互斥、聚合和权限 | `go test -tags=unit -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler ./internal/server/routes -run 'ChannelMonitor(V2|Mode|Probe)'` |
| Grok OAuth/session、Voice/Search/Video 路由 | `go test -tags=unit -p 1 -count=1 ./internal/pkg/xai ./internal/service ./internal/repository ./internal/handler ./internal/handler/admin ./internal/server/routes -run 'Grok|SessionStore|WebSearch'` |
| 上游响应模型采集与 mismatch 三态 | `go test -tags=unit -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler ./internal/handler/dto -run 'Upstream(ResponseModel|ModelMismatch)|UpstreamModel'` |
| Gemini 实际图片计数与 failover 重置 | `go test -tags=unit -p 1 -count=1 ./internal/service -run 'GeminiImage|Gemini.*Image'` |
| 前端 settings/auth/usage 往返 | `pnpm --dir frontend exec vitest run src/views/admin/__tests__/SettingsView.spec.ts src/views/auth/__tests__/RegisterView.spec.ts src/views/auth/__tests__/EmailVerifyView.spec.ts src/components/admin/usage/__tests__/UsageFilters.spec.ts src/components/admin/usage/__tests__/UsageTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts`（仓库根目录运行） |

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

配置来源优先级: 环境变量覆盖配置值；显式非空 `CONFIG_FILE` 时只读取该文件, 不再扫描 `DATA_DIR` 或默认目录, 文件缺失属于启动错误。未设置 `CONFIG_FILE` 时按 `DATA_DIR`、`/app/data`、当前目录、`./config`、`/etc/sub2api` 搜索 `config.yaml`；setup 前的 `GetServerAddress()` 使用同一文件选择逻辑。

主要配置组:

- `server`: host, port, mode, frontend_url, trusted_proxies, h2c, request body 上限; `read_header_timeout=10`、`max_header_bytes=65536`、`idle_timeout=120` 构成 HTTP ingress 基线, 不设置会截断 SSE/WS 的 `WriteTimeout`; `enable_server_timing` 默认 `false`, 也可用精确环境变量 `ENABLE_SERVER_TIMING=true` 开启管理端可观测响应头。`server.trusted_proxies` 只有在显式出现在配置或 `SERVER_TRUSTED_PROXIES` 环境变量时才启用 Gin 可信代理链；显式空数组表示禁用转发 IP 信任, 未配置时也使用直连 peer。
- `security.trust_forwarded_ip_for_api_key_acl` 是旧部署兼容开关, 代码默认 `true`: 开启时原始转发头接管客户端 IP 解析, 并按 `security.forwarded_client_ip_headers`、`CF-Connecting-IP`、`X-Real-IP`、`X-Forwarded-For` 顺序取值；关闭时才以 `server.trusted_proxies` 为权威。自定义头最多 16 个合法且不重复的 HTTP header 名, 可用 `SECURITY_FORWARDED_CLIENT_IP_HEADERS` 逗号分隔注入, 也可在管理设置页热更新。`deploy/config.example.yaml` 明确采用更安全的 `false` 与 loopback trusted proxies, 生产应按真实直连代理 CIDR 收紧。
- `run_mode`: `standard` 或 `simple`。
- `cors`: allowed origins 和 credentials。
- `security`: URL allowlist, response headers, CSP, proxy probe, proxy fallback, `trust_forwarded_ip_for_api_key_acl` 与 `forwarded_client_ip_headers`。客户端 IP 兼容开关默认 `true` 以兼容反代/Docker 升级；自定义 header 最多 16 个、按配置顺序优先, 仅在该开关开启时有效。关闭后必须正确配置 `server.trusted_proxies`。
- `gateway`: 上游超时, body size, request archive, request intercept, OpenAI WS, 调度, usage record, connection pool, Codex bridge。`max_body_size` 默认 256 MiB 供多模态/media, `text_max_body_size` 默认 32 MiB 并只用于 embeddings 与 alpha search 等纯文本入口。
- `gateway.grok.password_auth_enabled` 默认 `false`, 是 Grok 邮箱/密码转 SSO 再转 Build OAuth 的服务端能力开关；前端必须以 capabilities 响应为权威, 不能只根据本地状态显示入口。开启后 YesCaptcha 优先读取 `YESCAPTCHA_CLIENT_KEY`, 兼容旧名 `YESCAPTCHA_API_KEY`; 两者都缺失时密码授权返回 `GROK_OAUTH_CAPTCHA_KEY_REQUIRED`。密码和原始 SSO 只允许在单次授权链内短暂存在, 不写入配置、日志或账号凭据。
- Channel Monitor 的 `channel_monitor_enabled`、`channel_monitor_mode`(`v1|v2`)和 `channel_monitor_hide_throughput` 都是数据库 settings, 不是 YAML 配置。`channel_monitor_mode` 默认 `v1`; V1 主动探测与 V2 被动聚合互斥。`CHANNEL_MONITOR_V2_DISABLE_AGGREGATOR=1` 只跳过本节点 aggregator 的 `Start()`, 不改 mode、API guard 或数据库配置, 仅用于本地 seeded-data/demo 场景。
- 管理端运行时设置 `enable_client_dateline_normalization` 默认 `true`, 仅影响 Anthropic OAuth/SetupToken 转发, 用于清理客户端 dateline 隐写指纹; 关闭后请求体保持原样透传。
- `gateway.openai_ws.scheduler_score_weights.reset`: 默认 `0.0`, 用于给会话窗口最早重置的 OpenAI 账号加分; 关闭时不改变原调度行为。
- `gateway.openai_ws.scheduler_score_weights.quota_headroom`: 默认 `0.0`, 用于按 OpenAI/Codex 7d 剩余额度健康度给账号加分; 关闭时不改变原调度行为, 小流量灰度可从 `0.3` 起。
- `gateway.openai_scheduler`: OpenAI sticky session 逃逸配置; 默认开启, 可按 TTFT/error rate 跳过劣化 sticky 账号。
- `gateway.openai_proxy_stream_circuit`: OpenAI Responses SSE 代理断流的进程内 proxy-ID 熔断；`disabled` 默认 `false`, 可由 `GATEWAY_OPENAI_PROXY_STREAM_CIRCUIT_DISABLED` 整体关闭。`failure_threshold` 默认 2、`window_seconds` 默认 60、`ttl_seconds` 默认 600, 配置值只能非负, 0 回落默认。该状态不跨实例、不持久化, 重启清空；成功流清除观察, context cancel/deadline 不计失败。
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
- `redis`: Redis 连接池、ACL `username`、password 和 TLS；使用默认 Redis 用户时 `username` 留空。
- `api_key_auth_cache`: L1/L2 TTL、singleflight、`lookup_concurrency=64` 和进程内 invalid-auth abuse limiter；默认每可信客户端 IP（IPv6 按 `/64`）60 秒 120 次无效凭据后阻断 60 秒, capacity 16384。它不是 CDN/WAF 的替代品。
- `ops`: 运维监控开关。
- Panel API 限流不是 YAML 组, 由数据库 setting `panel_rate_limit_settings` 热管理；默认 `enabled=true,user_rpm=240,heavy_rpm=60,public_ip_rpm=300,exempt_admin=true`, 当前节点保存后立即刷新, 多节点最多等待 60 秒缓存 TTL。
- 动作验证码不是 YAML 独立组, 由数据库 settings 管理。腾讯验证码保存 AppID、AppSecretKey、Cloud SecretID/SecretKey；阿里云验证码保存 AccessKeyID/Secret、SceneID、Prefix、Region。默认关闭, public settings 只返回前端初始化所需的非 secret 字段。CSP 默认策略已放行 Cloudflare Turnstile、腾讯验证码和阿里云验证码 SDK/样式域名。
- OpenAI Codex 客户端版本自动同步默认开启, setting key 为 `openai_codex_version_auto_sync_enabled`；同步结果写入版本设置供 `codex_cli_only` 策略缓存读取。若部署不能访问 GitHub release API, 服务会保留旧值并告警。
- `jwt`, `totp`: 登录和 2FA 安全配置。
- OAuth: LinuxDo, WeChat, OIDC, DingTalk, GitHub, Google。
- `pricing`: 模型价格远程源, hash 校验和 fallback 文件。
- `billing`: 计费熔断。
- `gemini`: OAuth 和本地 quota 模拟。
- `image_storage`: 异步图片任务总开关与 S3-compatible 结果存储; `enabled=true` 仍要求 bucket/access key/secret 完整。`endpoint`, `region`, `prefix`, `public_base_url`, `presign_expiry_hours`, `max_download_bytes` 控制 R2/S3 兼容上传和 URL 结果下载上限。管理端备份页通过 `/api/v1/admin/backups/image-storage` 读取、测试和保存运行时设置, 可复用备份 S3 凭据；保存目标受 step-up 2FA 保护, 独立 secret 加密入库且 API 不回显。数据库配置优先且保存后失效 resolver/uploader 缓存, 下一次异步图片请求立即采用新配置, 无需重启；从未保存过后台配置时回落 YAML/环境变量。`IMAGE_STORAGE_*`、`SERVER_TRUSTED_PROXIES` 和 `SECURITY_FORWARDED_CLIENT_IP_HEADERS` 已有 env reachability guard。当前任务 worker 只在进程内运行, 服务重启不会恢复 Redis 中的 `processing` 任务, 可能保留到默认 24h TTL。生产环境若上游 URL 不可信, 应保持 `image_storage` 关闭直至 SSRF/任务恢复风险被上游修复。
- Ollama Cloud usage 不是 YAML 配置组, 由数据库 setting `OLLAMA_CLOUD_USAGE_SETTINGS` 热管理；默认 `enabled=false`、`interval_minutes=60`、`debounce_minutes=1`, 保存时分别钳制到 15-1440 与 1-60。后台 worker 由模型请求更新时间驱动 due 判定, 使用 PostgreSQL/Redis leader lock, 单周期最多刷新 20 个 eligible 账号、并发 4, 通过账号代理访问固定 `https://ollama.com/settings`；session 持久化要求固定 `TOTP_ENCRYPTION_KEY`。

Prompt Audit 是数据库运行时设置, 不在 YAML 中新增独立配置组:

- `settings.prompt_audit_config` 保存配置版本、启用/阻断开关、worker/queue、group 范围、scanner 和 OpenAI-compatible Guard endpoints；节点 token 以密文保存。`risk_control_enabled` 是上层总开关。
- 默认 `enabled=false`, 有效模式为 off / async audit / blocking。配置加载失败只在最近一次可解码的存储意图明确要求 blocking 且 `risk_control_enabled=true` 时 fail-closed；默认关闭或 async intent 不得因 untrusted config 被提升为 blocking。async 原文扫描载荷写 Redis key `sub2api:prompt_audit:payload:<job_id>`, TTL 最长 30 分钟, 完成或非重试失败后主动删除。
- 默认 worker 数 4、queue capacity 32768、节点 timeout 3000 ms、input limit 4000 rune。更新配置时必须带 `expected_config_version`, 多实例通过 PostgreSQL advisory lock 和 Redis version 通知刷新快照。
- 管理端读取 config 必须来自成功加载的 snapshot；持久配置激活失败且没有可信 snapshot 时返回 `prompt_audit_config_unavailable`, 不返回看似可用的默认关闭配置。setting 真正缺失时仍会成功建立 version 1 默认 snapshot；reload 失败保留最后一次可信 snapshot。

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
- 上述生产与开发 Compose 的 `sub2api` 服务都必须设置 `security_opt: no-new-privileges:true`; `deploy/tests/docker-compose-security-test.sh` 由 backend CI shell job 校验, 新增 Compose 变体时要同步覆盖。
- `deploy/Dockerfile` 的运行阶段必须把 builder 中的 `backend/resources` 复制到 `/app/resources` 并归属 `sub2api`, 保证容器运行时具有内置模型价格和上下文窗口 fallback 数据；release Dockerfile / GoReleaser 配置也必须保留等价资源打包, `deploy/tests/docker-runtime-resources-test.sh` 提供部署检查。
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
- `provideCleanup` 本身必须幂等：应用层 Stop 并行完成后才顺序关闭 Redis、Ent，重复调用不得再次 Stop/Close。quota flusher 的 Stop 只执行一次最终 flush，`flusher_enabled=false` 只禁止调度、不取消 shutdown flush；调度注册/取消由同一 lifecycle lock 串行化，Stop 必须等待在途 tick。TimingWheel 的 Cancel/Stop 必须使在途 recurring callback 不能再次自我挂载。Prompt Metrics 禁用时不创建 publisher worker；Token Analysis、并发 preset 和 quota flusher 的重复 Start 不得创建第二个后台任务。Token Analysis 的 Stop 是不可逆生命周期边界：之后自动 Start 为空操作，手动 async 索引返回 409，所有 `WaitGroup.Add` 必须在 Stop/Wait 前完成登记。合同入口见 `docs/features/upstream-fork-governance-validation-report-cn.md` 的 V03-V04。

## 上游历史分支合并注意事项

- `upstream/revert-114-feature/atomic-scheduling` 是 Wei-Shaw/sub2api 在 2026-01-01 创建的旧分支, 单提交 `30326cf2671a` 用于撤销早期 `#114` 负载感知账号调度优化。
- 当前分支历史已包含后续主线的同内容 revert `c5c12d4c8`, 也包含后续 reapply `7568dc850`; 当前 `GatewaySchedulingConfig`, `SelectAccountWithLoadAwareness`, `ConcurrencyService.GetAccountsLoadBatch`, scheduler snapshot/outbox 与 wait plan 代码是后续演进后的稳定实现。
- 以后若再次合并该元旦分支或等价历史分支, 冲突处理不应整块接受该旧 revert 的删除侧, 否则会回退当前网关调度、OpenAI/Gemini/Grok 路由选择和并发缓存能力。应先核对 `git log --grep "Reapply.*负载感知"` 与当前调用点, 再决定是否只补齐 merge 拓扑。

## 知识图谱工具

- `tools\check-understand-status.cmd`
- `tools\start-understand-dashboard.cmd`
- `tools\refresh-understand-wiki.cmd`
- 代码图谱重建: `/understand`（范围见 `.understand-anything/.understandignore`）
- Wiki 图谱重建: `/understand-knowledge llm-wiki` 或 refresh `.cmd`
- 入口说明: 使用 `.cmd`（内置 `-ExecutionPolicy Bypass`），避免 RemoteSigned 下 `powershell -File` 被拒

## 相关页面

- [[README]]
- [[backend]]
- [[frontend]]
- [[ops]]
- [[data-and-domain]]
- [[security-and-reliability]]
- [[ai-workflow]]
