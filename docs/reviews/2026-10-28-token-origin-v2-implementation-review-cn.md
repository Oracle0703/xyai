# Token Analysis 请求来源 v2 审核结论

2026-10-28，`feature/hy/10222_token_origin_v2`，基线 `02cf7a07cbebf4c77003f617ec7cb53c6e1150f2` 加未提交修复。

**首轮 3 项 P2 均已修复并独立复验关闭，未发现新的阻塞性编码遗漏。** 依据为 [当前设计合同](../features/token-analysis-request-origin-design-cn.md)；下表合并多轮结果，保留修复前后证据。

## 问题与最终处理

| 问题 | 修复前 | 最终处理与复验 | 状态 |
| --- | --- | --- | --- |
| F1 概览遗漏邮箱筛选 | 不匹配邮箱：排行为空，概览仍计 1 次人工输入 | 主查询、风险原因、人工次数及同期计费分母共用邮箱条件；原用例归零，正式测试覆盖部分命中/不命中 | 已关闭 |
| F2 项目名称碰撞 | 空项目与真实 unattributed 项目按展示名关联，2 次输入计为 4 次 | 按原始 client_project 关联，两行各计 1、合计 2；展示同名仍属既有合同 | 已关闭 |
| F3 工具图片前驱遗漏 | tool 与带固定说明的图片间插入纯上下文 user 消息，被判人工 | 跳过完整纯注入消息，原用例判 ai/tool_result/high；真实正文和机器通知仍阻断 | 已关闭 |

代码入口：F1/F2 为 `backend/internal/repository/token_analysis_repo.go`，F3 为 `backend/internal/service/token_analysis_origin.go`；正式回归见 `token_analysis_human_input_integration_test.go`、`token_analysis_origin_test.go`。

## 验证与证据

| 检查 | 修复后结果 |
| --- | --- |
| 原 F1/F2/F3 复现 | 用例逐字节不变，经 Go overlay 全部通过 |
| Go service/repository/handler/admin 专项 | 140 个顶层测试通过，0 失败/跳过 |
| 独立 PostgreSQL 18.3 空库 | 60 个顶层测试通过（58 正式＋原 F1/F2），0 失败/跳过 |
| 前端 View/API/locale 完整性/文案编译 | 4 文件、50 项通过 |
| 格式与收尾 | gofmt、locale ESLint、git diff --check 通过；隔离数据库已停止，无遗留 overlay 测试 |

- [首轮证据](../delivery/2026-10-28-token-origin-v2-review/README.md)：原失败日志、合成用例和当时源文件哈希。
- [修复后证据](../delivery/2026-10-28-token-origin-v2-review/rereview/README.md)：通过日志、验证摘要及复审源文件哈希。

已核对 v2 分类、v1/v2 兼容、当前筛选集合归并、NULL Key/无指纹逐条计数及请求/Token/费用维持原口径。无状态 compaction_resume/medium 是已接受的设计决定，不列为遗漏。旧 v1/EXISTS 描述与预览边界已同步。

## 剩余边界

hertz 显式来源、生产部署、原文保留、受控历史重索引、新窗口归并验收、真实数据性能及浏览器验收仍待完成，见设计“历史回填与待验收”。人工次数是估算，来源标签不证明物理操作。

本次审核仅使用本机合成数据，未修改业务代码或生产、未提交；未独立重跑整仓构建或百万行基准，也未测量缺陷的生产发生比例。
