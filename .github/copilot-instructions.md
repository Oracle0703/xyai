# Copilot 项目 llm-wiki 协作规则

在本仓库生成代码或回答项目问题前, Copilot 必须优先查阅 `llm-wiki/wiki/README.md`, 再按任务类型读取相关 wiki 页面。不要要求用户手动提示“请更新 llm-wiki”。

## 必读入口

- 总入口: `llm-wiki/wiki/README.md`
- 后端/网关/服务/仓储: `llm-wiki/wiki/backend.md`
- 前端/路由/store/API/组件: `llm-wiki/wiki/frontend.md`
- 启动/配置/CI/验证: `llm-wiki/wiki/ops.md`
- 数据模型/迁移/支付/订阅: `llm-wiki/wiki/data-and-domain.md`
- 安全/可靠性/限流/幂等: `llm-wiki/wiki/security-and-reliability.md`
- AI 日常协作流程: `llm-wiki/wiki/ai-workflow.md`

## 自动更新规则

当建议或生成的改动影响以下内容时, 同步更新 `llm-wiki/wiki/`:

- 架构, 路由, 依赖注入, 后台任务。
- 配置项, 环境变量, 启动命令, CI 命令。
- 数据库 schema, migration, Ent 生成规则。
- 认证, 权限, 限流, 幂等, 支付, 计费, 网关转发。
- 与 wiki 现有描述冲突的源码事实。

如果 wiki 与源码冲突, 以源码为准, 并把 wiki 修正为新的事实。

## 写作要求

- wiki 写稳定事实和维护约束, 不写临时对话过程。
- 保持简体中文, LF 换行。
- 不记录密钥, token, 私密凭据。
- 不修改业务代码来满足 wiki 更新需求。
