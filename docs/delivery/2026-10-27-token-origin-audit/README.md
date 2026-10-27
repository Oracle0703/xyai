# 2026-10-27 生产来源判断审计材料

结论与方法见 `docs/reviews/2026-10-27-token-analysis-production-origin-audit-cn.md`。

生产版本 `25bedc620d9783e065ec6efae45d56f984b6b01c`，北京时间窗口 `[2026-10-26 09:00, 2026-10-27 09:00)`。60,065 条已归档已索引请求中，人工标签 3,591 条已全量提取结构；AI 标签 56,387 条中分层抽查 470 条。

识别到机器用途却标人工的 478 条：显式元数据 87 条，高置信客户端结构 391 条。478/3,591=13.31%，478/60,065=0.796%；均为协议语义下的保守检出比例，不能当作全站真实误判率。AI 样本未发现可确认的新人工输入误判，不代表全量零误判。

| 文件 | 内容 |
| --- | --- |
| `audit-summary.json` | 固定窗口、分母、计数和边界 |
| `identified-misclassifications.json` | 478 条互斥判定，含证据层、类别、通用理由和定位 |
| `sample-locations.jsonl` | 4,061 条唯一核查记录的 ID、原标签、端点、元数据枚举、行位置和 SHA；不含正文 |
| `shape-check-facts.jsonl` | 135 条原文补查得到的结构布尔事实；末行是读取统计 |
| `skill-chain-check-facts.jsonl` | 2 条连续技能注入链的完整结构核验；末行是读取统计 |
| `ai-review-result.json` | 470 条 AI 样本的独立末有效事件复核 |
| `database-aggregate-evidence.json` | 实际只读 SQL、原输出、耗时、ROLLBACK/只读确认及最终服务状态 |
| `SHA256SUMS` | 本目录文件校验值（不含自身） |

定位生产原文：归档目录为 `/opt/sub2api/backend/data/request-archive/`。`source_file` 为日期文件，`source_offset` 是行末偏移。须向前找行首，核验 `event=request`、`archive_id` 与原始整行字节（含尾部 LF）的 `line_sha256`。禁止把该 offset 当行首读取下一条。生产归档有保留期，文件可能随后被清理，包内定位不保证永久可回读。

样本按标签×端点内 `md5('origin-audit-20261027:' || archive_id)` 固定排序；AI 主端点前 150 条，compact 全取。人工首次每端点最多 400，随后补齐全量。不同端点抽样比例不同，不能用首次混合样本的原始比例代替总体比例。

实际归档读取累计 8,872,057,311 bytes，包括重复确认；首次 3 条超过 24 MiB 单行上限，第二轮提高至 64 MiB 后全部读到，最终无提取错误。135 条补查排除了长节选中段不可见导致的误判风险；另 2 条确认连续技能封装之间也无新人工正文。所有补查行哈希与首次成功读取一致。

本包只保留结构事实和无业务正文的判定。未复制原始对话、工具结果、授权头或用户/API Key 标识。全部本机工作材料在 `E:/tmp/origin-audit/2026-10-27/`，其中节选仅做有限脱敏，不应直接共享该目录。原始读取脚本为该目录中的 `extract_samples_run.py`、`extract_human_census_run.py`、`extract_shape_check_run.py` 和 `extract_skill_chain_check.py`；数据库脚本为 `preflight.py`、`db_audit.py`、`db_followup.py`、`db_final.py`。本轮没有改动生产或执行重索引。

复核可先检查 JSON 计数与 ID 唯一性，再按 ID 对照 `sample-locations.jsonl` 的原人工标签和 SHA。高置信形态需要保留“完整闭合、无外部正文、专用系统用途、前序有效事件”等条件；不要将这些证据简化成关键词规则。
