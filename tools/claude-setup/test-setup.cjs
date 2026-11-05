// Isolated filesystem regression tests. Never read the user's Claude settings.
// macOS Foundation is simulated here; a real macOS acceptance run is still required.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const vm = require('node:vm');
const { spawnSync } = require('node:child_process');

const root = fs.mkdtempSync(path.join(os.tmpdir(), 'claude-setup-test-'));
const shell = fs.readFileSync(path.join(__dirname, 'setup-claude.sh'), 'utf8');
const jxa = shell.split("<<'CLAUDE_SETUP_JXA'\n")[1].split('\nCLAUDE_SETUP_JXA')[0];
const key = 'sk-test-"quoted\'$(false)`false`\\value';
const safe = "sk-test-second'$(false)";
const initial = {
  custom: { keep: '中文' }, env: { KEEP_ME: 'yes', ANTHROPIC_AUTH_TOKEN: 'old' },
  permissions: { allow: ['Read'], defaultMode: 'default' },
  enabledPlugins: { 'existing@plugin': true },
  modelSettings: { 'claude-opus-5-5': { other: true }, otherModel: { effortLevel: 'low' } }
};
const wrapper = path.join(root, 'invoke.ps1');
fs.writeFileSync(wrapper, `
function Read-Host {
  param($Prompt, [switch]$AsSecureString)
  if ($AsSecureString) { throw 'The setup prompt must use visible input.' }
  if ($env:CLAUDE_TEST_CONCURRENT) {
    [IO.File]::WriteAllText((Join-Path $env:CLAUDE_CONFIG_DIR 'settings.json'), '{"concurrent":true}')
  }
  Write-Host $Prompt
  $env:CLAUDE_TEST_KEY
}
& $env:CLAUDE_TEST_SCRIPT
if ($LASTEXITCODE) { exit $LASTEXITCODE }
`);

function windows(directory, input, concurrent = false, realPrompt = false) {
  const childEnv = { ...process.env, CLAUDE_CONFIG_DIR: directory,
    CLAUDE_TEST_SCRIPT: path.join(__dirname, 'setup-claude.ps1'),
    CLAUDE_TEST_KEY: input, CLAUDE_TEST_CONCURRENT: concurrent ? '1' : '',
    CLAUDE_TEST_INVOKE: realPrompt ? path.join(__dirname, 'setup-claude.ps1') : wrapper };
  // Let each PowerShell version locate its own modules, not the parent's PS7 modules.
  Object.keys(childEnv).filter(name => name.toLowerCase() === 'psmodulepath').forEach(name => delete childEnv[name]);
  return spawnSync(process.env.CLAUDE_TEST_POWERSHELL || 'powershell.exe',
    ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command',
      '[Console]::OutputEncoding = New-Object Text.UTF8Encoding($false); & $env:CLAUDE_TEST_INVOKE; if ($LASTEXITCODE) { exit $LASTEXITCODE }'], {
      encoding: 'utf8', env: childEnv, input: realPrompt ? input + '\n' : undefined
    });
}
function json(file) { return JSON.parse(fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, '')); }
function preserved(settings, token) {
  assert.equal(settings.env.ANTHROPIC_AUTH_TOKEN, token);
  assert.equal(settings.env.ANTHROPIC_BASE_URL, 'http://118.25.112.30:52997');
  assert.equal(settings.env.KEEP_ME, 'yes');
  assert.equal(settings.custom.keep, '中文');
  assert.deepEqual(settings.permissions.allow, ['Read']);
  assert.equal(settings.permissions.defaultMode, 'bypassPermissions');
  assert.equal(settings.enabledPlugins['existing@plugin'], true);
  assert.equal(settings.modelSettings['claude-opus-5-5'].other, true);
  assert.equal(settings.modelSettings['claude-opus-5-5'].effortLevel, 'high');
  assert.equal(settings.effortLevel, 'xhigh');
}

// JXA bridge test double uses real isolated files, with failure injection.
function mac(directory, input, options = {}) {
  fs.mkdirSync(directory, { recursive: true });
  const nativePath = file => path.toNamespacedPath(file).replace(/\\/g, '/');
  const environment = { CLAUDE_SETUP_KEY: input, ...options.env };
  for (const name of ['CLAUDE_CONFIG_DIR', 'ZDOTDIR']) {
    if (environment[name]) environment[name] = nativePath(environment[name]);
  }
  let failed = false;
  function wrapped(value) {
    return { raw: value, isNil: () => value === null, objectForKey: name => wrapped(value[name] ?? null),
      writeToFileAtomicallyEncodingError: (file) => {
        if (options.failPath && nativePath(options.failPath) === file && !failed) { failed = true; return false; }
        fs.writeFileSync(file, value); return true;
      }
    };
  }
  const fm = {
    fileExistsAtPath: fs.existsSync,
    createDirectoryAtPathWithIntermediateDirectoriesAttributesError: file => {
      fs.mkdirSync(file, { recursive: true }); return true;
    },
    attributesOfItemAtPathError: file => wrapped({ type: fs.lstatSync(file).isFile() ? 'NSFileTypeRegular' : 'other' }),
    copyItemAtPathToPathError: (from, to) => { fs.copyFileSync(from, to); return true; },
    setAttributesOfItemAtPathError: () => true,
    removeItemAtPathError: file => { fs.unlinkSync(file); return true; }
  };
  const bridge = Object.assign(wrapped, {
    NSFileManager: { defaultManager: fm }, NSProcessInfo: { processInfo: { environment: wrapped(environment) } },
    NSHomeDirectory: () => wrapped(nativePath(directory)), NSFileType: 'type', NSUTF8StringEncoding: 4,
    NSUUID: { UUID: { UUIDString: wrapped(require('node:crypto').randomUUID()) } },
    NSString: { stringWithContentsOfFileEncodingError: file => wrapped(fs.readFileSync(file, 'utf8')) },
    NSData: { dataWithContentsOfFile: file => ({ writeToFileAtomically: target => { fs.copyFileSync(file, target); return true; } }) }
  });
  const context = { $: bridge, ObjC: { import() {}, unwrap: obj => obj.raw } };
  vm.createContext(context);
  vm.runInContext(jxa, context);
  return { execute: () => vm.runInContext('run()', context), context };
}
let passed = 0;
function test(name, body) { body(); passed++; console.log('PASS ' + name); }
try {
  const winDir = path.join(root, 'Windows profile with spaces', '.claude');
  const winFile = path.join(winDir, 'settings.json');
  let pristine;
  test('Windows: first install, dynamic directory and Chinese prompts', () => {
    const result = windows(winDir, key);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.ok(fs.existsSync(winFile), result.stdout + result.stderr);
    assert.ok(!result.stdout.includes(key) && !result.stderr.includes(key));
    assert.match(result.stdout, /配置写入完成！/);
    assert.match(result.stdout, /请粘贴 API 密钥（输入可见）/);
    pristine = json(winFile);
    assert.equal(pristine.env.ANTHROPIC_AUTH_TOKEN, key);
    assert.equal(fs.readFileSync(winFile)[0], 123, 'UTF-8 without BOM');
  });
  test('Windows: actual Read-Host echoes the entered key and saves it correctly', () => {
    const directory = path.join(root, 'Visible input profile', '.claude');
    const result = windows(directory, safe, false, true);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.match(result.stdout, /粘贴的内容会显示/);
    assert.ok(result.stdout.includes(safe), 'Visible Read-Host should echo the fake test key');
    assert.equal(json(path.join(directory, 'settings.json')).env.ANTHROPIC_AUTH_TOKEN, safe);
  });
  test('Windows: merge, original-byte backup and repeated key replacement', () => {
    const before = '\ufeff' + JSON.stringify(initial);
    fs.writeFileSync(winFile, before);
    let result = windows(winDir, key);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    preserved(json(winFile), key);
    const backup = fs.readdirSync(winDir).find(name => name.startsWith('backup-'));
    assert.equal(fs.readFileSync(path.join(winDir, backup, 'settings.json'), 'utf8'), before);
    result = windows(winDir, safe);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    preserved(json(winFile), safe);
    assert.equal(fs.readdirSync(winDir).filter(name => name.startsWith('backup-')).length, 2);
  });
  test('Windows: invalid JSON / root / nested structure leave bytes unchanged', () => {
    for (const content of ['{"env":', '[{}]', 'null', '{"env":[]}']) {
      fs.writeFileSync(winFile, content);
      assert.notEqual(windows(winDir, safe).status, 0);
      assert.equal(fs.readFileSync(winFile, 'utf8'), content);
    }
  });
  test('Windows: empty or multiline key and concurrent edits are not applied', () => {
    fs.writeFileSync(winFile, '{}');
    for (const invalid of [' ', 'bad\nkey']) {
      assert.notEqual(windows(winDir, invalid).status, 0);
      assert.equal(fs.readFileSync(winFile, 'utf8'), '{}');
    }
    assert.notEqual(windows(winDir, safe, true).status, 0);
    assert.deepEqual(json(winFile), { concurrent: true });
    assert.ok(!fs.readdirSync(winDir).some(name => name.endsWith('.tmp')));
  });

  const macDir = path.join(root, 'Mac profile with spaces');
  const macFile = path.join(macDir, '.claude', 'settings.json');
  test('macOS logic: settings match Windows template; all shell startup files created', () => {
    const result = mac(macDir, key).execute();
    assert.ok(!result.includes(key));
    assert.deepEqual(json(macFile), pristine);
    for (const name of ['.zshrc', '.bashrc', '.bash_profile']) {
      const content = fs.readFileSync(path.join(macDir, name), 'utf8');
      assert.match(content, /export ANTHROPIC_AUTH_TOKEN=/);
      assert.match(content, /export ENABLE_PROMPT_CACHING_1H='1'/);
    }
  });
  test('macOS logic: merge, exact backup, idempotency and profile content preservation', () => {
    const before = '\ufeff' + JSON.stringify(initial);
    fs.writeFileSync(macFile, before);
    const rcPath = path.join(macDir, '.zshrc');
    fs.writeFileSync(rcPath, '# user comment\nexport KEEP_ME=yes\n');
    mac(macDir, safe).execute();
    preserved(json(macFile), safe);
    const backupRoot = path.dirname(macFile);
    assert.ok(fs.readdirSync(backupRoot).filter(name => name.startsWith('backup-')).some(name =>
      fs.existsSync(path.join(backupRoot, name, '0-settings.json')) &&
      fs.readFileSync(path.join(backupRoot, name, '0-settings.json'), 'utf8') === before));
    const first = fs.readFileSync(rcPath, 'utf8');
    mac(macDir, safe).execute();
    assert.equal(fs.readFileSync(rcPath, 'utf8'), first);
    assert.match(first, /^# user comment\nexport KEEP_ME=yes\n/);
    assert.equal(first.split('# >>> xunyou-claude >>>').length, 2);
  });
  test('macOS logic: invalid JSON and malformed blocks abort all target writes', () => {
    const before = fs.readFileSync(macFile, 'utf8');
    fs.writeFileSync(macFile, '{broken');
    assert.throws(() => mac(macDir, safe).execute(), /JSON/);
    assert.equal(fs.readFileSync(macFile, 'utf8'), '{broken');
    fs.writeFileSync(macFile, before);
    const rcPath = path.join(macDir, '.bashrc');
    const rcBefore = fs.readFileSync(rcPath, 'utf8');
    fs.writeFileSync(rcPath, '# >>> xunyou-claude >>>\n');
    assert.throws(() => mac(macDir, safe).execute(), /标记/);
    assert.equal(fs.readFileSync(macFile, 'utf8'), before);
    fs.writeFileSync(rcPath, rcBefore);
  });
  test('macOS logic: partial write failure restores prior files exactly', () => {
    const files = [macFile, ...['.zshrc', '.bashrc', '.bash_profile'].map(name => path.join(macDir, name))];
    const before = files.map(file => fs.readFileSync(file, 'utf8'));
    assert.throws(() => mac(macDir, key, { failPath: files[2] }).execute(), /已恢复/);
    files.forEach((file, i) => assert.equal(fs.readFileSync(file, 'utf8'), before[i]));
  });
  test('macOS logic: CLAUDE_CONFIG_DIR, ZDOTDIR and Bash login precedence', () => {
    const custom = path.join(root, 'custom mac');
    fs.mkdirSync(custom);
    fs.writeFileSync(path.join(custom, '.bash_login'), '# existing login\n');
    fs.writeFileSync(path.join(custom, '.profile'), '# fallback\n');
    const config = path.join(custom, 'config');
    const zsh = path.join(custom, 'zsh');
    mac(custom, safe, { env: { CLAUDE_CONFIG_DIR: config, ZDOTDIR: zsh } }).execute();
    assert.equal(json(path.join(config, 'settings.json')).env.ANTHROPIC_AUTH_TOKEN, safe);
    assert.ok(fs.existsSync(path.join(zsh, '.zshrc')));
    assert.ok(!fs.existsSync(path.join(custom, '.bash_profile')));
    assert.match(fs.readFileSync(path.join(custom, '.bash_login'), 'utf8'), /xunyou-claude/);
    assert.equal(fs.readFileSync(path.join(custom, '.profile'), 'utf8'), '# fallback\n');
  });
  if (process.env.CLAUDE_TEST_BASH) {
    test('Bash: shell syntax and actual source round-trip for quote / dollar / backtick key', () => {
      const syntax = spawnSync(process.env.CLAUDE_TEST_BASH, ['-n', path.join(__dirname, 'setup-claude.sh')], { encoding: 'utf8' });
      assert.equal(syntax.status, 0, syntax.stderr);
      const rc = path.join(macDir, '.zshrc').replace(/\\/g, '/');
      const result = spawnSync(process.env.CLAUDE_TEST_BASH, ['--noprofile', '--norc', '-c',
        'source "$1"; printf "%s" "$ANTHROPIC_AUTH_TOKEN"', 'test', rc], { encoding: 'utf8' });
      assert.equal(result.status, 0, result.stderr);
      assert.equal(result.stdout, safe);
    });
  }
  console.log(`${passed} checks passed. Real macOS Foundation/terminal acceptance is not covered.`);
} finally {
  // Only remove the directory returned by mkdtemp; never a user configuration directory.
  fs.rmSync(root, { recursive: true, force: true });
}
