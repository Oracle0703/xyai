package service

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

type startAfterStopTokenAnalysisRepoStub struct {
	tokenAnalysisRepoStub
	loadProjectRootsCalled chan struct{}
}

func (r *startAfterStopTokenAnalysisRepoStub) LoadProjectRoots(context.Context) (map[string]string, error) {
	select {
	case r.loadProjectRootsCalled <- struct{}{}:
	default:
	}
	return nil, nil
}

func TestTokenAnalysisAutoIndexStartAfterStopDoesNotTouchRepository(t *testing.T) {
	repo := &startAfterStopTokenAnalysisRepoStub{loadProjectRootsCalled: make(chan struct{}, 1)}
	svc := NewTokenAnalysisService(repo, &config.Config{
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true},
	}, nil)

	svc.StopAutoIndex()
	svc.startAutoIndexWithInterval(time.Hour)

	select {
	case <-repo.loadProjectRootsCalled:
		t.Fatal("StartAutoIndex touched the repository after StopAutoIndex")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTokenAnalysisIndexRangeAsyncAfterStopIsRejected(t *testing.T) {
	repo := &startAfterStopTokenAnalysisRepoStub{loadProjectRootsCalled: make(chan struct{}, 1)}
	svc := NewTokenAnalysisService(repo, &config.Config{
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true},
	}, nil)

	svc.StopAutoIndex()
	err := svc.IndexRangeAsync(TokenAnalysisIndexRequest{
		StartDate: "2026-08-12",
		EndDate:   "2026-08-12",
	})

	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	select {
	case <-repo.loadProjectRootsCalled:
		t.Fatal("IndexRangeAsync touched the repository after StopAutoIndex")
	case <-time.After(100 * time.Millisecond):
	}
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

func TestTokenAnalysisIndexRangeAsyncRunsInBackground(t *testing.T) {
	dir := t.TempDir()
	line := `{"archive_id":"async1","event":"request","timestamp":"2026-06-09T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash-async1"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-06-09.jsonl"), []byte(line), 0o600))

	repo := &lockedTokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	}, nil)

	// 非法日期同步报 400, 不会吞进后台。
	err := svc.IndexRangeAsync(TokenAnalysisIndexRequest{StartDate: "not-a-date"})
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))

	require.NoError(t, svc.IndexRangeAsync(TokenAnalysisIndexRequest{StartDate: "2026-06-09", EndDate: "2026-06-09"}))
	require.Eventually(t, func() bool {
		return repo.upsertCount() >= 1
	}, 3*time.Second, 10*time.Millisecond)

	// Stop 等待后台 goroutine 退出后再读, 避免与 worker 写并发。
	svc.StopAutoIndex()
	require.Equal(t, "async1", repo.upserts[0].ArchiveID)
}

func TestTokenAnalysisIndexRangeAsyncConflictsWhileRunning(t *testing.T) {
	dir := t.TempDir()
	line := `{"archive_id":"blk2","event":"request","timestamp":"2026-06-09T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash-blk2"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2026-06-09.jsonl"), []byte(line), 0o600))

	repo := &blockingTokenAnalysisRepoStub{entered: make(chan struct{}, 1)}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	}, nil)

	require.NoError(t, svc.IndexRangeAsync(TokenAnalysisIndexRequest{StartDate: "2026-06-09", EndDate: "2026-06-09"}))
	select {
	case <-repo.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("async index run did not start")
	}

	// 运行期间再次触发: 同步 409。
	err := svc.IndexRangeAsync(TokenAnalysisIndexRequest{StartDate: "2026-06-09", EndDate: "2026-06-09"})
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))

	// Stop 取消进行中的手动轮次并快速返回。
	done := make(chan struct{})
	go func() {
		svc.StopAutoIndex()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("StopAutoIndex did not return after cancelling manual run")
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

func TestProvideTokenAnalysisServiceStartsAutoIndexOnce(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("2006-01-02")
	line := `{"archive_id":"once1","event":"request","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `","method":"POST","endpoint":"/v1/responses","user_id":7,"api_key_id":9,"model":"gpt-5","body":"{\"model\":\"gpt-5\",\"input\":\"hello\"}","body_size":48,"body_sha256":"hash-once1"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, today+".jsonl"), []byte(line), 0o600))

	repo := &lockedTokenAnalysisRepoStub{}
	svc := ProvideTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, AutoIndexIntervalSeconds: 3600, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	}, nil)

	// Provider 已启动一次；重复调用公开 Start 不能创建第二条 loop。
	svc.StartAutoIndex()
	require.Eventually(t, func() bool { return repo.upsertCount() >= 1 }, 3*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	svc.StopAutoIndex()

	require.Equal(t, 1, repo.upsertCount(), "repeated StartAutoIndex must not create a second indexing loop")
}
