# Codex 全仓技术债审计

| 字段 | 值 |
| --- | --- |
| 审计日期 | 2026-08-12 |
| 基线 | `main@56e054b16314cd83e023853e75d2b9e9759b48ad` |
| 审计者 | Codex |
| 文档定位 | 独立于 `technical-debt-board-cn.md` 的增量审计；不覆盖 Grok 看板 |
| 编号 | `CX-TD-*`，避免与现有 `TD-*` 冲突 |
| 状态 | `open` / `doing` / `blocked` / `mitigated` / `done` / `wontfix` |
| 外部评审 | 2026-08-12 Grok 源码抽检评审；见本文「十一、外部评审（Grok）」 |
| 审计快照 | 文内证据相对审计日仓库状态；基线 SHA 不随 main 滚动失效，执行前须再核实路径与 workflow |

> 结论先行：现有 Grok 看板对业务代码复杂度、已知上游问题和测试基线覆盖较好，但漏掉了发布供应链、版本库敏感产物、最低支持数据库版本验证、并发测试门禁以及知识入口漂移。优先处理已经进入 Git 历史的 Redis 运行快照，并修复可能组合不同提交前后端产物的手动发布链路；同时**不降低** Grok 看板已标 P0 的业务面风险（如 TD-001 SSRF、TD-027 Live 零计费）。

---

## 一、审计方法与边界

### 1.1 证据来源

| 层次 | 本轮使用方式 |
| --- | --- |
| 项目规则 | 先读 `AGENTS.md` 与 `llm-wiki/wiki/README.md` |
| 稳定知识 | 读取 backend / frontend / ops / data / security wiki |
| 既有债务 | 逐项对照 `technical-debt-board-cn.md` 与 `golangci-lint-debt-cleanup-plan-cn.md` |
| 图谱导航 | 检查本机代码图谱；图谱含 7,862 nodes / 27,001 edges，代码路径基线无漂移 |
| 源码取证 | 定点读取 GitHub Actions、Makefile、集成测试 harness、安全设置、加密密钥路径、被跟踪产物 |
| Git 取证 | 核对当前分支、工作树、文件是否被跟踪、引入提交和现存 blob |

### 1.2 本文纳入标准

只有满足以下至少一项的条目才进入本文：

1. 当前源码或 Git 对象可直接证明风险存在；
2. 当前 CI / release 合同存在可复现的覆盖缺口；
3. 权威文档与真实配置明确冲突，足以误导开发或发布；
4. 已有清理结论长期没有执行，且仍持续制造风险。

### 1.3 本轮未执行

| 未执行项 | 原因 | 对结论的影响 |
| --- | --- | --- |
| 全量 Go unit / integration | 本轮只改文档，且全量验证成本高 | 不把历史测试结果当本轮 PASS；CI 结构结论来自当前 workflow/harness |
| 全量 Vitest / build | 本轮不改前端 | 不重新计数既有红项 |
| `golangci-lint` / `go mod tidy -diff` | 已由现有看板覆盖，本轮目标是找遗漏 | 不更新旧债数量 |
| 生产数据库、Redis、部署环境检查 | 本轮仅审计仓库 | CX-TD-001 的凭证有效性未知，必须由运维侧鉴别 |

因此本文是“高置信增量债务清单”，不是形式化安全审计，也不声称穷尽所有缺陷。

---

## 二、与 Grok 看板的关系

### 2.1 复核后同意的最高优先级共识

下表不重复展开，偿还时仍以原 `TD-*` 为主：

| 原编号 | Codex 复核结论 | 证据摘要 |
| --- | --- | --- |
| TD-001 | 同意 P0 | `image_storage.go` 下载上游 URL，缺少私网/DNS rebinding/redirect 防护 |
| TD-027 | 同意 P0 | OpenAI Live usage 成本仍为 0，需明确计费或正式免费合同 |
| TD-004 / 005 / 006 / 039 | 同意质量门禁优先修 | 全量 lint、后端合同、前端全量测试与 CI 门禁定义不一致 |
| TD-007 / 008 / 025 / 026 | 同意数据与性能风险 | migration 存量语义和两条高成本 SQL 都有明确源码/文档依据 |
| TD-009 / 013 / 028 / 041 / 042 | 同意长期结构债 | 本地 fork 热点、网关/配置/service package/巨型 SFC 的维护成本客观存在 |

### 2.2 本文新增的视角

| 方向 | 现有看板覆盖 | 本文补充 |
| --- | --- | --- |
| 发布供应链 | 基本未覆盖 | 指定 tag 发布时前后端可能来自不同提交；release 不依赖 CI |
| 仓库敏感产物 | 未覆盖 | 已跟踪 Redis RDB 含鉴权/调度运行态和疑似 credential |
| 债务治理闭环 | 未覆盖 | 已写清理方案的死文件/调试 diff 数月后仍在 main |
| 支持矩阵 | 未覆盖 | 宣称 PostgreSQL 15+，CI 只跑 PostgreSQL 18.1 |
| 并发验证 | 局部提及 | 113 个生产 `go func(`、132 个并发原语文件，但 CI 无 `-race` |
| 文档一致性 | 未覆盖 | `DEV_GUIDE.md` 的 Go/lint/security 信息已过期 |
| 默认安全姿态 | 部分 wiki 记录 | 原始 forwarded header 信任仍作为新设置默认并进入安全敏感路径 |
| 密钥生命周期 | 仅有操作警告 | 缺少启动期“持久密文与固定密钥一致性”检查/轮换机制 |

---

## 三、新增技术债总览

| ID | P | 状态 | 标题 | 类别 | 建议动作 |
| --- | --- | --- | --- | --- | --- |
| CX-TD-001 | P0 | open | Git 历史中存在 Redis 运行快照与疑似凭证材料 | 安全/仓库 | 立即鉴别、轮换、删除并评估历史净化 |
| CX-TD-002 | P0 | open | 手动指定 tag 发布可能组合不同提交的前后端产物 | 发布/供应链 | 所有 job 固定解析后的同一 SHA |
| CX-TD-003 | P1 | open | Release workflow 不依赖验证门禁且跳过 GoReleaser validate | 发布/CI | 增加 release preflight 和不可绕过依赖 |
| CX-TD-004 | P1 | open | PostgreSQL 15+ 合同没有最低版本 CI | 数据/兼容 | 增加 PostgreSQL 15 matrix |
| CX-TD-005 | P1 | open | 并发密集后端没有持续 race detector 门禁 | 可靠/CI | 增加定向或夜间 `-race` |
| CX-TD-006 | P1 | open | 原始 forwarded header 信任仍是数据库默认 | 安全/兼容 | 迁移到显式可信代理并增加健康告警 |
| CX-TD-007 | P1 | open | 持久加密密钥缺少启动期一致性检查和轮换合同 | 安全/可靠 | 检测已有密文 + 临时/错误 key 并 fail closed |
| CX-TD-008 | P2 | open | 已批准的仓库清理计划长期未闭环 | 治理/仓库 | 执行清理并加防回归检查 |
| CX-TD-009 | P2 | open | `DEV_GUIDE.md` 与真实 CI/版本漂移 | 文档/入门 | 从可执行配置生成或加一致性测试 |
| CX-TD-010 | P2 | open | 安全扫描不可复现且门禁归属分散 | 供应链/安全 | 固定 govulncheck 版本，恢复含 gosec 的 lint 门禁 |
| CX-TD-011 | P2 | open | release 版本文件由 workflow 临时改写，源码 tag 内容不自洽 | 发布/可追溯 | tag 前校验 VERSION，避免发布后 bot 回写补偿 |
| CX-TD-012 | P3 | open | `integration` build tag 同时承载真实基础设施测试与纯逻辑测试 | 测试结构 | 重新定义 test taxonomy，缩短反馈并明确覆盖 |

---

## 四、P0：立即处理

### CX-TD-001 · Git 历史中存在 Redis 运行快照与疑似凭证材料

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 当前文件 | 根目录 `dump.rdb`，12,409 bytes，Git blob `3ee93cc932b0d1607cc8f4e2cc69df957c7b4279` |
| 引入提交 | `bd8d37582624c106b623073461e3ebfdbf226ca4`（2026-05-11） |
| 读取得到的类型 | refresh-token 哈希族、user/token-version、sticky session、scheduler account metadata、疑似 `rk-...` credential 字段 |
| 为什么是 P0 | 文件已经进入 Git 历史；即使现在删除，任何历史 clone 仍可读取。仅凭仓库无法证明这些数据是测试数据或已经失效 |
| 不应做 | 不要在 issue/PR/文档中粘贴 RDB 内具体值；不要只加 `.gitignore` 后宣称完成 |

建议处置顺序：

1. 由运维/安全负责人离线鉴别 RDB 来源、环境和时间范围；
2. 若无法证明全部为无效测试数据，轮换相关 API credential、refresh-token family/JWT 会话和受影响账号凭据；
3. 从当前树删除 `dump.rdb`，并添加根级 `dump.rdb` / `*.rdb` ignore 与 secret-scan 规则；
4. 根据远端分发范围决定是否使用 `git filter-repo` 净化历史；历史改写必须单独审批和通知所有 clone；
5. 在 CI 加入 tracked runtime artifact 检查，防止 `.rdb`、数据库 dump、日志和未脱敏 runtime report 再进入仓库。

关闭标准：

- 完成数据来源判断和凭证处置记录；
- 当前分支不再跟踪 RDB；
- 对“是否改写历史”有明确决策；
- CI 可阻止同类文件再次提交。

### CX-TD-002 · 手动指定 tag 发布可能组合不同提交的前后端产物

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 入口 | `.github/workflows/release.yml` |
| 触发方式 | `workflow_dispatch.inputs.tag` |
| 直接证据 | `release` job checkout 使用 `${{ github.event.inputs.tag || github.ref }}`；但 `update-version` 与 `build-frontend` 的 checkout 没有 `ref`，默认读取工作流所在默认分支 |
| 失败模式 | 手动发布旧 tag 时，前端 dist 来自当前默认分支，后端源码来自旧 tag，VERSION 又由输入临时生成；最终二进制不是任何单一 commit 的可重建产物 |
| 影响 | 前后端 API contract 漂移、资产不可复现、回滚目标被污染、SBOM/审计无法映射到唯一 SHA |

建议：增加 `resolve-ref` job，将 tag 解析并校验为唯一 commit SHA；`update-version`、`build-frontend`、`release` 全部 checkout 同一 SHA，artifact name/attestation 同时带 SHA。拒绝不存在、非 `v*`、不指向 commit 或与 VERSION 合同冲突的输入。

关闭标准：对任意旧 tag 手动发布时，前端、后端、VERSION、GoReleaser metadata 都指向同一 commit；增加 workflow 静态测试或最小 dry-run 证明。

---

## 五、P1：发布、数据与可靠性

### CX-TD-003 · Release workflow 不依赖验证门禁

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 证据 | release job 仅 `needs: [update-version, build-frontend]`，不执行 unit / integration / lint / security；GoReleaser 使用 `--skip=validate` |
| 影响 | tag 可直接构建和推送未经过同 SHA 验证的发布物；普通 CI 的失败不会阻止发布 workflow |
| 与 TD-004/006/039 的关系 | 即使修复普通 CI 基线，release 仍没有把该结果作为发布前置条件 |

建议将发布拆成：`resolve-ref` → `verify` → `build frontend` / `build artifacts` → `publish`。`verify` 至少执行仓库定义的 required checks，并在 publish 前校验 tag commit 的 GitHub check-suite 结论。若必须保留 `--skip=validate`，需记录具体上游限制并用等价自有校验替代。

### CX-TD-004 · PostgreSQL 15+ 合同没有最低版本 CI

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 对外合同 | `README.md` / `README_CN.md` 写 PostgreSQL 15+；部分 deploy 文档甚至仍写 14+ |
| 当前 CI | repository integration harness 固定 `postgres:18.1-alpine3.23` |
| 源码自证 | harness 已提供 `SUB2API_TEST_POSTGRES_IMAGE`，并明确说明 PostgreSQL 14-16 与 17+ 的 `jsonpath .datetime()` 行为不同 |
| 风险 | migration、JSONPath、锁和 SQL 计划在 18 通过，不代表最低支持的 15 通过 |

建议在 CI 加最小矩阵：PostgreSQL 15（合同下界）+ 18（当前默认）。可让完整 suite 只跑 18，在 15 跑 migration/schema 和版本敏感 repository 包，控制耗时。

关闭标准：明确唯一最低支持版本；CI 在该版本执行 migration 和版本敏感 SQL；README、deploy 与 wiki 统一。

### CX-TD-005 · 并发密集后端没有持续 race detector 门禁

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 量化 | 生产代码约 113 处 `go func(`；132 个文件使用 `sync.Mutex` / `RWMutex` / `atomic` |
| 当前门禁 | PR 用 `.github/workflows/backend-ci.yml` 与根/`backend` Makefile **无** `-race` |
| 局部例外 | `deploy/Makefile` 的 integration 目标带 `-race`（部署/本地脚本路径，**不是** PR CI 门禁） |
| 已有局部实践 | RequestArchive 等曾定向跑 `-race`，说明关键模块可执行，但没有持续化 |
| 风险 | 调度、usage billing、缓存失效、后台 worker、WS/SSE 生命周期的竞态只能靠偶发测试暴露 |

建议先建立可控的夜间/定向门禁，不必一开始全仓：`internal/server/middleware`、核心 gateway/billing/cache/runner 包跑 `go test -race -count=1`；逐步处理 CGO、时长和 flaky 后再扩大。

### CX-TD-006 · 原始 forwarded header 信任仍是数据库默认

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 默认来源 | `config.go` viper 默认 `security.trust_forwarded_ip_for_api_key_acl=true`；`setting_parse.go` factory 默认字符串 `"true"`（缺省即信任原始 forwarded header） |
| 安全影响面 | `api_key_auth.go` / `api_key_auth_google.go` 的 IP ACL；`session_binding.go` 的会话 IP/UA 绑定；日志与限流也共享客户端 IP 口径 |
| 已有缓解 | `deploy/config.example.yaml` 已显式设 `false`；文档要求源站只接受可信边缘并覆盖请求头 |
| 剩余债务 | 应用无法证明源站防火墙/边缘覆盖条件成立，兼容默认可能把可伪造 header 作为安全身份 |
| 表述修正（评审） | 未在当前 `setting_parse` 片段中找到「把已持久化的 false 强制改回 true」的迁移语句；准确说法是**配置/DB 缺省为 true**，与 trusted-proxy 模型并存。偿还时勿按「找 false→true migration」排查 |

这不是建议直接翻转默认。正确偿还方式是分阶段：增加启动健康告警和管理端显著风险提示，遥测仍启用的实例；为新安装固定安全默认；给旧安装提供验证过的 trusted-proxy 迁移向导，最后再移除兼容模式。

### CX-TD-007 · 持久加密密钥缺少启动期一致性检查和轮换合同

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 当前行为 | `TOTP_ENCRYPTION_KEY` 缺失时每次启动自动生成临时 key；后台禁止新启用 TOTP，支付/S3/Prompt Audit/Ollama 等新 secret 写入已有部分 guard |
| 未覆盖路径 | 服务启动不检查数据库是否已经存在 TOTP secret、channel monitor API key、旧 payment/S3 ciphertext 等持久密文，也没有 key ID/version 或轮换流程 |
| 失败表现 | 重启/多实例使用不同 key 后，已有密文才在运行时解密失败；例如 TOTP 登录失败、channel monitor key 被清空为不可用、旧配置静默退化 |

建议给密文加 envelope metadata（key ID/version），引入启动 preflight：若检测到持久密文但 key 未固定或无法解密，release mode fail closed 或至少 readiness=false；同时设计双 key 读、单 key 写的滚动轮换流程。

---

## 六、P2：治理、文档与供应链

### CX-TD-008 · 已批准的仓库清理计划长期未闭环

`cleanup-plan-2026-07-08.md` 已明确认定下列文件为误提交或死代码，但截至当前 `main` 仍全部被跟踪：

| 类型 | 路径 | 现状 |
| --- | --- | --- |
| 合并调试 diff | `_conflict_oai.diff`、`_local_oai.diff`、`_merged_oai.diff`、`_merged_test.diff`、`_upstream_oai.diff` | 约 182 KiB，仍在根目录 |
| Go 合并脚手架 | `backend/_local_compat.go`、`backend/_upstream_fallback.go` | Go 工具链因下划线前缀永不编译，正式实现已迁移 |
| 临时对比 | `ours_numstat.txt`、`theirs_numstat.txt` | 后续合并产物继续进入 main |

债务不只是这些文件，而是“计划存在但没有 owner、截止时间、PR 和验证”的闭环缺失。建议把清理作为独立 chore PR，并增加 tracked artifact allow/deny check；不要在下一次上游合并中顺手删除。

### CX-TD-009 · `DEV_GUIDE.md` 与真实 CI/版本漂移

| 文档声称 | 当前事实 |
| --- | --- |
| Go 1.25.7 | `backend/go.mod` 与 CI 为 1.26.5 |
| golangci-lint v2.7 | CI 为 v2.9 |
| security scan 包含 gosec | workflow 只有 govulncheck + pnpm audit |
| CI 概述未提前端 lint/typecheck/critical Vitest 边界 | 当前 `make test-frontend` 明确是 critical 子集 |

建议指定 `llm-wiki/wiki/ops.md` 为维护事实，`DEV_GUIDE.md` 只做稳定入口并引用它；或者写脚本从 `go.mod` / workflows / Makefile 生成版本表，并在 CI 做一致性断言。

### CX-TD-010 · 安全扫描不可复现且门禁归属分散

| 字段 | 内容 |
| --- | --- |
| 状态 | open |
| 证据 | security workflow 每次 `go install .../govulncheck@latest`；`gosec` 实际由 `backend/.golangci.yml` 启用，不在独立 security workflow 中 |
| 风险一 | 同一 commit 的扫描结果随时间变化，工具供应链不可复现；上游最新工具故障可直接打红 main |
| 风险二 | 源码安全规则依赖当前基线红的 golangci-lint job；当团队把该 job 当“已知红”忽略时，gosec 新问题也会淹没在同一失败信号中 |

建议固定 govulncheck 版本并由 Renovate/Dependabot 升级；先按 TD-004 恢复 lint 门禁，或把 gosec 拆成可独立判定新问题的 job。secret scanning 需覆盖二进制/数据库 dump 文件名和历史，而不仅是文本正则。

### CX-TD-011 · Release VERSION 与 tag 内容不自洽

当前 workflow 在发布时临时覆盖 `backend/cmd/server/VERSION`，成功后再由 bot 向默认分支提交 `[skip ci]`。这让 tag 对应源码中的 VERSION 可能不是发布版本，且默认分支的补偿 commit 没有正常 CI。

建议改为“先在源码提交中更新 VERSION并通过 CI，再创建 tag”；release 只校验 `tag == v$(cat VERSION)`，不改源码。若业务必须由 tag 生成版本，产物应把 tag/SHA 写入 ldflags，而不是改工作树文件后再反向同步 main。

---

## 七、P3：测试结构

### CX-TD-012 · `integration` 标签语义混杂

当前约 75 个 integration-tag 测试中，repository harness 会拉起 PostgreSQL 18.1 + Redis 8.4，属于真实基础设施集成；但 `thinking_protocol_filter_integration_test.go`、部分 TLS/route 测试只验证内存逻辑或协议组合。统一标签导致：

- 改纯逻辑也可能必须等待容器级 suite；
- “integration PASS” 难以看出哪些真实依赖被覆盖；
- Windows 本地容易因为 Docker/文件锁跳过或失败，反馈边界不清楚。

建议逐步划分：普通 unit、component（多模块内存组合）、integration-postgres、integration-redis、e2e。Makefile 和 CI 输出各类 test count / skipped count，避免“退出 0”被误解成所有外部合同都执行。

---

## 八、建议偿还顺序

> 评审校准后的执行序（与「只修 CX、不碰业务 P0」相反）：CX 与 Grok 看板 P0 **并行**，按风险面插队。

| 阶段 | 条目 | 目标 |
| --- | --- | --- |
| 立即响应 | **CX-TD-001** | 鉴别泄露面、完成轮换决策、阻止继续分发（按安全事件，不只是债项） |
| 发布止血 | **CX-TD-002、003、011** | 每个发布物绑定唯一 SHA，并且只能从已验证提交发布 |
| 业务面 P0（Grok 看板，不降级） | **TD-001、TD-027**（及看板其它 P0） | 异步图 SSRF、Live 零计费等；与 CX 并行，不因「先拆供应链」而搁置 |
| 低成本卫生 | **CX-TD-008** | 执行已有 cleanup-plan；独立 chore，不塞进 upstream merge |
| 安全/扫描合同 | CX-TD-006、007、010 | 分阶段去掉隐式信任；密钥 preflight；固定 govulncheck 版本 |
| 数据/并发 | CX-TD-004、005 | 最低数据库版本与高并发路径进入持续门禁 |
| 工程治理 | CX-TD-009、012 | 统一权威入口和测试语义 |

### 推荐拆分为独立 PR

| PR | 内容 | 是否改历史 |
| --- | --- | --- |
| Security incident | CX-TD-001 鉴别/轮换/当前树删除/防回归 | 历史净化需单独批准 |
| Release provenance | CX-TD-002 + CX-TD-003 + CX-TD-011 | 否 |
| Supported DB matrix | CX-TD-004 + 文档统一 | 否 |
| Concurrency gate | CX-TD-005 定向 race | 否 |
| Forwarded IP migration | CX-TD-006 分阶段告警与迁移 | 可能涉及运行时设置迁移 |
| Secret key lifecycle | CX-TD-007 preflight + rotation design | 可能涉及 schema/config |
| Repository hygiene | CX-TD-008 + denylist | 否 |
| Docs/test taxonomy | CX-TD-009 + CX-TD-012 | 否 |

---

## 九、关闭记录模板

```markdown
### 关闭记录 · CX-TD-00X

| 字段 | 内容 |
| --- | --- |
| 日期 / 分支 / commit | |
| 修复摘要 | |
| 验证命令与结果 | |
| 生产/安全侧证据 | |
| wiki / 本文状态 | |
| 残留风险 | |
```

关闭条目时只改状态并追加关闭记录，不删除原始证据。若最终确认是有意权衡，标记 `wontfix` 或 `mitigated`，同时写清 owner、复审日期和可接受边界。

---

## 十、相关文档

| 文档 | 用途 |
| --- | --- |
| `docs/features/technical-debt-board-cn.md` | Grok 深度扫描看板，仍是 `TD-*` 主清单 |
| `docs/features/upstream-fork-governance-design-cn.md` | TD-009 方案 A 五类强类型接缝、迁移与评审闭环 |
| `docs/features/upstream-fork-governance-effectiveness-test-standard-cn.md` | TD-009 v1.1 效果验收标准（含 Grok 评审） |
| `docs/features/golangci-lint-debt-cleanup-plan-cn.md` | lint 存量明细 |
| `cleanup-plan-2026-07-08.md` | 已识别但尚未执行的仓库清理计划 |
| `.github/workflows/backend-ci.yml` | 当前 PR/push 验证合同 |
| `.github/workflows/release.yml` | 当前发布链路 |
| `.github/workflows/security-scan.yml` | 当前依赖安全扫描 |
| `backend/internal/repository/integration_harness_test.go` | PostgreSQL/Redis integration 真实基线 |
| `llm-wiki/wiki/ops.md` | 当前构建、CI、发布和配置事实 |
| `llm-wiki/wiki/security-and-reliability.md` | 客户端 IP、密钥和网关安全边界 |

---

## 十一、外部评审（Grok，2026-08-12）

| 字段 | 内容 |
| --- | --- |
| 评审者 | Grok |
| 日期 | 2026-08-12 |
| 方法 | 通读本文与 fork 效果标准；对 P0/P1 主张做仓库/源码抽检（非形式化渗透） |
| 结论 | **采纳为正式增量债清单**；先做 CX-TD-001 / 002 族与仓库卫生，业务面 Grok P0 不降级 |

### 11.1 总评

| 维度 | 结论 |
| --- | --- |
| 增量价值 | **高**。补上 Grok 看板几乎未覆盖的发布供应链、Git 敏感产物、支持矩阵、race 门禁、密钥生命周期、仓库卫生闭环 |
| 证据质量 | **整体扎实**。关键 P0/P1 多数可在仓库复现 |
| 与 `TD-*` 关系 | **处理得当**：`CX-TD-*` 独立编号；同意项只交叉引用 |
| 可执行性 | 审计条目可直接拆 PR；偿还顺序已按评审校准（见第八节） |
| 风险校准 | 总体正确；「最紧急不是拆 god-file」成立，但须并列业务面 P0，避免只修 workflow |

### 11.2 抽检核实摘要

| ID | 评审 | 抽检要点 |
| --- | --- | --- |
| CX-TD-001 | **同意 P0；升安全事件** | 根目录 `dump.rdb` 仍 tracked（约 12,409 bytes，引入 `bd8d37582`）；内容类型含 refresh_token / sticky / `rk-…` 形态字段。**勿在 issue/PR 粘贴具体值**。仅 ignore 不算关闭 |
| CX-TD-002 | **同意 P0** | `release.yml`：`update-version` / `build-frontend` checkout 无 `ref`；`release` 才用 `inputs.tag`。手动旧 tag 可导致前后端非同一 SHA |
| CX-TD-003 | **同意 P1** | `needs` 仅 version+frontend；无 unit/lint/security；GoReleaser `--skip=validate` |
| CX-TD-004 | **同意 P1** | harness 固定 `postgres:18.1-alpine3.23`；已有 `SUB2API_TEST_POSTGRES_IMAGE`；对外合同 15+ 与 CI 不一致 |
| CX-TD-005 | **同意 P1（补 nuance）** | 生产 `go func(`≈113、mutex/atomic 文件≈132 与文一致；PR CI 无 `-race`；`deploy/Makefile` 有 race 属脚本例外 |
| CX-TD-006 | **同意现象；收紧表述** | 默认信任成立；已改正文「false→true migration」表述（见条目内评审修正） |
| CX-TD-007 | **同意方向；排期靠后** | 空 `TOTP_ENCRYPTION_KEY` 启动生成临时 key 已核实；属设计+可能 schema，勿与 001 抢同一 sprint |
| CX-TD-008 | **同意 P2；应本周可做** | cleanup-plan 所列 `_*.diff`、`backend/_local_compat.go`、`_upstream_fallback.go`、numstat 等仍 tracked |
| CX-TD-009 | **同意 P2** | `DEV_GUIDE.md` 仍写 Go 1.25.7、golangci v2.7、security 含 gosec；与 CI 1.26.5 / v2.9 / 仅 govulncheck 冲突 |
| CX-TD-010 | **同意 P2** | `govulncheck@latest` 不可复现；gosec 挂在基线常红的 golangci 上易被淹没 |
| CX-TD-011 | **同意 P1** | 发布改 VERSION 再 bot `[skip ci]` 回写 default branch，tag 源码与版本可不自洽 |
| CX-TD-012 | **同意 P3** | `//go:build integration`≈75；真基础设施与内存逻辑混标签 |

### 11.3 采纳决议

**立即采纳并行动**

1. **CX-TD-001**：安全事件流程（鉴别 → 轮换 → 当前树删除 → ignore + CI denylist → 历史净化另批审批）
2. **CX-TD-002 + 003 + 011**：单一 Release provenance PR
3. **CX-TD-008**：独立 hygiene chore（执行 `cleanup-plan-2026-07-08.md`）
4. Grok 看板 **TD-001 / TD-027** 等业务 P0：保持并行，不降级

**纳入债板、1–2 周内排期**

- CX-TD-004、005、009、010

**设计后再做**

- CX-TD-006、007、012
- TD-009 方案 A **实现设计**已形成候选稿，需用户评审批准后再写实施计划；效果标准已升 v1.1 并保留硬门槛

**不要做的**

- 用 fork A/B 或拆 god-file 代替 001/002 止血
- 在 upstream 合并 PR 里顺手清 RDB/diff/改 release
- 未补契约测试就大拆 gateway/service 并宣称 TD-009 mitigated
- 知识图谱 json 刷新与审计文档绑同一业务 commit（若仅为 refresh 噪音）

### 11.4 文档工程建议（已部分落地）

- 主看板 `technical-debt-board-cn.md` 增加 Codex 审计交叉引用与合并优先级
- wiki 快速定位已链到本文；关闭 CX 项时同步主看板「外部审计」摘要状态
- 两份清单编号独立；关闭只改状态、保留证据

### 11.5 评审变更记录

| 日期 | 说明 |
| --- | --- |
| 2026-08-12 | 首轮 Grok 评审入库；修正 CX-TD-006 表述与 CX-TD-005 deploy Makefile nuance；偿还顺序并入业务 P0 与 CX-TD-008 |
| 2026-08-12 | TD-009 后续提升：效果标准升 v1.1，新增方案 A 正式设计候选与评审门禁；未进入业务代码实施 |
