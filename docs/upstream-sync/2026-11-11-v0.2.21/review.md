# 0.2.21 上游同步审核

日期：2026-11-11；分支：`feature/hy/10234_merge_sub2api_221`；状态：合并待审核，未 commit / push / 合回 main / 部署。

## 1 范围

| 项目 | 固定值 |
| --- | --- |
| 第一父、本地 main | `dc72e67a0449b025a83fed701d6ac2cf7818f0d6` |
| 指定上游 | `Wei-Shaw/sub2api main@6dd5a045b62e27e5b5b139adef91a171097f639b`，VERSION 0.2.21 |
| 共同祖先 | `6b4b37db43443b5e88e424fe3c69602cc3810dfb`（0.2.20） |
| 增量 | 82 commits / 200 上游路径；40 个双方修改路径、160 个仅上游路径、775 个仅本地路径 |
| 操作边界 | 从干净的本地 main 建分支，`git merge --no-commit --no-ff <固定 SHA>`；fetch 时 upstream/main 已前进至 `530c5e3a4`，仍只合入用户指定 SHA |
| 修补边界 | 只解决冲突与验证所需的合并格式；不修上游 bug，不扩展本地功能 |

## 2 冲突与处理

### 文本冲突

唯一冲突文件 `backend/internal/repository/migrations_runner.go` 有两段：常量定义、非事务迁移预处理 switch。保留本地 Token Analysis usage-link、prefix fingerprint、classification pending/due 索引恢复逻辑，并加入上游 `usageLogsAPIKeyCostIndexMigration` / `usageLogsAPIKeyCostIndex` 及 `dropInvalidIndexIfPresent` 分支。二者管理不同索引，没有可互相替代的业务。gofmt 补齐常量段前空行。

### 自动合并复核

| 范围 | 本地与上游的组合结论 |
| --- | --- |
| Ent 三个生成文件 | usage 30m/12h 字段与本地部门、权限等实体并存；Ent/Wire 连续生成两次无漂移，生成器验证比手工挑选一侧更可靠 |
| config 与示例 | 上游自适应 body budget + 本地 RequestArchive/RequestIntercept、Token Analysis、Prompt Metrics、默认 effort、大请求保护、组织日汇总配置并存 |
| DTO / 类型 | 上游 cache-write 分项 + 本地用户 `account{id,name}`、部门与管理员权限字段；原有敏感字段投影保留 |
| OpenAI Responses handler | 上游 UTF-8 拒绝、page context 规范化 + 本地鉴权、并发故障分类、归档与请求事实接线 |
| OpenAI messages / body / usage | 上游最终 effort 校验、30m/12h 解析与计费 + 本地 compatible cache 正值补缺、thinking 清理、图片价格缺失拒绝；未发现完整替代本地独有能力的上游实现 |
| passthrough / WS | 上游 metadata、冷却字段匹配、完整历史搜索激活、后续 turn 禁止首次重放和 TTFT + 本地 Token Analysis、有限 Retry-After、时区改写 |
| 认证与账号用量 | 上游 refresh token 原子消费仍返回本地 AdminPermissions；Antigravity force 查询仍保留本地只读用量不清账号错误边界 |
| 前端账号、分组、设置、管理用量 | 账号刷新旧行/额度保留、12h 价与用量、sticky 保存 + 本地账号操作员限制、分组组织归属、时区/归档设置、用户账户列与禁止用户点击跳转 |
| 国际化 / README / gitignore / 测试桩 | 本地键和 docs 白名单与上游新增项并存，refresh token stub 新接口随上游同步；没有恢复已归档文档目录 |

三方增删序列复核：40 路径中 37 个在双方视角均完整保留双方增删内容（迁移文件仅额外有 gofmt 空行）；另外 3 个是 Ent 生成文件，由生成一致性验证覆盖。完整路径与两侧差异保留于仓库外 `manifest.json`、`overlap-ours.diff`、`overlap-upstream.diff`、`delta-audit.json`。

## 3 保留的本地能力

| 本地功能 | 结果 |
| --- | --- |
| Token Analysis / RequestArchive / RequestIntercept / Prompt Metrics/Risk | 索引恢复、配置、路由、Wire、请求事实与前端入口保留 |
| 组织/部门/组织负责人/分组组织归属 | 权限、CAS、迁移、组织日汇总、成员管理和受限订阅路径保留 |
| 子管理员与账号操作员 | HTTP 方法/路由白名单、菜单与操作限制、精简选项和活动通知保留；无新增 HTTP 路由扩大权限 |
| 订阅自助日重置/批量重置、GPT 额度、并发预设 | 源码与接线保留，执行现有测试 |
| OpenAI 本地适配 | 默认 effort、compatible cache、时区改写、大请求保护、并发基础设施错误分类、空响应兜底与插件账号目录保留 |
| 生图与用户用量账户列 | 页面、状态/自定义尺寸、本地账户最小摘要及只读表格行为保留 |
| `docs/features/` | 26 个文件零修改、零删除；仅按本次实际变化维护 wiki、组件 README 和本轮唯一 review |

## 4 采用的上游行为

| 上游增量 | 行为与边界 |
| --- | --- |
| 缓存写入时长 | usage 新增 30m/12h；上游报告的分项必须完整且合计与 aggregate 一致，才使用时长价；NULL 12h 价沿通用价，显式 0 保留 |
| 数据迁移 | 新增 `249_add_usage_logs_api_key_cost_index_notx.sql`、`250_openai_cache_write_ttl.sql`；与本地同前缀迁移按完整文件名共存，既有 SQL 不改 |
| 认证/调度/审计 | refresh token 原子消费；账号 hydration 失败归还槽位；审计入队唤醒本进程 worker |
| 网关 | 非法 UTF-8 拒绝、Codex page context 与 metadata、sealed function metadata、effort 策略最终校验、WS 搜索激活/重放/TTFT、Gemini thinking 与 Antigravity tool media |
| 运行配置 | 未显式配置的 body memory budget 为检测内存上限的 1/4、最低 1 GiB；显式正数或 0 不被覆盖。内存检测考虑物理内存、cgroup、GOMEMLIMIT |
| 模型/价格 | Fable/Mythos 5.2、Sonnet 5.5 fallback 缓存价、价格目录 hash 不一致重试、Fast 免费输入价格保留倍率 |
| 前端/支付 | API Key 列表异步用量、账号刷新保留数值、sticky 保存；充值倍率 6 位小数、EasyPay 每实例退款成功码 |
| 图片格式说明 | `docs/OPENAI_IMAGES_CODEX.md` 说明实际格式可能不同于请求，沿上游保留成功内容与 usage，不擅自重试 |

## 5 验证

| 验证 | 实际结果 |
| --- | --- |
| 前端 frozen install / lint:check / typecheck | 全部通过，package.json/lockfile 零漂移 |
| 完整 Vitest | 438 files / 3622 tests 全部通过 |
| 前端 build | 通过，1220 modules；沿用已有 chunk size/Browserslist 提示 |
| Go default 全量：`go test -json -p 1 -count=1 ./...` | 退出 0；57 包、7860 顶层 / 16287 含子用例 PASS，0 顶层 FAIL；14 显式 skip |
| Go unit 全量：同上 + `-tags=unit`（CI 单测门禁） | 退出 1；62 包、13368 顶层 / 28042 含子用例 PASS，1 顶层 FAIL；41 显式 skip；失败仅为上游新增 TTFT 时序测试，见第 6 节，不写成全绿 |
| Go integration 全量：同上 + `-tags=integration` | 退出 0；57 包、7366 顶层 / 15457 含子用例 PASS，0 顶层 FAIL；18 显式 skip；repository 无 Docker 整包跳过，不计作数据库通过 |
| 隔离 PG 升级 | PostgreSQL 18.3 新库先跑 main 的 330 条迁移，再跑合并结果成为 332 条；集合差恰好为本次新增的 249/250，旧迁移无删除/修改 |
| 隔离 PG 回归 | 缓存时长往返、迁移、Token Analysis、部门/组织、订阅自助重置、MCP/邮箱黑名单：202 顶层 / 250 含子用例 PASS，0 FAIL；5 个性能/EXPLAIN 门控 skip |
| 受限管理密钥 PG 补跑 | repository/handler 专项 51 顶层 / 141 含子用例 PASS；再补 3 个代理/全权密钥用例与 6 个 service 授权/事务用例，均通过。unit 中全部 23 个 SCOPED_ADMIN_TEST_DSN 门控用例已覆盖，无 skip（pg-coverage.json 集合核对） |
| 索引与 schema | API Key cost、Token Analysis client request/due 三个索引均 valid/ready；30m/12h 列 NOT NULL、默认 0；临时数据库已停止 |
| Ent / Wire | 各连续生成两次，首轮及第二轮均无源文件漂移；插件账号目录本地 provider 保留 |
| Go normal / embed build | 均通过；embed 使用本轮前端 dist，`-version` 返回 0.2.21 |
| golangci-lint 2.14.0 | 增量、Windows 全量、GOOS=linux 全量最终均 0 issues；冲突拼接处 gofmt 格式问题已修正 |
| go mod tidy -diff | 退出 1，仅提议删除上游本次新增的 8 行 go.sum 校验和；go.sum 保持与 MERGE_HEAD 相同，未顺手整理依赖元数据 |
| Git / 文档硬门 | 未合并索引条目和冲突标记为 0；160 仅上游路径逐 blob 与目标一致；775 仅本地路径只更新本轮文档/图谱，源码保持第一父；旧 SQL 零变更、26 份 features 零变更、旧文档目录未恢复；台账只追加 |
| diff --check | 排除上游原生 EOF 空行的 migration 250 后通过；该 SQL blob 与 MERGE_HEAD 完全相同，不在合并中整理 |
| Wiki 图谱 | 35 nodes / 76 edges，62 wikilinks / 0 unresolved；未提交阶段 meta 仍以第一父为 Git 基线，不宣称提交后 READY |


Go 1.27.3，pnpm 9.15.4；Go default/unit/integration 顺序执行，各命令使用全新 GOTMPDIR、仓库固定 Go 缓存及 Git `usr/bin` 的 sh。Ent/Wire 工具临时增加的 go.sum checksum 已还原至合并快照。原始脚本、日志、三方差异和校验 JSON 位于仓库外 `E:/tmp/xyai-merge-221-20261111/`，不入库。

## 6 遗留与验证边界

| 事项 | 归属 / 处理 |
| --- | --- |
| repository 完整 Docker/Redis 集成 | 本机无 Docker，integration 全量中整包跳过；已补隔离 PG 的 202 个功能用例，但不等于完整 Docker/Redis 套件 |
| 显式 skip | 分类明细见下表；SCOPED_ADMIN_TEST_DSN 门控已全部补跑，其余遵循现有外部服务/凭据/网络/性能开关；没有把 skip 写成 PASS |
| 上游 SQL EOF 空行 | `250_openai_cache_write_ttl.sql:25` 自带末尾空行，保持指定上游 blob，diff-check 明确豁免这一文件；非本轮修补范围 |
| 上游 go.sum 整理差异 | tidy 提议移除 go-runewidth v0.0.15、tablewriter v0.0.5、uniseg v0.2.0、cobra v1.7.0 的各两行校验和；这 8 行恰好等于 base→指定上游的 go.sum 增量。按合并边界原样保留，不当作依赖或构建失败修补 |
| 请求体预算与本地归档 | RequestArchive/RequestIntercept 的整体读取仍不计入 body memory budget，第一父已有边界保留；自动预算最低 1 GiB，部署前核对显式配置与机器大小 |
| 实际外部行为 | 未做生产、真实模型/支付/SMTP、真实浏览器登录或完整端到端验收；本轮是本地合并验证，不是上线验收 |
| 上游 TTFT 测试时序失败 | `TestOpenAIWSPassthroughFirstTokenFollowsTTFTMode`：unit 全量 visible 第二轮测得 1.084s < 1.1s；合并结果精确复跑还出现 499ms < 500ms。指定上游完整 backend 原始快照也复现，因此不属于合并造成的本地功能回归。测试与 relay 源文件逐 blob 等于 MERGE_HEAD，未修改测试断言或上游实现，等待上游解决；当前全量 unit 不是全绿 |

复现证据：合并结果精确 -count=3：2 PASS / 1 FAIL；纯上游快照精确 -count=3：1 PASS / 2 FAIL。纯上游快照通过 `git archive <指定 SHA> backend` 导出到仓库外，不替换当前工作区；两侧使用同一 Go 工具链和环境。测试的假上游连续播放第二轮延迟，而实际 TTFT 从第二轮请求被接收后开始；测试将全部名义延迟作为无下界容差的最小耗时，存在调度与毫秒取整敏感性。此解释不代表已完成生产 TTFT 正确性验收。

| 跳过原因 | default | unit | integration | PG 专项 |
| --- | ---: | ---: | ---: | ---: |
| DingTalk sentinel 占位用例 | 1 | 1 | 1 | 0 |
| Linux 抓包样本平台限制 | 1 | 1 | 1 | 0 |
| Prompt Audit 外部 Redis | 3 | 3 | 3 | 0 |
| Prompt Audit 独立 PG DSN | 6 | 6 | 6 | 0 |
| TLS 指纹外网开关 | 0 | 3 | 0 | 0 |
| auth_pending_identity_service_test.go:365: legacy NULL pending_auth_session_id rows only exist in production PostgreSQL history; sqlite unit schema rejects NULL | 0 | 1 | 0 | 0 |
| auth_rate_limit_integration_test.go:25: Docker 未启用，跳过认证限流集成测试 | 0 | 0 | 1 | 0 |
| dialer_capture_test.go:48: 跳过外部 TLS 指纹 capture 测试：未设置 TLSFINGERPRINT_CAPTURE_URL | 0 | 0 | 1 | 0 |
| rate_limiter_integration_test.go:28: Docker 未启用，跳过依赖 testcontainers 的集成测试 | 0 | 0 | 1 | 0 |
| rate_limiter_integration_test.go:60: Docker 未启用，跳过依赖 testcontainers 的集成测试 | 0 | 0 | 1 | 0 |
| 性能 / EXPLAIN 开关 | 0 | 0 | 0 | 5 |
| 插件进程包/环境未提供 | 1 | 1 | 1 | 0 |
| 真实模型凭据/开关 | 2 | 2 | 2 | 0 |
| 管理密钥 PG DSN（已补跑） | 0 | 23 | 0 | 0 |


## 7 提交前状态

HEAD 和 main 仍为第一父，MERGE_HEAD 固定为指定上游。所有合并与文档改动已暂存，未创建合并提交，等待用户审核后明确授权 commit。
