package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// lockedTokenAnalysisRepoStub 给并发 worker 用的加锁 stub。
type lockedTokenAnalysisRepoStub struct {
	tokenAnalysisRepoStub
	mu sync.Mutex
}

func (r *lockedTokenAnalysisRepoStub) UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokenAnalysisRepoStub.UpsertRequestSummary(ctx, summary)
}

func (r *lockedTokenAnalysisRepoStub) upsertCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.upserts)
}

func TestTokenAnalysisAutoIndexIndexesToday(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("2006-01-02")
	line := `{"archive_id":"auto1","event":"request","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash-auto1"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, today+".jsonl"), []byte(line), 0o600))

	repo := &lockedTokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	}, nil)

	svc.startAutoIndexWithInterval(20 * time.Millisecond)
	defer svc.StopAutoIndex()

	require.Eventually(t, func() bool {
		return repo.upsertCount() >= 1
	}, 3*time.Second, 10*time.Millisecond)

	// Stop 等待 goroutine 退出后再读, 避免与 worker 写并发。
	svc.StopAutoIndex()
	require.Equal(t, "auto1", repo.upserts[0].ArchiveID)
}

func TestTokenAnalysisAutoIndexDisabled(t *testing.T) {
	repo := &lockedTokenAnalysisRepoStub{}

	// index_enabled=false: 不启动。
	svcDisabled := NewTokenAnalysisService(repo, &config.Config{
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: false, AutoIndexIntervalSeconds: 1},
	}, nil)
	svcDisabled.StartAutoIndex()
	svcDisabled.StopAutoIndex()

	// interval=0: 不启动。
	svcZero := NewTokenAnalysisService(repo, &config.Config{
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, AutoIndexIntervalSeconds: 0},
	}, nil)
	svcZero.StartAutoIndex()
	svcZero.StopAutoIndex()

	require.Zero(t, repo.upsertCount())
}
