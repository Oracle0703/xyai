# 0.2.23 上游合并审核

日期：2026-11-18；分支：`feature/hy/10236_merge_sub2api_223`；状态：未提交，等待用户审核。

## 1 范围

| 项目 | 固定值 / 约束 |
| --- | --- |
| 当前第一父 | 本地 `main@74af5ac80ab13c8bc0c134a674c75fbf07e17072`；迁回原仓库时快进纳入 |
| 初次验证基线 | `d8dab5fad236a73a22f9701094931c85f7d08ca0`；下文既有“第一父”测试对照和原始索引统计以此为准 |
| 上游 | `Wei-Shaw/sub2api main@2af3f1e15c98c893a89255a037f551046883b8c0`，版本 0.2.23 |
| merge-base | `15871f904056f548dd874a1d069ffc6cdf777b78` |
| 增量 | 上游 32 commits / 90 paths；本地 834 paths；双方修改 15 paths |
| 合并方式 | `git merge --no-commit --no-ff <固定 SHA>`；保持 MERGE_HEAD，不 commit / push / 合回 main |
| 范围 | 仅解决文本及必要语义冲突；保留 `docs/features/` 记录的本地独有功能，真正重叠采用上游行为；不修上游自身 bug |
| 工作位置 | 已按用户要求迁回 `E:/allsite/xyai` 的同名分支，仍为未提交合并。原仓库此前两项修改已进入 `74af5ac80`，均已保留；原临时工作树已从 Git 登记和文件系统删除 |

迁回时，`74af5ac80` 相对初次验证基线只删除旧演示 HTML 并补充 backend wiki 的 Grok 排障说明，没有业务代码变化。迁回后的完整暂存树先与“原审核树 + 最新 main 两项修改”的独立重建树逐项核对相等；backend、frontend 子树与已测试版本完全相同。随后仅更新本报告、台账、wiki 状态和图谱；本次迁移不重复运行全量业务测试，原测试结果和失败边界继续保留。恢复补丁、索引与合并状态备份位于原证据目录的 `transfer-to-primary/`。

## 2 冲突与处理

唯一文本冲突：`backend/internal/service/openai_gateway_messages.go`。

上游把 `handleAnthropicStreamingResponse` 终态用量顺序调整为顶层 `usage` 在先、嵌套 `response.usage` 在后；本地已经是这一顺序，并补充 compatible cache 字段及嵌套缺省时的顶层对象复用。因此保留本地完整段落、去掉自动合并插入的重复赋值，最终文件等于第一父；保留上游要求的嵌套用量优先行为。直接采用上游整个文件会删除尚未被覆盖的本地兼容处理，不适合此处。

| 双方改动路径 | 核对结论 |
| --- | --- |
| `pkg/apicompat/chatcompletions_responses_bridge.go`、`types.go` | 上游 cache presence/TTL 语义与本地 Responses→Chat 参数过滤、stream lifecycle 类型及 cache key 字段互补 |
| `service/openai_codex_transform.go` | 上游 GPT-6.2 模型映射与本地默认 effort 判定、独立上下文 id/namespace 原样保留互补 |
| `service/openai_gateway_cc_pipeline.go` | 上游 flush-before-read helper 与本地兼容缓存补充并存 |
| `service/openai_gateway_chat_completions_raw.go`、对应测试 | 上游合并 flush 与本地 thinking 清理、compatible cache 回显并存 |
| `service/openai_gateway_messages.go` | 按上述方式解决，嵌套 usage 优先；上游新增 `TestResponsesTerminalUsageMatchesAnthropicOutput` 验证该合同 |
| `service/openai_gateway_request_body.go` | GPT-6.2 采样/Ultrafast 权益/effort 校验与本地 thinking 清理并存 |
| `service/openai_gateway_response_handling.go` | 上游 idle deadline/首输出切换预算与本地兼容缓存别名/TTL 补充并存 |
| `service/openai_gateway_responses_chat_fallback.go` | 上游 GPT-6.2 采样剔除、DeepSeek 单遍编辑与本地第三方参数过滤并存 |
| `service/openai_gateway_usage.go` | 上游 cache billing projection/Ultrafast 价格时点与本地图片零价记录、缺价边界并存 |
| `i18n/locales/{en,zh}/admin/{channels,overview}.ts` | 上游 Ultra Fast 文案与本地 Prompt Risk、日重置、组织/部门和子管理员权限文案并存 |

表中 Go 路径位于 `backend/internal/`，i18n 路径位于 `frontend/src/`。14 个自动合并文件双向增删行多重集合逐项相等：结果相对目标的差异等于本地增量，结果相对第一父的差异等于上游增量。唯一例外为上述已等价覆盖的手工冲突文件。文档更新前，75 个仅上游路径逐 blob 等于目标，819 个仅本地路径逐 blob 等于第一父；最终文档例外与硬门见第 5 节。

## 3 保留的本地能力

| 能力 | 保留依据 |
| --- | --- |
| Token Analysis、RequestArchive/RequestIntercept、Prompt Metrics/Risk/judge | 本地路径及路由/接线保持第一父，国际化自动合并保留本地增量；上游未新增网关入口 |
| 组织/部门/负责人/分组归属、子管理员与账号操作员 | schema、迁移、范围、授权白名单及页面保持第一父；没有新增管理路由需要扩权 |
| 生图页、用户用量账户列、订阅自助/批量日重置、GPT 额度、用户并发预设 | 仅本地路径与第一父一致 |
| OpenAI 本地适配 | 默认 effort、参数过滤、compatible cache、时区改写、空响应兜底、大请求保护、并发错误分类保留；共享文件见第 2 节 |
| Codex 额外独立上下文 | 7 个本地生产者及 bootstrap 并存兜底保留；handler 文件与第一父一致，service 两处字段保留未丢失 |
| Antigravity 本地身份/Dual-use 改写 | 第一父实现保留，本轮上游仅改变该网关的 usage 提取 |
| 文档与迁移 | `docs/features/` 26 个 tracked 文件零修改/零删除；既有 SQL migration 不改不重命名，新 252 与本地 252 共存 |

## 4 采用的上游行为

| 增量 | 结果 |
| --- | --- |
| Anthropic/OpenAI 缓存与 TTL | 保留快照字段存在性、aggregate 与原始分项；桥接、计费及未知 TTL 分配沿指定提交；OpenAI 30m/12h 与 Anthropic 5m/1h 分开 |
| GPT-6.2 Sol/Astra | 模型目录/metadata、别名、effort、工具协议、采样、价格与 Pro Ultrafast 权益沿上游 |
| 渠道 Ultra Fast | 可选正数 `ultrafast_multiplier`，NULL 继承模型默认；migration、SQL repository、API、价格 resolver、前端与测试一并采用 |
| SSE idle | 按最后读取时间重设 Timer；输出前 idle 可进入一次首输出账号切换预算；已有输出后不切换 |
| 性能与测试 | Chat 读取前合并 flush、DeepSeek/Ollama 单遍改写、billing heap 用例调整均沿上游 |
| 依赖 | 第一父已为 Go 1.27.4；本轮 go.mod/go.sum、前端 package.json/lockfile 无增量 |

## 5 验证

| 验证 | 结果 |
| --- | --- |
| 前端 frozen install / lint:check / typecheck | 通过；package.json / pnpm-lock.yaml 零漂移，独立 TEMP/TMP |
| 完整 Vitest | 441 files：440 通过、1 失败；3661 tests：3660 通过、1 失败。唯一失败是上游 Ultra Fast 文案断言失配，归属见第 6 节 |
| 前端 build | 通过（含 check:i18n、vue-tsc、Vite）；已有 chunk 大小提示保留 |
| 前端失败对照 | `git archive` 第一父同一文件 4/4 通过；纯指定上游 3/4 通过，同一断言失败。测试文件与上游同 blob；不改文案或测试 |
| 冲突相关 Go unit 专项 | service / apicompat / handler 3 包通过，222 顶层 / 1085 含子用例 PASS，0 FAIL / 0 SKIP；覆盖用量优先级、缓存转换、Codex 独立上下文及 Ultrafast |
| Go default 全量 | `go test -json -p 1 -count=1 -timeout 60m ./...`：退出 0；57 包、8019 顶层 / 16963 含子用例 PASS，14 显式 SKIP |
| Go unit 全量（CI tag） | `go test -json -p 1 -count=1 -timeout 60m -tags=unit ./...`：退出 0；63 包、13750 顶层 / 30812 含子用例 PASS，41 显式 SKIP |
| Go integration 全量 | `go test -json -p 1 -count=1 -timeout 60m -tags=integration ./...`：退出 0；57 包、7517 顶层 / 16121 含子用例 PASS，18 显式 SKIP；repository 包因无 Docker 整包跳过，不计作数据库用例通过 |
| 隔离 PostgreSQL 18.3 升级 | 第一父 334 条迁移 → 合并结果 335 条，唯一新增 `252_channel_pricing_ultrafast_multiplier.sql`；既有 checksum 零变化。存量 Fast=2.5 / Flex=0.5 不变，新字段 NULL；列为 nullable numeric(12,6) |
| PG 渠道与本地能力专项 | 合并结果 120 顶层 / 184 含子用例 PASS；第一父相同选择 118 顶层 / 182 含子用例 PASS。两侧同为订阅列表 suite 的 6 个子用例失败，均受此前测试残留的 4 条订阅影响；5 个性能/EXPLAIN 开关 SKIP。合并结果在全新库单独跑订阅 suite：54/54 含子用例通过 |
| PG 扩大回归 | 仓库外 overlay 只给 postgres-only harness 补 `integrationDSN = dsn`；跳过 16 个 Redis 顶层入口和含 Redis 子用例的 `TestAccountRepoSuite`。853 顶层 / 1670 含子用例 PASS，6 个显式 SKIP；1 个部门并发测试等待 pg_stat_activity 锁状态超时。第一父相同条件的新库整包 852 顶层 / 1669 含子用例 PASS、6 SKIP、0 FAIL；双方各在全新库精确 `-count=3` 复跑该并发用例，均 3/3 PASS |
| Ent / Wire | 各生成 3 次，末轮逐文件 SHA-256 相对上一轮零漂移，生成代码与合并索引一致；工具临时增加的 google/subcommands、inconshreveable/mousetrap checksum 共 4 行已还原。`ProvidePluginManager` 与 `SetAccountDirectory` 接线保留 |
| golangci-lint | 增量（`--new-from-rev=d8dab5fad236a73a22f9701094931c85f7d08ca0`）、Windows 全量、`GOOS=linux` 全量均 0 issues；全量使用 `--max-same-issues=0 --max-issues-per-linter=0` |
| Go normal / embed build | 均通过：`go build -trimpath ./cmd/server`、`go build -tags=embed -trimpath ./cmd/server`；embed 使用本轮前端 dist，`-version` 返回 0.2.23；二进制只放仓库外 |
| go mod tidy -diff | 退出 1，仅提议删除 go-runewidth v0.0.15、tablewriter v0.0.5、uniseg v0.2.0、cobra v1.7.0 各 2 行旧 checksum。第一父同工具链复现相同差异；go.mod/go.sum 与第一父、目标均未变化，不作依赖整理 |
| Git / 文档硬门 | 未合并索引条目 0、冲突标记 0、`git diff --cached --check` 通过；75 个仅上游路径逐 blob 等于目标；819 个仅本地路径中 808 个等于第一父，例外仅本轮 11 个文档/图谱文件。14 个自动合并文件保留双方全部增量，唯一手工冲突文件等于第一父；`docs/features/` 26 文件零变更。唯一新增本地文档为本 review，台账只追加、旧目录未恢复；共暂存 101 个路径，无未暂存或未跟踪文件，HEAD/main/MERGE_HEAD 均符合固定边界 |
| 工作树迁回验证 | 原审核树 `f95b309b575ce1c6d2d8918604b061e15dff177e` 与最新 main 增量独立组合得到 `89e4be3d642b40ebb85f80d200b07d706b9e3395`，迁回后的暂存树精确一致；业务代码未变，最新 main 的演示删除和 Grok wiki 补充保留。迁移记录更新后再次检查暂存区与合并边界 |
| Wiki 图谱 | 已刷新：35 nodes / 76 edges；迁回后按当前 wiki 和 `74af5ac80` 重新刷新，正文与图谱元数据使用 UTF-8 / LF |

Go 1.27.4；golangci-lint 2.14.0（Go 1.27.4 构建）；pnpm 9.15.4 / Node 24.13.0。三种 Go tag 顺序执行、每个命令独立 GOTMPDIR；复用本机已安装的 Go module cache 与仓库 GOCACHE。原始日志、快照、脚本和归属证据只放仓库外 `E:/tmp/xyai-merge-223-20261118/`。逐项 SKIP 名称及原因保存在该目录的 `logs/go-default-skips.txt`、`logs/go-unit-skips.txt`、`logs/go-integration-skips.txt`、`logs/pg-full-skips.txt`；主要边界为 Docker/Redis、Prompt Audit 外部 DSN、真实 API/TLS capture/插件包未配置，以及性能开关未开启。临时 PostgreSQL 已停止。

## 6 遗留与提交前状态

| 事项 | 归属与处理 |
| --- | --- |
| Ultra Fast 文案断言 | 上游原生：`frontend/src/views/admin/__tests__/groupsOpenAIFast.spec.ts` 要求提示包含 `service_tier=ultrafast`，上游新的 GPT-6.2 提示已移除该片段。第一父通过、纯上游同样失败；不改文案、断言或上游实现，等待上游处理 |
| PG 部分选择的订阅夹具残留 | 第一父既有；相同测试选择在新库重现相同 6 项子用例失败；合并结果的订阅 suite 在另一新库单独运行全部通过。未修测试或业务代码 |
| PG 并发时序 | `TestDepartmentSubscriptionsIntegration_ReadAndResetIsolation` 等待 `pg_stat_activity` 锁状态超过 5s；相关测试、部门及订阅实现与第一父相同。第一父整包通过、双方精确复跑各 3/3 通过，根因未闭环，保留原失败，不宣称整个 PG 扩大回归全绿，也未修改业务代码或放宽断言 |
| 依赖元数据 | tidy 的 8 行旧 checksum 清理建议在第一父同样存在；保留既有依赖文件，未混入工具生成所需的临时 checksum |
| 集成环境 | 无 Docker/真实 Redis；标准 integration 的 repository harness 整包跳过不能算数据库通过。PG 覆盖来自独立新库与明确列出的 overlay/跳过范围，不是完整 Docker/Redis 套件 |
| 实际外部行为 | 未做生产、真实模型/支付或浏览器端到端验收 |

合并及上述验证已完成，失败与跳过边界如上；合并结果、wiki、图谱、组件说明、本 review 与只追加台账一并暂存。原仓库当前分支为 `feature/hy/10236_merge_sub2api_223`，HEAD 为 `74af5ac80ab13c8bc0c134a674c75fbf07e17072`，MERGE_HEAD 固定指定上游，未 commit、push 或合回 main，等待用户审核。
