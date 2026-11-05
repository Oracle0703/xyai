# Claude Code for VS Code 接入页与本地配置脚本

## 接入页与使用范围

- 线上入口：`http://118.25.112.30:52996/claude.html`；源码 `tools/claude-setup/claude.html`。沿用既有 Codex 指南的蓝绿品牌、四步布局与响应式样式，独立静态发布，不是网关 Vue 路由。
- 仅允许本机 VS Code 的 **Claude Code for VS Code** 官方扩展图形面板；页面明确禁止独立 CLI、VS Code 终端会话、Claude 网页/桌面、其他编辑器和第三方客户端使用本中转站 Claude 服务。终端只用于运行配置脚本。
- 页面引导安装 Anthropic 发布的 `anthropic.claude-code`，在 VS Code 设置中关闭 `claudeCode.useTerminal`、开启 `claudeCode.disableLoginPrompt`；按用户提供截图增加注意项：搜索 `bypass`，手动勾选 **Claude Code: Allow Dangerously Skip Permissions**（`claudeCode.allowDangerouslySkipPermissions`）。该扩展开关允许使用跳过权限确认模式，配置脚本不代改 VS Code 设置。配置后通过 `Open in New Tab` / `Open in Side Bar` 进入扩展。官方依据：`https://code.claude.com/docs/en/vs-code`，2026-11-05 核对；VS Code 至少 1.94.0，扩展自带运行组件。
- 这是使用规范与引导范围，**不是服务端强制客户端鉴权**；共享 `.claude/settings.json` 和 shell 环境变量不能阻止其他客户端读取 Key。本次没有添加网关客户端限制，也不宣称已实现技术封锁。
- 页面只提供 Windows/macOS 切换、官方下载链接、同目录配置脚本下载、命令/示例消息复制、备份和 FAQ；不接收 Key、不请求业务或模型 API。HTTP 部署下用 `execCommand('copy')` 兜底，失败显示可手动复制文本。
- 发布目录 `/opt/xunyou/xunyou-codex-guide`，同目录放 `claude.html`、`setup-claude.ps1`、`setup-claude.sh` 及页面引用的 `assets/` 图片。bypass 示意图是用户截图的设置项裁剪，带内容哈希文件名以适配线上静态资源长缓存；换图时换文件名。现有 Nginx 52996 静态规则直接提供访问；保留原 Codex 首页及脚本，不需重启 Nginx/网关。发布原始证据在仓库外，维护时先核对线上文件再替换。

## 用法

先完全退出 VS Code，在脚本所在目录打开系统终端，执行对应命令，再粘贴自己的 API Key 并按回车。两套脚本使用中文提示，Key 输入直接可见，用户可确认粘贴完整；无需管理员权限，脚本不安装客户端、不发送模型请求。

| 系统 | 脚本 | 运行命令 |
| --- | --- | --- |
| Windows | `tools/claude-setup/setup-claude.ps1` | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\setup-claude.ps1` |
| macOS | `tools/claude-setup/setup-claude.sh` | `bash ./setup-claude.sh` |

完成后重新打开 VS Code，在 Claude Code for VS Code 扩展图形面板中使用。脚本会显示实际写入路径和备份位置。无需修改脚本内的用户名或盘符，用户只需输入 Key。

## 写入位置与配置合同

| 系统 | 位置 | 行为 |
| --- | --- | --- |
| Windows | 当前用户主目录下的 `.claude/settings.json`，通常是 `%USERPROFILE%\.claude\settings.json` | 用系统 UserProfile API 定位，不写死 `C:\Users\Admin`；兼容 Windows PowerShell 5.1 / PowerShell 7 |
| macOS | `~/.claude/settings.json` | 同时保存完整 JSON 设置；依赖系统自带 Bash、osascript 和 Foundation，无需 Python / Node |
| macOS Zsh | `~/.zshrc`；已设置 `ZDOTDIR` 时用该目录的 `.zshrc` | 写入下表七个环境变量 |
| macOS Bash | `~/.bashrc` 和登录配置 | 按 `.bash_profile`、`.bash_login`、`.profile` 选择首个已存在的登录配置；均不存在时新建 `.bash_profile`，覆盖 macOS 常见登录终端 |
| 自定义目录 | `CLAUDE_CONFIG_DIR/settings.json` | 两套脚本都尊重已有变量；macOS 自定义目录须为绝对路径。Windows 还支持 `-ConfigDir` 参数，用于隔离测试 |

`settings.json` 的 `env` 和 macOS 终端配置使用相同值：

| 字段 | 值 |
| --- | --- |
| `ANTHROPIC_AUTH_TOKEN` | 本次交互输入的 Key，去除两端空白；拒绝空值、内部空白及控制字符 |
| `ANTHROPIC_BASE_URL` | `http://118.25.112.30:52997` |
| `CLAUDE_CODE_ATTRIBUTION_HEADER` | `0` |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | `1` |
| `DISABLE_ERROR_REPORTING` | `1` |
| `DISABLE_TELEMETRY` | `1` |
| `ENABLE_PROMPT_CACHING_1H` | `1` |

其余 JSON 按用户指定模板写入：`permissions.defaultMode=bypassPermissions`、`disableAgentView=true`、`outputStyle=Concise`、`effortLevel=xhigh`、`skipDangerousModePermissionPrompt=true`、`theme=auto`、`crossSessionInbound=accept`，以及 `modelSettings.claude-opus-5-5.effortLevel=high`。`enabledPlugins` 初次生成为空对象；更新时保留已有插件。

这些是 Claude Code 配置，不是 Claude 消费者桌面聊天应用的代理配置。脚本只保证生成指定内容；客户端版本是否识别全部字段、实际 Key 和模型权限需在客户端发送消息验证。

## 保留、备份与恢复

- JSON 对象递归合并，只覆盖模板列出的值；保留其他环境变量、插件、权限 allow/deny、其他模型和自定义字段。不强制指定当前模型。
- 既有 `settings.json` 若不是合法 JSON 对象，或需要合并的字段不是对象，立即退出并保留原文件。目标文件若为目录或符号链接也拒绝处理，避免破坏链接管理的配置。
- 写入前在 Claude 配置目录创建唯一 `backup-before-xunyou-时间-随机值/`，备份保留原始字节。备份可能含旧 Key，不应公开分享。
- Windows 将新配置写入同目录临时文件，再原子替换；替换前核对原文件是否被其他进程改变，失败清理临时文件。首次创建若目标已被其他进程创建则失败，不覆盖。
- `setup-claude.ps1` 使用 UTF-8 **带 BOM** 编码，保证 Windows PowerShell 5.1 正确读取中文提示；生成的 `settings.json` 仍是 UTF-8 **无 BOM**。Windows 用普通 `Read-Host`，macOS 用 `read -r`，不再隐藏输入；成功信息只显示文件路径，不重复打印 Key。
- macOS 先校验全部配置并备份，再逐文件原子写入；普通写入失败尝试恢复已触及的文件，失败信息指出备份位置。新目录权限 `0700`，写入文件及备份 `0600`；不宣称抵抗进程被强制杀死或断电的跨文件事务。
- macOS 使用 `# >>> xunyou-claude >>>` / `# <<< xunyou-claude <<<` 管理块，每次删除旧块后在末尾写入一次，其他内容保留。异常标记拒绝写入；Key 经过 shell 单引号转义，不能作为命令执行。不会自动 source 用户启动文件。
- Windows 恢复：退出 Claude Code，将备份中的 `settings.json` 复制回原目录。macOS 恢复：退出终端/Claude Code，根据备份目录的 `manifest.json` 将 `backup` 文件复制回对应 `path`；`backup=null` 表示脚本运行前不存在该文件，可删除该次新建文件。恢复后重新打开终端及客户端。

## 代码与验证

| 入口 | 职责 |
| --- | --- |
| `tools/claude-setup/claude.html` / `selectOS`、`copyText` | 四步接入页，系统切换、脚本下载、HTTP 兼容复制；脚本文件须与页面同目录分发，保留 `assets/` 中页面引用的设置示意图 |
| `tools/claude-setup/setup-claude.ps1` / `Merge-ClaudeSettings` | Windows 交互、JSON 合并、备份、原子替换 |
| `tools/claude-setup/setup-claude.sh` / `settingsPatch`、`mergeSettings`、`updateProfile`、`run` | macOS 模板、合并、终端块更新、Foundation 文件操作与恢复 |
| `tools/claude-setup/test-setup.cjs` | 在系统临时目录测试 Windows 真正执行；用文件系统桥接替身测试 macOS 逻辑，不访问真实用户配置 |

仓库根目录运行 `node tools/claude-setup/test-setup.cjs`。可设置 `CLAUDE_TEST_POWERSHELL` 指定 PowerShell 可执行文件；设置 `CLAUDE_TEST_BASH` 指定 Bash 后额外检查 shell 语法和真实 source 的特殊字符往返。Node 仅为开发测试依赖，最终用户运行脚本不需要。

2026-11-05 脚本验证：Windows PowerShell 5.1 与 PowerShell 7.6.5 实际执行均通过；macOS 逻辑替身与 Git Bash 校验通过，每个 PowerShell 版本运行 11 项检查。覆盖中文提示正确读取、真实 `Read-Host` 的假 Key 可见回显与保存、首次写入、模板一致、换 Key、递归保留、原字节备份、非法 JSON/结构/Key、并发修改、重复运行、登录文件优先级、自定义目录、写入失败恢复、shell 转义。尚未在 macOS 实机执行 Foundation / 终端验收，也未调用真实模型。

2026-11-05 页面已发布至上述入口：三个公开 URL 均 HTTP 200，文件与本地产物逐字节一致，原 Codex 首页和脚本哈希保持不变；未重启服务。应用内浏览器检查 Windows/macOS 切换、两套脚本下载字节一致、1280px/390px 无横向溢出、控制台无错误；线上 HTTP 页面两种命令均经系统剪贴板比对通过。仅页面/静态发布验收，不代替真实 Key 与 VS Code 扩展接入验收。
