package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// gatewaySnapshotRepo stores the timezone rewrite switch and can hold one snapshot
// read after it has taken its values, so a settings save can land between that
// read and the cache publish.
type gatewaySnapshotRepo struct {
	*gatewayTTLSettingRepo
	mu        sync.Mutex
	global    string
	readErr   error
	getAllErr error
	taken     chan struct{}
	release   chan struct{}
	takenAt   time.Time
}

func newGatewaySnapshotRepo(global string) *gatewaySnapshotRepo {
	return &gatewaySnapshotRepo{gatewayTTLSettingRepo: &gatewayTTLSettingRepo{data: map[string]string{}}, global: global}
}

// holdNextRead makes the next GetMultiple block after reading until release is closed.
func (r *gatewaySnapshotRepo) holdNextRead() (taken, release chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.taken, r.release = make(chan struct{}), make(chan struct{})
	return r.taken, r.release
}

func (r *gatewaySnapshotRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	values, _ := r.gatewayTTLSettingRepo.GetMultiple(ctx, keys)
	values[SettingKeyEnableOpenAIRequestTimezoneRewrite] = r.global
	err := r.readErr
	taken, release := r.taken, r.release
	r.taken, r.release = nil, nil
	r.takenAt = time.Now()
	r.mu.Unlock()
	if taken != nil {
		close(taken)
		<-release
	}
	if err != nil {
		return nil, err
	}
	return values, nil
}

func (r *gatewaySnapshotRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if value, ok := settings[SettingKeyEnableOpenAIRequestTimezoneRewrite]; ok {
		r.global = value
	}
	return r.gatewayTTLSettingRepo.SetMultiple(ctx, settings)
}

func (r *gatewaySnapshotRepo) GetAll(ctx context.Context) (map[string]string, error) {
	r.mu.Lock()
	err := r.getAllErr
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return r.gatewayTTLSettingRepo.GetAll(ctx)
}

func (r *gatewaySnapshotRepo) setFailures(readErr, getAllErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readErr, r.getAllErr = readErr, getAllErr
}

func newGatewaySnapshotSettings(t *testing.T, repo *gatewaySnapshotRepo) *SettingService {
	t.Helper()
	expireLocaleSettingsSnapshot()
	t.Cleanup(expireLocaleSettingsSnapshot)
	return NewSettingService(repo, &config.Config{})
}

// rewriteStateInFlight starts a snapshot load that is held right after it read
// the stored value; the returned channel yields what that caller finally sees.
func rewriteStateInFlight(t *testing.T, settings *SettingService, repo *gatewaySnapshotRepo) (release chan struct{}, seen <-chan bool) {
	t.Helper()
	taken, release := repo.holdNextRead()
	result := make(chan bool, 1)
	go func() {
		enabled, _ := settings.OpenAIRequestTimezoneRewriteState(context.Background())
		result <- enabled
	}()
	select {
	case <-taken:
	case <-time.After(3 * time.Second):
		t.Fatal("snapshot load did not start")
	}
	return release, result
}

func requireRewriteState(t *testing.T, settings *SettingService, want bool) {
	t.Helper()
	enabled, _ := settings.OpenAIRequestTimezoneRewriteState(context.Background())
	require.Equal(t, want, enabled)
}

func TestGatewayForwardingSnapshotIgnoresReadThatPredatesSave(t *testing.T) {
	repo := newGatewaySnapshotRepo("true")
	settings := newGatewaySnapshotSettings(t, repo)
	release, seen := rewriteStateInFlight(t, settings, repo)

	require.NoError(t, settings.UpdateSettings(context.Background(), &SystemSettings{EnableOpenAIRequestTimezoneRewrite: false}))
	requireRewriteState(t, settings, false)
	saved := gatewayForwardingCache.Load()

	close(release)
	require.False(t, <-seen, "the request that started the stale read gets the saved value")
	requireRewriteState(t, settings, false)
	require.Same(t, saved, gatewayForwardingCache.Load(), "the stale read must not replace the saved snapshot")
}

func TestGatewayForwardingSnapshotInvalidatedWhenReloadAfterPartialSaveFails(t *testing.T) {
	repo := newGatewaySnapshotRepo("true")
	settings := newGatewaySnapshotSettings(t, repo)
	requireRewriteState(t, settings, true)

	repo.setFailures(nil, errors.New("settings database unavailable"))
	omitted := OmittedSettingKeys{SettingKeySiteName: {}}
	require.NoError(t, settings.UpdateSettingsOmitting(context.Background(), &SystemSettings{EnableOpenAIRequestTimezoneRewrite: false}, omitted))
	require.Equal(t, "false", repo.global)
	requireRewriteState(t, settings, false)

	// Storage failing entirely after the save still keeps the rewrite off.
	repo.mu.Lock()
	repo.global = "true"
	repo.mu.Unlock()
	repo.setFailures(errors.New("settings database unavailable"), errors.New("settings database unavailable"))
	require.NoError(t, settings.UpdateSettingsOmitting(context.Background(), &SystemSettings{EnableOpenAIRequestTimezoneRewrite: false}, omitted))
	enabled, unavailable := settings.OpenAIRequestTimezoneRewriteState(context.Background())
	require.False(t, enabled)
	require.True(t, unavailable)
}

func TestGatewayForwardingSnapshotRereadsWhenInvalidatedDuringRead(t *testing.T) {
	repo := newGatewaySnapshotRepo("true")
	settings := newGatewaySnapshotSettings(t, repo)
	release, seen := rewriteStateInFlight(t, settings, repo)

	repo.setFailures(nil, errors.New("settings database unavailable"))
	require.NoError(t, settings.UpdateSettingsOmitting(context.Background(), &SystemSettings{EnableOpenAIRequestTimezoneRewrite: false}, OmittedSettingKeys{SettingKeySiteName: {}}))

	close(release)
	require.False(t, <-seen, "an invalidation during the read forces a fresh read of the saved value")
	requireRewriteState(t, settings, false)
}

func TestGatewayForwardingSnapshotExpiryAnchoredAtReadStart(t *testing.T) {
	repo := newGatewaySnapshotRepo("true")
	settings := newGatewaySnapshotSettings(t, repo)
	release, seen := rewriteStateInFlight(t, settings, repo)
	time.Sleep(100 * time.Millisecond)
	close(release)
	require.True(t, <-seen)

	cached := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings)
	repo.mu.Lock()
	takenAt := repo.takenAt
	repo.mu.Unlock()
	require.LessOrEqual(t, cached.expiresAt, takenAt.Add(gatewayForwardingCacheTTL).UnixNano(),
		"a slow read must not extend the snapshot beyond TTL from when the value was read")
}
