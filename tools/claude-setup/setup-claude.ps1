# 兼容 Windows PowerShell 5.1 和 PowerShell 7；请保留此脚本的 UTF-8 BOM。
# 仅在本机写入配置。API 密钥通过可见的交互输入读取，不作为命令参数。
[CmdletBinding()]
param([string]$ConfigDir = $env:CLAUDE_CONFIG_DIR)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0
$temporaryPath = $null
$apiKey = $null

function Merge-ClaudeSettings($Target, $Patch) {
    foreach ($property in $Patch.PSObject.Properties) {
        $existing = $Target.PSObject.Properties[$property.Name]
        if ($property.Value -is [pscustomobject]) {
            if ($null -eq $existing) {
                $Target | Add-Member -MemberType NoteProperty -Name $property.Name -Value ([pscustomobject]@{})
            } elseif ($existing.Value -isnot [pscustomobject]) {
                throw "settings.json 中的 $($property.Name) 必须是 JSON 对象，原文件未修改。"
            }
            Merge-ClaudeSettings $Target.($property.Name) $property.Value
        } else {
            $Target | Add-Member -MemberType NoteProperty -Name $property.Name -Value $property.Value -Force
        }
    }
}

try {
    if ([string]::IsNullOrWhiteSpace($ConfigDir)) {
        $profileDirectory = [Environment]::GetFolderPath('UserProfile')
        if ([string]::IsNullOrWhiteSpace($profileDirectory)) { throw '无法找到当前用户目录。' }
        $ConfigDir = Join-Path $profileDirectory '.claude'
    }
    $ConfigDir = [IO.Path]::GetFullPath($ConfigDir)
    $settingsPath = Join-Path $ConfigDir 'settings.json'
    Write-Host '=== Claude Code for VS Code 一键配置 ==='
    Write-Host '请先完全退出 VS Code。已有配置会自动备份。'
    Write-Host "配置保存位置：$settingsPath"
    Write-Host '将按指定模板配置跳过权限确认模式（bypassPermissions）及相关设置。'
    Write-Host ''

    $settings = [pscustomobject]@{}
    $originalBytes = $null
    if (Test-Path -LiteralPath $settingsPath) {
        $item = Get-Item -LiteralPath $settingsPath -Force
        if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw 'settings.json 必须是普通文件，不能是文件夹或符号链接。'
        }
        $originalBytes = [IO.File]::ReadAllBytes($settingsPath)
        try {
            $originalText = [IO.File]::ReadAllText($settingsPath)
            if (-not $originalText.TrimStart().StartsWith('{')) { throw '配置必须是 JSON 对象。' }
            $settings = $originalText | ConvertFrom-Json -ErrorAction Stop
        } catch { throw 'settings.json 格式不正确，原文件未修改。' }
        if ($settings -isnot [pscustomobject]) { throw 'settings.json 必须包含一个 JSON 对象。' }
    }

    Write-Host '请先在中转站的“API 密钥”页面复制完整 Key。'
    Write-Host '在下方右键粘贴或按 Ctrl+V；粘贴的内容会显示，确认后按回车。'
    $apiKey = (Read-Host '请粘贴 API 密钥（输入可见）').Trim()
    if ([string]::IsNullOrWhiteSpace($apiKey) -or $apiKey -match '\s|[\x00-\x1f\x7f]') {
        throw 'API 密钥不能为空，也不能包含空格、换行或控制字符，请重新复制完整 Key。'
    }

    $patch = @'
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "",
    "ANTHROPIC_BASE_URL": "http://118.25.112.30:52997",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "DISABLE_ERROR_REPORTING": "1",
    "DISABLE_TELEMETRY": "1",
    "ENABLE_PROMPT_CACHING_1H": "1"
  },
  "permissions": { "defaultMode": "bypassPermissions" },
  "disableAgentView": true,
  "enabledPlugins": {},
  "outputStyle": "Concise",
  "effortLevel": "xhigh",
  "skipDangerousModePermissionPrompt": true,
  "theme": "auto",
  "crossSessionInbound": "accept",
  "modelSettings": { "claude-opus-5-5": { "effortLevel": "high" } }
}
'@ | ConvertFrom-Json
    $patch.env.ANTHROPIC_AUTH_TOKEN = $apiKey
    Merge-ClaudeSettings $settings $patch
    $serialized = ($settings | ConvertTo-Json -Depth 100) + [Environment]::NewLine

    [IO.Directory]::CreateDirectory($ConfigDir) | Out-Null
    $backupDirectory = Join-Path $ConfigDir ('backup-before-xunyou-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
    if ($null -ne $originalBytes) {
        [IO.Directory]::CreateDirectory($backupDirectory) | Out-Null
        [IO.File]::WriteAllBytes((Join-Path $backupDirectory 'settings.json'), $originalBytes)
    }
    $temporaryPath = Join-Path $ConfigDir ('.settings-' + [guid]::NewGuid().ToString('N') + '.tmp')
    [IO.File]::WriteAllText($temporaryPath, $serialized, (New-Object Text.UTF8Encoding($false)))
    if ($null -ne $originalBytes) {
        # 原文件被其他进程修改时不覆盖。
        if ([Convert]::ToBase64String([IO.File]::ReadAllBytes($settingsPath)) -cne [Convert]::ToBase64String($originalBytes)) {
            throw '配置期间 settings.json 被其他程序修改，请关闭相关程序后重新运行脚本。'
        }
        [IO.File]::Replace($temporaryPath, $settingsPath, [NullString]::Value)
    } else {
        [IO.File]::Move($temporaryPath, $settingsPath)
    }
    $temporaryPath = $null
    Write-Host ''
    Write-Host '配置写入完成！' -ForegroundColor Green
    Write-Host "配置文件：$settingsPath"
    if ($null -ne $originalBytes) { Write-Host "原配置备份：$backupDirectory" }
    Write-Host '下一步：重新打开 VS Code，在 Claude Code for VS Code 扩展面板中开始使用。'
    Write-Host '本脚本只写入本地配置，尚未发送模型请求。'
} catch {
    # 不输出解析器的原始错误，避免错误内容再次包含密钥。
    Write-Host '配置失败：请检查配置文件格式、目录写入权限，或关闭其他正在修改配置的程序后重试。' -ForegroundColor Red
    Write-Host ("错误位置：脚本第 {0} 行。" -f $_.InvocationInfo.ScriptLineNumber)
    Write-Host '原配置未被覆盖；如已生成备份，可在 Claude 配置目录中找到。'
    exit 1
} finally {
    if ($temporaryPath -and [IO.File]::Exists($temporaryPath)) { [IO.File]::Delete($temporaryPath) }
    $apiKey = $null
}
