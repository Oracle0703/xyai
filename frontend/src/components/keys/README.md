# Key Components

本目录维护 API Key 的创建、编辑和使用指引。`UseKeyModal.vue` 根据分组平台生成客户端配置，配置内容必须使用当前网关地址和当前 API Key，不得写入固定凭据。

## `UseKeyModal.vue`

| 平台 | 默认客户端页签 | 生成内容 |
|---|---|---|
| Anthropic | Claude Code | macOS/Linux、CMD、PowerShell 环境变量与 OpenCode |
| OpenAI | Codex | Codex、Codex WebSocket、可选 Claude Code、OpenCode |
| Gemini | Gemini CLI | Gemini CLI 与 OpenCode |
| Antigravity | Claude Code | Claude/Gemini 两种配置与 OpenCode |
| Grok | Grok CLI | `~/.grok/config.toml` / `%userprofile%\.grok\config.toml` 与 OpenCode |

## Grok 配置契约

- Grok 页签使用网关 API base URL，不直写上游 xAI 地址。
- `generateGrokFiles` 生成 `grok` provider/model key，默认模型 `grok-4.5`、`api_backend = "responses"`，并启用 backend search。
- Windows 路径使用 `%userprofile%\.grok`，macOS/Linux 使用 `~/.grok`。
- Grok OpenCode 配置使用 `@ai-sdk/openai`，模型清单包含 `grok-4.5`、`grok-4.3`、`grok-build-0.1` 和 `grok-composer-2.5-fast`。
- 切换 platform 时必须同时重置 client tab 和 OS tab，避免复用上一平台的配置模板。
- 展示代码经过 HTML escape 后再添加高亮，API Key 和 URL 不能以未转义 HTML 注入。

## Codex 认证模式

- 普通 Codex 模板保留本地 `model_provider = "xunyou"` / `[model_providers.xunyou]`; WebSocket v2 模板使用 `OpenAI` provider，并设置 `supports_websockets = true`。
- Legacy Login 输出 `requires_openai_auth = true`。API Key Mode 输出 `requires_openai_auth = false` 和 `x-openai-actor-authorization = "local-image-extension"`; 两种模式都生成 `auth.json`。
- provider 选择器测试必须分别按普通模板的 `xunyou` 和 WebSocket 模板的 `supports_websockets = true` 定位，不能假设两个模板使用同一 provider 名。

## Claude/OpenCode 模型清单

- `generateOpenCodeConfig` 的 Claude 清单包含 `claude-fable-5-1` / `Claude Fable 5.1`，context 为 1,048,576，output 为 128,000，支持 text/image/pdf 输入和 adaptive thinking。
- 新增或重命名 Claude 模型时，要同步后端 `pkg/claude.DefaultModels`、白名单/preset、账号状态短标签与 `UseKeyModal.spec.ts`，避免配置生成器与模型列表分叉。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts
cmd.exe /c pnpm --dir frontend run typecheck
```

修改 provider 名、路径或模型清单时，必须同步 `UseKeyModal.spec.ts` 和对应 i18n key。
