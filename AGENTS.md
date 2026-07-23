# 项目 llm-wiki 协作规则

本文件是 Codex 在本仓库工作的项目级规则。进入任务后, 必须先按本文读取 `llm-wiki`, 任务完成前按本文判断是否需要更新 `llm-wiki`。用户不需要额外提示 AI 沉淀知识。

## 工作前读取顺序

1. 先读 `llm-wiki/wiki/README.md`, 确认知识库入口和文档地图。
2. 再按任务类型读取对应文档:
   - 后端, 网关, 路由, 服务, 仓储, 数据库: `llm-wiki/wiki/backend.md`
   - 前端, 页面, 组件, store, API client: `llm-wiki/wiki/frontend.md`
   - 配置, 启动, 构建, CI, 验证命令: `llm-wiki/wiki/ops.md`
   - 数据模型, 迁移, 计费, 支付, 订阅: `llm-wiki/wiki/data-and-domain.md`
   - 安全, 认证, 幂等, 限流, 网关边界: `llm-wiki/wiki/security-and-reliability.md`
   - 日常 AI 协作流程: `llm-wiki/wiki/ai-workflow.md`
3. 如果 wiki 内容与源码冲突, 以源码和现有测试为准, 并在本轮修改中修正 wiki。
4. 只有 wiki 无法回答任务上下文时, 再系统扫描相关源码, 不要每次从全仓库重新开始。

## 工作中维护规则

以下情况必须更新 `llm-wiki/wiki/`:

- 新增或修改跨模块架构, 路由, 数据流, 依赖注入, 后台任务。
- 新增或修改配置项, 环境变量, 启动方式, CI/验证命令。
- 新增或修改数据库 schema, SQL migration, Ent schema, 关键索引。
- 新增或修改认证, 权限, 限流, 幂等, 支付, 计费, 网关转发等高风险逻辑。
- 发现 wiki 过期, 缺失或与源码不一致。

以下情况通常不需要更新 wiki:

- 仅修正文案, 样式, 局部测试断言, 小范围 typo。
- 不改变外部行为的局部实现细节。

## 上游合并记录规则

每次与远程仓库 `Wei-Shaw/sub2api` (`https://github.com/Wei-Shaw/sub2api`) 合并后, 必须把本次合并记录追加写入 `docs/features/sub2api -merage-list.md`。

- 该文件是追加型记录, 只能新增条目, 不可以直接覆盖、清空或删除已有记录。
- 记录至少包含合并日期、工作分支、上游分支、上游提交、合并提交、冲突文件、处理方式和验证结果。
- 如合并过程中没有冲突, 也需要明确写入“无冲突”和对应验证结果。

## 写作约束

- `llm-wiki/wiki/` 是 AI 可读的稳定知识库, 写结论, 路径, 约束和验证方式, 不写流水账。
- `llm-wiki/raw/` 用于放原始调研材料, 粘贴片段或临时事实来源; 默认不作为最终知识入口。
- wiki 内容必须简洁, 可被后续 AI 在开发前快速读取。
- 更新 wiki 时保持 LF 换行。
- 不把密钥, token, 本机私密凭据写入 wiki。
- 记录文件路径时使用仓库相对路径。

## 任务收尾检查

完成代码或文档任务前, Codex 必须检查:

- 本轮是否改变了需要沉淀的项目知识。
- 若需要, 已更新对应 `llm-wiki/wiki/*.md`。
- 已执行与修改类型匹配的验证命令; 若未执行, 说明原因。

## 参考实践

本仓库采用 llm-wiki 的分层实践:

- `raw/`: 原始资料和事实来源。
- `wiki/`: 面向 AI 开发前快速读取的整理版知识。
- 项目级规则文件: 约束 AI 自动读取和更新 wiki, 避免每次重复扫描全项目。

## 知识图谱（Understand Anything）

llm-wiki 是权威知识正文；知识图谱是导航层，不能替代 wiki。

### 本机产物

- 代码图谱: `.understand-anything/knowledge-graph.json`（体积大，默认 gitignore，本机生成）
- Wiki 图谱: `llm-wiki/.understand-anything/knowledge-graph.json`（体积小，可入库共享）
- 共享配置: `.understand-anything/config.json`, `.understand-anything/.understandignore`

### AI 何时使用图谱

1. 仍先读 `llm-wiki/wiki/README.md` 与对应 wiki 页（硬性）。
2. 需要定位模块影响面、路由/服务关系、本地能力边界时，可检索本机图谱节点（file/function/concept/endpoint）。
3. 看 diff 影响可用 `/understand-diff`（需已有代码图谱）。
4. 图谱与源码冲突时以源码和测试为准；与 wiki 冲突时先修 wiki。

### 维护命令

Windows 下请用 **`.cmd` 入口**（内部 `ExecutionPolicy Bypass`，避免 RemoteSigned 拦截）:

- 状态检查: `tools\check-understand-status.cmd`
- 启动可视化: `tools\start-understand-dashboard.cmd`
- 刷新 wiki 图谱: `tools\refresh-understand-wiki.cmd`

等价写法: `powershell -NoProfile -ExecutionPolicy Bypass -File tools\check-understand-status.ps1`（不要只用 `powershell -File`，在 RemoteSigned 下可能被拒绝）。

- 重建代码图谱: 运行 `/understand`（遵守 `.understand-anything/.understandignore` 范围）
- 重建 wiki 图谱: 运行 `/understand-knowledge llm-wiki` 或 `tools\refresh-understand-wiki.cmd`

重大架构/网关变更或上游合并后，建议刷新 wiki 图谱；代码图谱按需增量更新，不必每次提交重建。

`check-understand-status` 会校验: 图谱 JSON 完整性、wiki/code 基线相对 HEAD 是否过期（`meta..HEAD` 是否改动了对应目录）、`llm-wiki` 是否 dirty。过期或 dirty 时返回 PARTIAL，不会误报 READY。
