#!/bin/bash
# macOS: uses only system Bash and JavaScript for Automation (Foundation).
set +x
set -euo pipefail
umask 077

if [ "$(uname -s)" != Darwin ]; then
  printf '%s\n' '此脚本用于 macOS；Windows 请运行 setup-claude.ps1。' >&2
  exit 1
fi
if [ ! -x /usr/bin/osascript ]; then
  printf '%s\n' '找不到 macOS 系统组件 /usr/bin/osascript。' >&2
  exit 1
fi
printf '%s\n' '=== Claude Code for VS Code 一键配置 ===' \
  '请先完全退出 VS Code。已有配置会自动备份。' \
  '将按指定模板配置跳过权限确认模式（bypassPermissions）及相关设置。' \
  '' '请先在中转站的“API 密钥”页面复制完整 Key。' \
  '在下方按 Command+V 粘贴；粘贴的内容会显示，确认后按回车。'
printf '%s' '请粘贴 API 密钥（输入可见）：'
if ! IFS= read -r claude_setup_key; then
  printf '\n%s\n' '未读取到输入，配置已取消，请重新运行脚本。' >&2
  exit 1
fi
printf '\n'
if [ -z "$claude_setup_key" ]; then
  printf '%s\n' 'API 密钥不能为空，请重新运行脚本并粘贴完整 Key。未修改任何配置。' >&2
  exit 1
fi

# The key is passed only to this child process, never in argv or shell source.
CLAUDE_SETUP_KEY="$claude_setup_key" /usr/bin/osascript -l JavaScript <<'CLAUDE_SETUP_JXA'
ObjC.import('Foundation');

function isObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function mergeSettings(target, patch) {
  Object.keys(patch).forEach(function (key) {
    if (isObject(patch[key])) {
      if (!Object.prototype.hasOwnProperty.call(target, key)) target[key] = {};
      if (!isObject(target[key])) throw new Error('配置字段必须是 JSON 对象：' + key);
      mergeSettings(target[key], patch[key]);
    } else {
      target[key] = patch[key];
    }
  });
  return target;
}

function settingsPatch(key) {
  return {
    env: {
      ANTHROPIC_AUTH_TOKEN: key,
      ANTHROPIC_BASE_URL: 'http://118.25.112.30:52997',
      CLAUDE_CODE_ATTRIBUTION_HEADER: '0',
      CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
      DISABLE_ERROR_REPORTING: '1',
      DISABLE_TELEMETRY: '1',
      ENABLE_PROMPT_CACHING_1H: '1'
    },
    permissions: { defaultMode: 'bypassPermissions' },
    disableAgentView: true,
    enabledPlugins: {},
    outputStyle: 'Concise',
    effortLevel: 'xhigh',
    skipDangerousModePermissionPrompt: true,
    theme: 'auto',
    crossSessionInbound: 'accept',
    modelSettings: { 'claude-opus-5-5': { effortLevel: 'high' } }
  };
}

function shellQuote(value) {
  return "'" + value.replace(/'/g, "'\\''") + "'";
}

function updateProfile(source, environment) {
  var begin = '# >>> xunyou-claude >>>';
  var end = '# <<< xunyou-claude <<<';
  var inside = false;
  var lines = source.replace(/\r\n/g, '\n').split('\n');
  var kept = [];
  lines.forEach(function (line) {
    if (line === begin) {
      if (inside) throw new Error('配置块标记不完整，请先修复终端配置。');
      inside = true;
    } else if (line === end) {
      if (!inside) throw new Error('配置块标记不完整，请先修复终端配置。');
      inside = false;
    } else if (!inside) kept.push(line);
  });
  if (inside) throw new Error('配置块标记不完整，请先修复终端配置。');
  var original = kept.join('\n').replace(/\n+$/, '');
  var block = Object.keys(environment).map(function (name) {
    return 'export ' + name + '=' + shellQuote(environment[name]);
  }).join('\n');
  return (original ? original + '\n\n' : '') + begin + '\n' + block + '\n' + end + '\n';
}

function run() {
  var fm = $.NSFileManager.defaultManager;
  var processEnv = $.NSProcessInfo.processInfo.environment;
  function env(name) {
    var value = processEnv.objectForKey(name);
    return value.isNil() ? '' : ObjC.unwrap(value);
  }
  function exists(path) { return Boolean(fm.fileExistsAtPath(path)); }
  function directory(path) {
    if (!fm.createDirectoryAtPathWithIntermediateDirectoriesAttributesError(path, true,
      $({ NSFilePosixPermissions: 448 }), null)) throw new Error('无法创建目录：' + path);
  }
  function read(path) {
    if (!exists(path)) return null;
    var attributes = fm.attributesOfItemAtPathError(path, null);
    if (!attributes || ObjC.unwrap(attributes.objectForKey($.NSFileType)) !== 'NSFileTypeRegular') {
      throw new Error('配置目标必须是普通文件（不支持符号链接）：' + path);
    }
    var value = $.NSString.stringWithContentsOfFileEncodingError(path, $.NSUTF8StringEncoding, null);
    if (value.isNil()) throw new Error('无法以 UTF-8 读取文件：' + path);
    return ObjC.unwrap(value);
  }
  function write(path, content) {
    if (!$(content).writeToFileAtomicallyEncodingError(path, true, $.NSUTF8StringEncoding, null)) {
      throw new Error('无法写入文件：' + path);
    }
    if (!fm.setAttributesOfItemAtPathError($({ NSFilePosixPermissions: 384 }), path, null)) {
      throw new Error('无法设置文件权限：' + path);
    }
  }

  var key = env('CLAUDE_SETUP_KEY').trim();
  if (!key || /\s|[\x00-\x1f\x7f]/.test(key)) throw new Error('API 密钥不能为空，也不能包含空格、换行或控制字符，请重新复制完整 Key。');
  var userDirectory = ObjC.unwrap($.NSHomeDirectory());
  var configDirectory = env('CLAUDE_CONFIG_DIR') || userDirectory + '/.claude';
  var zshDirectory = env('ZDOTDIR') || userDirectory;
  if (configDirectory.charAt(0) !== '/' || zshDirectory.charAt(0) !== '/') {
    throw new Error('CLAUDE_CONFIG_DIR 和 ZDOTDIR 必须使用绝对路径。');
  }
  var settingsPath = configDirectory + '/settings.json';
  var originalSettings = read(settingsPath);
  var settings = {};
  if (originalSettings !== null) {
    try { settings = JSON.parse(originalSettings.replace(/^\uFEFF/, '')); }
    catch (_) { throw new Error('settings.json 不是有效 JSON，原文件未修改。'); }
    if (!isObject(settings)) throw new Error('settings.json 必须是 JSON 对象。');
  }
  var patch = settingsPatch(key);
  var plans = [{ path: settingsPath, before: originalSettings,
    after: JSON.stringify(mergeSettings(settings, patch), null, 2) + '\n' }];
  // Bash login shells read only the first existing file in this order.
  var loginNames = ['.bash_profile', '.bash_login', '.profile'];
  var loginPath = userDirectory + '/.bash_profile';
  for (var i = 0; i < loginNames.length; i++) {
    if (exists(userDirectory + '/' + loginNames[i])) {
      loginPath = userDirectory + '/' + loginNames[i];
      break;
    }
  }
  [zshDirectory + '/.zshrc', userDirectory + '/.bashrc', loginPath].forEach(function (path) {
    var before = read(path);
    plans.push({ path: path, before: before, after: updateProfile(before || '', patch.env) });
  });
  // Validate every input before writing any target. Back up all originals first.
  directory(configDirectory);
  directory(zshDirectory);
  var backupDirectory = configDirectory + '/backup-before-xunyou-' +
    new Date().toISOString().replace(/[:.]/g, '-') + '-' + ObjC.unwrap($.NSUUID.UUID.UUIDString);
  directory(backupDirectory);
  plans.forEach(function (plan, index) {
    plan.backup = plan.before === null ? null : backupDirectory + '/' + index + '-' + plan.path.split('/').pop();
    if (plan.backup) {
      if (!fm.copyItemAtPathToPathError(plan.path, plan.backup, null)) throw new Error('备份失败，未写入配置。');
      if (!fm.setAttributesOfItemAtPathError($({ NSFilePosixPermissions: 384 }), plan.backup, null)) {
        throw new Error('无法保护备份，未写入配置。');
      }
    }
  });
  write(backupDirectory + '/manifest.json', JSON.stringify(plans.map(function (plan) {
    return { path: plan.path, backup: plan.backup };
  }), null, 2) + '\n');
  plans.forEach(function (plan) {
    if (read(plan.path) !== plan.before) throw new Error('配置被其他进程修改，请重新运行脚本。');
  });
  var attempted = [];
  try {
    plans.forEach(function (plan) {
      attempted.push(plan);
      write(plan.path, plan.after);
    });
  } catch (_) {
    var restored = true;
    attempted.reverse().forEach(function (plan) {
      try {
        if (plan.backup) {
          // Restore original bytes, including an optional BOM.
          var data = $.NSData.dataWithContentsOfFile(plan.backup);
          if (!data.writeToFileAtomically(plan.path, true)) restored = false;
        } else if (exists(plan.path) && !fm.removeItemAtPathError(plan.path, null)) restored = false;
      } catch (_) { restored = false; }
    });
    throw new Error((restored ? '写入失败，已恢复原文件。' : '写入失败，请从备份恢复。') + '备份：' + backupDirectory);
  }
  return '配置写入完成！\n配置文件：\n' + plans.map(function (plan) { return plan.path; }).join('\n') +
    '\n原配置备份：' + backupDirectory + '\n下一步：重新打开 VS Code，在 Claude Code for VS Code 扩展面板中开始使用。\n本脚本只写入本地配置，尚未发送模型请求。';
}
CLAUDE_SETUP_JXA
unset claude_setup_key
