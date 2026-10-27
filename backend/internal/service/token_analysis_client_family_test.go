package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// 用例取自生产样本中的真实 UA, 覆盖规则顺序依赖(codex 带 vscode、claude-cli 带 sdk)。
func TestTokenAnalysisClientFamilyDefaultRules(t *testing.T) {
	m := newTokenAnalysisClientFamilyMatcher(nil)
	tests := []struct {
		ua   string
		want string
	}{
		{"Codex Desktop/0.163.0-alpha.10.2 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.1013.40934)", "codex"},
		{"codex-tui/0.164.0 (Windows 10.0.22621; x86_64) vscode/1.95.3 (codex-tui; 0.163.0)", "codex"},
		{"codex_vscode/0.162.0-alpha.17.2 (Windows 10.0.26100; x86_64) unknown (VS Code; 26.1007.21434)", "codex"},
		{"claude-cli/2.1.306 (external, claude-vscode, agent-sdk/0.3.306)", "claude_code"},
		{"GitHubCopilotChat/0.69.0", "copilot"},
		{"opencode/latest/2.0.30/cli", "opencode"},
		{"opencode/1.19.3 ai-sdk/provider-utils/4.0.23 runtime/node.js/24", "opencode"},
		{"hertz", "hertz"},
		{"Go-http-client/1.1", "sdk_script"},
		{"OpenAI/Python 2.24.0", "sdk_script"},
		{"Python-urllib/3.14", "sdk_script"},
		{"tauri-plugin-http/2.5.9", "other"},
		{"pi/1.4.0 (linux; node/v26.12.0; x64)", "other"},
		{"", "other"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, m.Match(tt.ua), tt.ua)
	}
}

func TestTokenAnalysisClientFamilyCustomRulesReplaceDefaults(t *testing.T) {
	m := newTokenAnalysisClientFamilyMatcher([]config.TokenAnalysisClientFamilyRule{
		{Family: "internal_tool", Pattern: `^tauri-plugin-http/`},
	})
	require.Equal(t, "internal_tool", m.Match("TAURI-plugin-http/2.5.9"))
	// 自定义规则整体替换内置规则。
	require.Equal(t, "other", m.Match("Codex Desktop/0.160.0"))
}

func TestTokenAnalysisClientFamilyVersion(t *testing.T) {
	a := newTokenAnalysisClientFamilyMatcher(nil)
	b := newTokenAnalysisClientFamilyMatcher(nil)
	require.Equal(t, a.version, b.version)
	require.Positive(t, a.version)

	changed := newTokenAnalysisClientFamilyMatcher([]config.TokenAnalysisClientFamilyRule{{Family: "codex", Pattern: `codex`}})
	require.NotEqual(t, a.version, changed.version)
}

func TestTokenAnalysisAgentFamily(t *testing.T) {
	require.True(t, IsTokenAnalysisAgentFamily("codex"))
	require.True(t, IsTokenAnalysisAgentFamily("hertz"))
	require.False(t, IsTokenAnalysisAgentFamily("sdk_script"))
	require.False(t, IsTokenAnalysisAgentFamily("other"))
	require.False(t, IsTokenAnalysisAgentFamily(""))
}

func TestNormalizeTokenAnalysisUserAgent(t *testing.T) {
	require.Equal(t, "ab", normalizeTokenAnalysisUserAgent(" a\x00b "))
	long := strings.Repeat("界", 600)
	require.Len(t, []rune(normalizeTokenAnalysisUserAgent(long)), tokenAnalysisUserAgentMaxChars)
}

// 规则变化后按存下的 UA 分批重新映射: 游标续读, 扫完一遍后低频复查, 因此滚动
// 发布期间旧实例晚写入的旧版本行最终也会被补映射。
func TestTokenAnalysisRefreshClientFamiliesCursorAndRecheck(t *testing.T) {
	repo := &tokenAnalysisRepoStub{staleFamilies: []TokenAnalysisClientFamilyRow{
		{ID: 1, UserAgent: "Codex Desktop/0.160.0", Version: 5},
		{ID: 2, UserAgent: "hertz", Version: 5},
		{ID: 3, UserAgent: "", Version: 5},
		{ID: 4, UserAgent: "", Version: 0}, // 未识别的历史行, 等受控重索引
	}}
	svc := NewTokenAnalysisService(repo, &config.Config{}, nil)
	ctx := context.Background()
	now := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)

	n, err := svc.refreshTokenAnalysisClientFamilies(ctx, 2, now)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	n, err = svc.refreshTokenAnalysisClientFamilies(ctx, 2, now)
	require.NoError(t, err)
	require.Equal(t, 1, n, "cursor continues after id 2")
	require.Equal(t, []string{"codex", "hertz", "other"}, []string{repo.familyUpdates[0].Family, repo.familyUpdates[1].Family, repo.familyUpdates[2].Family})

	// 扫完一遍后, 旧实例又写入一条旧版本行: 复查间隔内不扫表, 到期后补上。
	repo.staleFamilies = append(repo.staleFamilies, TokenAnalysisClientFamilyRow{ID: 5, UserAgent: "GitHubCopilotChat/0.69.0", Version: 5})
	calls := repo.familyListCalls
	n, err = svc.refreshTokenAnalysisClientFamilies(ctx, 2, now.Add(time.Minute))
	require.NoError(t, err)
	require.Zero(t, n)
	require.Equal(t, calls, repo.familyListCalls, "no scan before the recheck interval")
	n, err = svc.refreshTokenAnalysisClientFamilies(ctx, 2, now.Add(tokenAnalysisClientFamilyRecheckInterval+time.Minute))
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "copilot", repo.familyUpdates[len(repo.familyUpdates)-1].Family)
	require.EqualValues(t, 0, repo.staleFamilies[3].Version, "version-0 legacy rows are untouched")
}

// 写回按读出快照条件提交: 读出后被改过的行不被覆盖, 下一遍重新读取。
func TestTokenAnalysisRefreshClientFamiliesSkipsChangedRows(t *testing.T) {
	repo := &changingFamilyRepoStub{}
	repo.staleFamilies = []TokenAnalysisClientFamilyRow{{ID: 1, UserAgent: "Codex Desktop/0.160.0", Version: 5}}
	svc := NewTokenAnalysisService(repo, &config.Config{}, nil)

	_, err := svc.refreshTokenAnalysisClientFamilies(context.Background(), 10, time.Now())
	require.NoError(t, err)
	require.Empty(t, repo.familyUpdates)
	require.Equal(t, TokenAnalysisClientFamilyRow{ID: 1, UserAgent: "hertz", Version: 9, Family: "hertz"}, repo.staleFamilies[0])
}

// changingFamilyRepoStub 在读出与写回之间模拟另一次索引改写该行。
type changingFamilyRepoStub struct {
	tokenAnalysisRepoStub
}

func (r *changingFamilyRepoStub) ListStaleClientFamilies(ctx context.Context, version int32, afterID int64, limit int) ([]TokenAnalysisClientFamilyRow, error) {
	rows, err := r.tokenAnalysisRepoStub.ListStaleClientFamilies(ctx, version, afterID, limit)
	r.staleFamilies[0] = TokenAnalysisClientFamilyRow{ID: 1, UserAgent: "hertz", Version: 9, Family: "hertz"}
	return rows, err
}
