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

// blockingTokenAnalysisRepoStub 在 FindNearestUsageLog 阻塞直到 ctx 取消,
// 模拟一轮长索引(每行索引都会经过 usage 匹配)。
type blockingTokenAnalysisRepoStub struct {
	tokenAnalysisRepoStub
	entered chan struct{}
}

func (r *blockingTokenAnalysisRepoStub) FindNearestUsageLog(ctx context.Context, eventTime time.Time, userID, apiKeyID *int64, model string, window time.Duration) (*TokenAnalysisUsageMatch, error) {
	select {
	case r.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTokenAnalysisAutoIndexStopCancelsRunningRound(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("2006-01-02")
	line := `{"archive_id":"blk1","event":"request","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash-blk1"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, today+".jsonl"), []byte(line), 0o600))

	repo := &blockingTokenAnalysisRepoStub{entered: make(chan struct{}, 1)}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	}, nil)

	svc.startAutoIndexWithInterval(time.Hour)
	// 等索引轮次真正进入阻塞点。
	select {
	case <-repo.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("auto index round did not start")
	}

	// Stop 必须取消进行中的轮次并快速返回, 不被阻塞的索引拖住。
	done := make(chan struct{})
	go func() {
		svc.StopAutoIndex()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("StopAutoIndex did not return after cancelling running round")
	}
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
