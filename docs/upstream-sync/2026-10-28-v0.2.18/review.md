# 0.2.18 上游合并审核

## 范围与状态

| 项目 | 结果 |
| --- | --- |
| 日期 | 2026-10-28 |
| 分支 | `feature/hy/10224_merge_sub2api_218` |
| 第一父 / 本地 main | `b3940712d7c7d6c1b7a7550fa661583363a3423f` |
| 固定目标 | `Wei-Shaw/sub2api main@5c185c5c5d4f5e409e2139806e0964ae944e03e8`，VERSION 0.2.18 |
| 上游增量 | 45 commits / 114 paths（相对 merge base）；相对本地 main 的自动 merge 为 113 paths，既有 PR 文件不重复变化 |
| merge base | `b0a02a3304a0a78cdd003ddda1dd23ba02c03084` |
| fetch 时 upstream/main | `f178e5ae3`，后续提交不纳入本轮 |
| 合并方式 | `git merge --no-commit --no-ff <固定目标>` |
| 文本冲突 | 无冲突；测试发现 1 处本地断言语义冲突，已按上游新 payload 合同适配 |
| 合并提交 | 尚未创建，保留 MERGE_HEAD 等待用户审核 |
| 边界 | 仅处理合并兼容；不修复上游自身 bug，不提交、不推送、不部署 |

## 三方核对

初始 merge index 的核对证据：`backend/.gocache/merge218-audit.json`（本机忽略目录）。

| 范围 | 结论 |
| --- | --- |
| 95 个仅上游修改路径 | index blob 全部等于指定上游提交 |
| 971 个仅本地修改路径 | 文档维护前 index blob 全部等于第一父，含本地删除路径的状态 |
| 19 个双方修改路径 | 16 个增删行序列等于 merge-base→目标，保留双方增量；另 3 个重叠 PR #8139 见下 |
| 34 个 docs/features 文件 | 逐 blob 等于第一父，零删除；包括设计/素材，不代表所有规划均已实现 |
| 重叠功能 | PR #8139 本地已提前应用，指定上游正式包含该 PR；metadata/queued 分类及测试按上游整合，不重复应用 |

### 双方修改路径裁决

| 路径 | 核对结论 |
| --- | --- |
| README.md / README_CN.md | 合入上游赞助资料，保留本地内容 |
| backend/go.mod | 采用上游 Go 1.27.3，保留本地依赖 |
| backend/internal/handler/dto/mappers.go | 采用 SupportsAccountQuota，保留 AdminPermissions/部门及权限版本 DTO |
| backend/internal/handler/gemini_v1beta_handler.go | 采用上游生图 gate/懒探测，保留本地 ParseGeminiModelActionPath |
| backend/internal/service/openai_capacity_shed_test.go | 既有 PR #8139 与上游后续空输出/Retry-After 测试自然合并 |
| backend/internal/service/openai_gateway_passthrough.go | 既有 metadata 分类保留，吸收上游空字符串 delta/done 输出边界；本地兼容增量保留 |
| backend/internal/service/openai_gateway_request_body.go | 采用 DeepSeek image_url 归一化，保留本地 thinking 字段清理 |
| backend/internal/service/openai_oauth_passthrough_test.go | 更新上游 CLI 身份预期，保留本地测试 |
| backend/internal/service/openai_ws_http_bridge_test.go | 上游与第一父相同（此前 PR #8139），无需重复修改 |
| backend/internal/service/settings_view.go | TUI 注释更新为 CLI，本地设置字段保留 |
| frontend/package.json / pnpm-lock.yaml | 上游 DOMPurify/follow-redirects 约束合入，本地 html2canvas 等依赖保留 |
| frontend/src/components/account/CreateAccountModal.vue | 支持 service_account 配额，保留本地 preset 和子管理员接入 |
| frontend/src/components/account/__tests__/AccountUsageCell.spec.ts | 添加上游 Vertex 场景，保留本地额度展示测试 |
| frontend/src/components/admin/account/AccountActionMenu.vue | 支持 service_account 配额判断，useAccountOperatorAccess 权限门不变 |
| frontend/src/i18n/locales/{en,zh}/admin/settings.ts | 使用上游 CLI 文案，保留本地词条 |
| frontend/src/views/admin/AccountsView.vue | 增加平台/状态筛选持久化，保留子管理员、GPT 额度与列表本地逻辑 |

### 测试发现的语义冲突

`backend/internal/service/token_analysis_forward_facts_test.go#TestTokenAnalysisForwardFactsHashBuilderFinalBody` 的 standard/passthrough 两个场景原先要求最终提升出的图片带 `url`。指定上游 `afbef7c8c` 已改为只保留 `image_url`，上游自己的 DeepSeek 测试也同步删除重复 `url` 预期。首次 default 因这一本地旧前置断言失败，尚未执行后续哈希断言。

仅将本地测试改为检查 `type=input_image`、精确 `image_url` 和 `url` 不存在，保留原最终正文 items/root 哈希断言；生产代码不动。修正后 Token Analysis 最终正文与上游 DeepSeek 归一化专项通过。这是本地测试对上游协议变更的冲突适配，不是修复上游 bug。

## 本地能力保留

- Token Analysis 风险/来源/用户输入/usage-link、RequestArchive/RequestIntercept、Prompt Metrics/Risk、组织/部门及日汇总、子管理员账号运维、订阅自助重置、并发预设、GPT 共享额度与圆环展示的本地独有生产源码保持第一父；仅上述本地测试断言作冲突适配。
- OpenAI 默认 effort、compatible cache usage、请求参数过滤、大请求保护、空响应检查继续存在；网关更新不替换这些独立链路。
- 上游 Vertex 账号配额与本地 GPT 共享额度不是同一功能，分别保留；未识别到需删除的本地独有功能。

## 新行为与审核注意事项

| 事项 | 结论 |
| --- | --- |
| Gemini 生图迁移 | 新增 246_enable_gemini_image_generation_groups.sql，既有 Gemini/Antigravity false 开关将改为 true；Composite 不受该 SQL 影响 |
| 同号迁移 | 与本地 246_organization_usage_daily_rollups.sql 按完整 filename/checksum 共存；不重命名、不改旧 SQL |
| 账号配额 | Vertex service_account 纳入额度/计费/停调度；SQL 按日/周设置选变体并带回退；本地用户平台额度独立 |
| 网关 | 兼容流断开收尾计费、Gemini 非负 input、Codex CLI 身份/空输出重试/Retry-After、DeepSeek 图片等采用指定上游 |
| 调度 | 进程内只读快照母本复用、模型映射签名与限流写时复制；单 Antigravity 账号按需探测 |
| 图片安全 | 上游已补目的地/重定向/直连 IP 检查，修正 wiki 的旧“尚无 SSRF 防护”描述；代理解析边界仍保留 |
| 构建版本差异 | backend/go.mod、CI、根 Dockerfile 为 1.27.3；backend/Dockerfile、deploy/Dockerfile 仍 1.27.2，三个文件与指定上游一致。记录上游不一致，不本地修复 |
| schema / provider | 没有 Ent schema 或 Wire provider 变更，不进行无关生成 |

## 失败归属（不混入修复）

| 用例/边界 | 证据与处理 |
| --- | --- |
| 两个 DashboardAggregation 分区清理 unit | mock 期望直接 DROP，但本地日汇总实现先执行 mark_user_usage_rollup_dirty_day。原始 main 快照两个用例均复现同样失败，相关实现/测试本轮未变；不修 |
| TestApplyDefaultOpenAIReasoningEffort 的 none 场景 | 原始 main 快照复现旧断言失配；本地测试与默认 effort 实现本轮未变；不修 |
| TestCommandCodeGatewayPassesThroughCatalogProtocols | 合并结果 unit 全量失败；合并结果与原始 main 各 count=3 聚焦均为 2 pass / 1 fail，同为 model_protocol_catalog_test.go:374。目录/路由相关文件本轮未变；保留波动记录，不修 |
| SameCodexThreadStillPreempts | unit 全量等待 ingress websocket 结束超时；合并结果与原始 main 各 count=3 聚焦均 3/3 通过。相关 WebSocket execution-scope 测试/实现未被本轮改动；保留全量时序失败，不放宽断言，不宣称证明所有调度条件均正常 |
| 工具准备 | 初始宿主 GOROOT 指向旧 Go 导致命令/编译器不一致；正式结果使用同一 Go 1.27.3 GOROOT/PATH 与 local toolchain。前端正式安装使用 pnpm 9.15.9 frozen lockfile |

本地基线通过 `git archive main backend` 导出到忽略目录 `backend/.gocache/merge218-main-baseline`，未修改原始快照的源码/断言；验证使用同一 Go 1.27.3 工具链。完整失败日志和聚焦结果保留，不能以复跑通过掩盖全量失败。

## 验证

| 验证 | 实际结果 |
| --- | --- |
| pnpm 9.15.9 install --frozen-lockfile | 通过，未产生锁文件漂移 |
| 前端 lint:check / typecheck | 通过；lint 使用只读模式 |
| 完整 Vitest | 414 files / 3211 tests 全通过 |
| 前端 build | 通过，i18n 3/3、1180 modules；保留 Browserslist、动态导入和大 chunk 提示，不做无关优化 |
| Go default 全量（首次） | 退出 1，53 包通过；仅 service 中本地 DeepSeek 旧字段断言失败（1 顶层/2 子用例），其余 14415 条 PASS，14 显式 skip |
| 语义冲突专项 | 修正后 2 顶层/13 含子用例 PASS；验证最终正文哈希与上游 DeepSeek 归一化 |
| Go default service 全包（修正后） | 退出 0，4395 顶层/8933 含子用例 PASS，3 skip；其他 53 包源码/测试未再改动，沿用首轮结果，未声称首轮命令全绿 |
| Go unit 全量 | 退出 1，58 包通过、2 包失败；12589 顶层/24591 含子用例 PASS，5 个顶层失败（含子用例共 6 FAIL）、18 skip；失败归属见上一节 |
| Go integration 全量 | 退出 0，54 包返回通过，6987 顶层/13602 含子用例 PASS，18 显式 skip；repository 因 Docker 缺失整包跳过，不能视为完整 Docker/Redis 集成通过 |
| 隔离 PostgreSQL 18.1 专项 | 退出 0，21 顶层/84 含子用例 PASS，0 skip；覆盖 quota SQL 变体/service_account/删除账号、迁移幂等/会话锁、本地部门/自助重置/GPT 额度 |
| 迁移实测 | 两个 246 文件均在 schema_migrations 中；事务内造 Gemini/Antigravity/Composite 分组，执行上游原 SQL 后结果依次 true/true/false；已回滚 fixture，临时 PG 已 fast stop |
| Go normal / embed build | 均通过；embed 使用本轮新前端产物 |
| golangci-lint 2.14 --new-from-rev=HEAD | 本地测试适配后再次通过，0 issues |
| Wiki 图谱 | 35 nodes / 76 edges、60 wikilinks / 0 unresolved；源码 hash 一致，check-understand-status.cmd -AllowDirtyWiki 为 READY；不是代码图谱重建或生产验收 |

正式 Go 命令使用 `-p 1 -count=1 -json`，全量套件为 `./...`；service 复跑仅 `./internal/service`，unit/integration 分别加对应 `-tags`。default/unit/integration 全量按顺序运行，避免 Ent 生成测试的共享目录竞争。PG 专项和 service 专项不运行 Ent 生成测试。

未完成 Docker/Redis 全套集成、真实模型/支付/SMTP、浏览器端到端或生产验证；本轮未部署。显式 test skip 与没有测试的包分别统计，不将返回 ok 的整包跳过当作真实数据库测试通过。

本机日志均为 `backend/.gocache/merge218-*`，不入库。测试不得把 Docker/外部依赖 skip 视为实际通过，不触达生产数据库或真实支付/模型业务。


## 交付状态

- 保持 `HEAD=main=b3940712d7c7d6c1b7a7550fa661583363a3423f`、`MERGE_HEAD=5c185c5c5d4f5e409e2139806e0964ae944e03e8`；未 commit、未 push、未合回 main、未部署。
- 待审内容为固定上游增量、1 处本地测试语义适配、wiki/组件 README/图谱及本报告与追加合并台账。没有独立生产 bug 修补。
- 95 个仅上游路径仍逐 blob 等于目标；34 个 features 文件仍逐 blob 等于第一父；本地独有生产源码未改写。无未解决冲突，所有交付改动暂存，供用户审核后决定是否 commit。
- 合并台账保留原文件完整字节前缀，只追加本轮。最终审计：`backend/.gocache/merge218-final-audit.json`；测试汇总：`backend/.gocache/merge218-test-summary.json`。这些本机日志/快照/数据库目录均忽略，不提交。

本轮不是全绿交付：unit 的 5 个顶层失败和完整 Docker/外部服务/浏览器覆盖缺口明确保留。此次完成合并与验证，等待人工 commit 审核，不代表上线批准。
