# 0.2.19 上游合并审核

## 范围与状态

| 项目 | 结果 |
| --- | --- |
| 日期 | 2026-10-29 |
| 位置 | 原仓库工作目录；按用户要求不使用 worktree |
| 前置提交 | 用户授权先提交公司介绍资料：`eb34cc973ef99e9e20b9c6f8e7f4936a0551cd11`，位于本地 main，未推送 |
| 工作分支 | `feature/hy/10225_merge_sub2api_219` |
| 第一父 / 本地 main | `eb34cc973ef99e9e20b9c6f8e7f4936a0551cd11` |
| 固定上游 | `Wei-Shaw/sub2api main@a9240d5016c226b341242a8c54a01a8605cbbc72`，VERSION 0.2.19 |
| merge base | `5c185c5c5d4f5e409e2139806e0964ae944e03e8` |
| fetch 时 upstream/main | `a3298b46e`；其后于指定目标的内容不纳入 |
| 上游增量 | 71 commits / 259 paths，46 个双方修改路径 |
| 合并方式 | `git merge --no-commit --no-ff <固定目标>` |
| 合并提交 | 尚未创建，保留 MERGE_HEAD 等待用户审核 |
| 边界 | 仅解决文本/语义合并冲突；不修上游自身 bug，不推送、不合回 main、不部署 |

## 文本冲突裁决

| 文件 | 双方内容与处理 |
| --- | --- |
| `.gitignore` | 本地 docs 分类白名单与上游 admin-api-keys 文档白名单合并保留 |
| `backend/go.mod` | x/net/crypto/sync/sys/term/text/tools 采用目标上游版本；本地直接引用的 sys/text 仍列直接依赖，不重复声明 |
| `backend/internal/config/config_test.go` | 本地默认 effort 校验与上游 Gemini 同账号重试边界用例同时保留 |
| `backend/internal/handler/admin/subscription_handler.go` | 保留本地部门授权及 scope_version 分页信封，同时对 items 使用上游受限密钥字段投影；其他返回分支沿上游投影 |
| `backend/internal/server/routes/admin.go` | 同时挂本地子管理员操作通知与上游受限密钥账户处理器；权限管理路由和 step-up 沿上游 |
| `backend/internal/service/admin_service_role_test.go` | 保留本地子管理员/CAS 用例和上游 400 错误断言；新增降级用例补本地必需的 ExpectedAdminAccessVersion |
| `backend/internal/service/openai_gateway_response_handling.go` | 双方重叠的 prompt_cache_hit_tokens 行采用上游含说明版本；本地其他兼容字段不丢失 |
| `backend/internal/service/openai_gateway_usage.go` | 上游图片端点按张计费优先级与本地图片 token 价格缺失拒绝并存；提升 resolved 变量作用域供本地检查使用 |

## 自动合并的必要适配

| 位置 | 问题与处理 |
| --- | --- |
| `handler/admin/idempotency_helper.go` | 自动合并把上游 present 回调留在外层、本地 writeAdminIdempotentJSONResult 在内层引用，导致未定义变量。统一传入投影，nil 为恒等映射；保留本地 AtomicSuccess 与 fail-close，执行/降级/重放均沿已有响应出口 |
| `server/middleware/admin_auth.go` | 新受限密钥认证缺少本地订阅所需的 DepartmentActor。追加已认证创建者身份且保留 Principal；只用 WithDepartmentActor，不复用旧全权密钥标记，数据库仍复核创建者角色/状态，路由权限不扩大 |
| `handler/admin/scoped_admin_subscription_response_test.go` | 上游新测试使用单参构造器，与本地双参生产构造器冲突；改用已有 newSubscriptionHandler 测试入口，保留响应脱敏与幂等重放断言 |
| `cmd/server/wire_gen.go` | 运行真实 Wire 生成，仅把新 ScopedAdminKeyRepository 初始化移到依赖图顺序；本地 provider/cleanup 接线保留 |
| 合并回归 | 新增受限密钥身份与未知本地路由拒绝、部门分页信封字段脱敏两项回归；既有 AtomicSuccess、scope、子管理员及上游投影测试继续执行 |

这些改动适配的是本地与上游同时存在后的冲突，不是额外修复上游实现。

## 三方与本地能力核对

| 范围 | 核对结论 |
| --- | --- |
| 34 个 docs/features 文件（含素材） | 逐 blob 等于第一父，零删除；设计文件存在不等于所有规划均已实现 |
| 949 个仅本地修改路径 | 文档维护前逐 blob 等于第一父；最终仅允许本轮 wiki/组件 README/台账等文档维护 |
| 213 个仅上游修改路径 | 除上游订阅新测试的构造器适配，其余逐 blob 等于目标；无上游独有生产文件补丁 |
| 46 个双方修改路径 | 初次核对 39 个路径的增删行序列等于 merge-base→目标；其余 7 个见冲突及语义适配。随后 Wire 顺序由生成器调整 |
| JWT 子管理员与受限 API 密钥 | 登录角色/部门范围/操作通知与机器密钥按操作/资源授权用途不同，均保留；新上游接口不自动加入本地子管理员白名单 |
| 本地网关链路 | RequestArchive/RequestIntercept、Prompt Metrics/Risk、Token Analysis、默认 effort、compatible cache usage、大请求保护与空响应处理保留 |
| 本地管理/数据功能 | 组织/部门与日汇总、订阅自助重置、GPT 共享额度/圆环、并发预设、quota flusher 及插件目录接线保留 |
| 生图 | 本地 Pinia 持久状态/多图输入与上游后端按张计费是独立层次；不删除本地页面 |
| 重叠实现 | DeepSeek 缓存命中字段采用上游，未重复添加；未识别到应整项删除的本地独有能力 |

## 升级行为

| 事项 | 采用的上游行为 |
| --- | --- |
| 管理密钥 | 新增哈希存储的 scoped_admin_api_keys；JWT 管理员 + step-up 管理，旧全权密钥停止新建但保留已有密钥的轮换/删除 |
| 图片计费 | migration 248 与 Ent 新增 image_count_billing，默认 false；图片端点按实际返回张数计费，无法解析按上游回退；混合文本请求不套强制图片分支 |
| Grok | 模型目录来源/时效/尝试状态、编辑草稿隔离、受管 extra CAS、响应模型审计；新增诊断组件 |
| Gemini | 同号重试可配 0–10（默认 4、不含首次）；绑定缓存的池账号同号重试；声明模型列表/详情一致性和 fileData 图片计数 |
| 模型/价格/兼容 | GPT-6.1 Terra、DeepSeek cache hit 与角色兼容、模型广场分组定价优先、频道 web search 平台维度 |
| 安全/构建 | multipart 超限拒绝、分页防溢出、国产频率 429 秒级冷却；x/net 等依赖升级，Go 仍 1.27.3 |
| migration | 新增 247_scoped_admin_api_keys.sql、248_group_image_count_billing.sql；旧迁移零修改、不重命名本地同号文件 |

## 验证结果

| 验证 | 实际结果 |
| --- | --- |
| pnpm 9.15.4 frozen install | 通过，package.json 与锁文件零漂移 |
| 前端 lint:check / typecheck | 均通过，只读 lint |
| 完整 Vitest | 418 files / 3350 tests 全通过 |
| 前端 build | 通过，保留原动态导入、大 chunk 等构建提示，不做无关优化 |
| Go default 全量 | 退出 0，54 包、7511 顶层 / 14541 含子用例 PASS，14 显式 skip |
| Go unit 全量 | 退出 1，59 包通过、2 包失败；12821 顶层 / 25265 含子用例 PASS，3 顶层失败（含子用例 4 FAIL），35 显式 skip；失败已逐项在原始 main 复现 |
| Go integration 全量 | 退出 0，54 包、7024 顶层 / 13718 含子用例 PASS，18 显式 skip；repository 因 Docker 不可用整包跳过，不能当作完整数据库/Redis 集成通过 |
| 冲突专项 | 首轮 140 顶层 / 213 含子用例 PASS，覆盖订阅投影/重放、AtomicSuccess、角色保护、图片与兼容缓存；最终部门身份标记回归又在正式 unit 中通过 |
| 新密钥隔离 PostgreSQL unit | 31 顶层 / 70 含子用例 PASS，0 skip；随后补齐默认跳过的 5 个代理/Codex/旧密钥用例，5 顶层 / 12 含子用例 PASS，0 skip |
| 隔离 PostgreSQL integration 专项 | 67 顶层 / 101 含子用例 PASS，部门并发 1 个顶层超时失败，2 个性能专项因未启用 opt-in 跳过；覆盖 Grok CAS/受管 extra、迁移、部门、自助重置及 GPT 额度 |
| 部门并发失败对照 | 合并结果与原始 main 各 count=3，均 3/3 通过；保留首次超时，不把复跑通过称为完整首轮通过 |
| 新迁移实测 | 两个 247、两个 248 均在 schema_migrations；image_count_billing 为 NOT NULL boolean、默认 false，fixture 回读 false，更新回读 true 后清理 |
| Ent / Wire | 两条生成命令通过；Ent 与自动 merge 一致，Wire 仅按依赖图调整新仓储初始化顺序；全部本地接线保留 |
| Go normal / embed build | 均通过，embed 使用本轮前端构建产物 |
| golangci-lint 2.14 --new-from-rev=HEAD | 通过，0 issues |
| go mod tidy -diff | 最终通过；测试生成阶段追加的 10 行工具 checksum 已精确去掉，go.sum 恢复目标上游内容，未改变业务依赖 |
| Wiki 图谱 | 35 nodes / 76 edges、60 wikilinks / 0 unresolved；刷新与源码 hash 校验通过；未重建代码图谱 |

正式 Go 全量命令使用 Go 1.27.3、`-p 1 -count=1 -json ./...`，unit/integration 分别加对应 tag，三套依次运行以避免 Ent 生成测试共享目录竞争。初次准备误命中宿主旧 go.exe 已纠正为工具链绝对路径，正式日志不沿用该失败结果。工具生成的 checksum 是验证副产物，不混入上游合并。

隔离 PostgreSQL 18.1 仅监听 127.0.0.1，使用新建 `merge219_test` 与 `merge219_main`，没有读取生产配置。所有数据库专项结束后已 fast stop。SQL 专项不替代 Docker/Redis 全套集成；外部模型、TypeSafe live、Prompt Audit 外部依赖、浏览器端到端、真实 SMTP/支付和生产没有验收。无测试包与显式 skip 分别统计。

## 失败归属（保持原样，不顺带修复）

| 失败/限制 | 证据与判断 |
| --- | --- |
| `TestDashboardAggregationRepositoryCleanupUsageLogsPartitionedSortsAndInvalidatesEachDropBeforeSync` | mock 预期直接 DROP，但既有本地实现先执行 mark_user_usage_rollup_dirty_day；原始 main 同样失败 |
| `TestDashboardAggregationRepositoryCleanupUsageLogsPartitionFailureRollsBackAndStops` | 同一 SQL mock 顺序不匹配；原始 main 同样失败，相关生产/测试源码本轮未变 |
| `TestApplyDefaultOpenAIReasoningEffort/config_none_normalizes_to_empty_->_disabled` | 旧 none 断言失配；原始 main 同样失败，默认 effort 相关实现/测试本轮未修补 |
| `TestDepartmentSubscriptionsIntegration_ReadAndResetIsolation` | 首次在等待并发余额写入的数据库行锁观察处 5 秒超时（测试第 132/148 行）；相关代码本轮未改，双方各 3 次聚焦均通过。首次超时根因未复现，不宣称已修复或完全排除并发时序风险 |
| 完整 integration 覆盖 | 本机无 Docker，repository 整包返回跳过；另有 Redis、外部服务等显式 skip。已补本轮新增密钥 SQL 和关键 PG 专项，但不宣称全套集成/上线验收通过 |

原始 main 由 `git archive HEAD backend` 导出至忽略缓存目录 `backend/.gocache/merge219-main-baseline`，不创建分支或 worktree，不修改原始源码来跑对照。基线验证与合并结果使用同一 Go 1.27.3 工具链；两个 PG 数据库相互隔离。

## 最终审核状态与证据

- `HEAD=main=eb34cc973ef99e9e20b9c6f8e7f4936a0551cd11`；`MERGE_HEAD=a9240d5016c226b341242a8c54a01a8605cbbc72`。本轮合并未 commit、未 push、未合回 main、未部署，全部交付改动暂存供审核。
- 34 个 features 文件逐 blob 保留；仅上游生产路径逐 blob 等于固定目标；旧 migration 零修改。合并台账保留原文件完整字节前缀，只追加本轮。
- `llm-wiki/wiki/` 六页、受影响组件 README、wiki 图谱、本报告及合并台账已更新。没有独立生产 bug 修补；本次完成合并与测试，不代表全绿或上线批准。
- 本机证据均在忽略目录 `backend/.gocache/merge219-*`：`audit.json` / `overlap-delta.json` / `final-audit.json`、`test-summary.json`、各命令 `.log` / `.result.json`、PG 验证与原始 main 对照。它们不入库，正式结论保存在本报告。
