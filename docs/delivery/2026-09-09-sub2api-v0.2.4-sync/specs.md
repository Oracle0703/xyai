# 设计规格

## 摘要

| 字段 | 内容 |
|---|---|
| 需求来源 | `requirements.md` |
| 交付目标 | 形成可审核、未提交的 Sub2API `0.2.4` merge 快照。 |
| 主要验收标准 | 精确提交边界、冲突清零、本地功能保留、文档完整、验证证据真实。 |

## 当前状态

| 区域 | 当前行为 / 证据 |
|---|---|
| 分支基线 | 本地 `main@f6bce5db`，与刷新后的 `github/main` 一致，工作区预检时干净。 |
| 上游边界 | `upstream/main@98d86915b`，`VERSION=0.2.4`，merge base 为 `270eac697`。 |
| 上游增量 | 3 commits、19 paths、`+336/-10`，聚焦 GPT Image 2.5 与 OAuth 生图主控模型。 |
| 合并状态 | `MERGE_HEAD=98d86915becae9fe9491a91ffc6defd5235c8d2b`；无文本冲突。 |
| 三方审查 | `README_CN.md` 与 `backend/internal/service/openai_codex_transform.go` 保留双方独有增量；17 个仅上游路径与 exact target blob 一致。 |
| 本地功能 | `docs/features/` 的 24 个 tracked 文件全部保留、零删除。 |

## 目标行为

| ID | 行为 | 用户 / 系统影响 |
|---|---|---|
| TB-1 | 合入 GPT Image 2.5 Flare/Sunburst、日期快照识别、内置价格与模型白名单候选。 | 固定获得上游 `0.2.4` 能力。 |
| TB-2 | OAuth/Setup Token 生图的 Responses 主控模型默认改为 `gpt-5.6-luna`，并支持 `SUB2API_IMAGES_MAIN_MODEL` 运维覆盖。 | 图片模型与文本主控模型保持独立。 |
| TB-3 | 本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk、Token Analysis、组织用量、子管理员、并发 preset 等独有能力继续存在。 | 避免上游同步回退本地功能。 |
| TB-4 | 两个双方修改路径按三方语义合并；真正重叠代码采用上游实现，本地独有代码保留。 | 自动合并不产生隐性功能丢失。 |
| TB-5 | 上游缺陷原样保留并记录，不为测试绿灯改写上游实现。 | 保持上游可追溯性。 |

## 接口与数据契约

| 接口 / 字段 | 目标合同 | 兼容性说明 |
|---|---|---|
| `SUB2API_IMAGES_MAIN_MODEL` | 可选、trim 后非空时覆盖 OAuth/Setup Token 生图的 Responses 文本主控模型。 | 不替换客户端选择的图片模型；Compose 变体同步透传。 |
| GPT Image 2.5 模型 | 支持 `gpt-image-2.5-flare`、`gpt-image-2.5-sunburst` 及 `-2026-09-08` 快照。 | 显式账号映射和分组白名单不自动扩大权限。 |
| 生图请求 | `reqBody.model` 使用主控模型，`tools[image_generation].model` 保留图片模型。 | 已提供合法文本主控模型的 Responses 请求不应被覆盖。 |
| 生图 usage | 保留文本输入、图片输入和图片输出 token 明细。 | GPT Image 2.5 内置价格仅在显式价格缺失时回落。 |

## 数据流

| 步骤 | 输入 | 处理 | 输出 |
|---|---|---|---|
| 1 | `main@f6bce5db`、上游 `98d86915b` | `git merge --no-commit --no-ff` | 暂存 merge 树与固定 `MERGE_HEAD`。 |
| 2 | 双方修改和可能冲突 | base/local/upstream 三方语义审查 | 本地独有能力与上游 0.2.4 增量并存。 |
| 3 | 合并树 | focused/full tests、lint、build、静态审计 | 可审核验证证据。 |
| 4 | 已验证事实 | 更新 wiki、图谱与 append-only ledger | 稳定知识和合并审计记录。 |

## 失败处理

| 场景 | 预期处理 | 需要的证据 |
|---|---|---|
| 未合并索引或冲突标记残留 | 不进入用户审核点。 | `git ls-files -u`、标记扫描。 |
| 自动合并丢失本地功能 | 仅按三方语义恢复本地独有部分。 | 双方修改路径和 feature 清单审查。 |
| 测试暴露上游 bug | 记录并对照固定上游或第一父，不修改生产逻辑。 | `test-review.md` 归因。 |
| 生成物漂移 | 仅当 schema/Wire source 变化时重新生成并复核。 | 变更路径审计和二次生成。 |
| merge commit 未生成 | ledger 使用明确的审核状态描述。 | `MERGE_HEAD` 与 Git 状态。 |

## 验收标准映射

| 成功标准 | 规格覆盖 | 验证方式 |
|---|---|---|
| SC-1 / SC-2 | 精确基线和 merge 数据流 | `rev-parse`、`merge-base`、index/marker 检查。 |
| SC-3 / SC-4 | TB-3 / TB-4 / TB-5 | 三方 diff、features 清单、额外编辑审计。 |
| SC-5 | 文档数据流 | wiki diff、ledger 尾部、图谱状态。 |
| SC-6 | 失败处理 | 专项与完整测试矩阵。 |
| SC-7 | 审核门禁 | `MERGE_HEAD` 保留且无 commit/push/PR。 |
