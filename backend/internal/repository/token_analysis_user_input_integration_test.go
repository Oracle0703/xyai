//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 质量评估与内容哈希绑定: 同一份输入重建索引保留评估, 内容变化即清空; 删除后不可再读到。
func TestTokenAnalysisUserInputQualityFollowsContentHash(t *testing.T) {
	ctx := context.Background()
	repo := NewTokenAnalysisRepository(integrationDB)
	archiveID := fmt.Sprintf("origin-review-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_user_inputs WHERE archive_id = $1`, archiveID)
	})
	eventTime := time.Date(2026, 10, 23, 1, 2, 3, 0, time.UTC)
	upsert := func(content, sha string) {
		require.NoError(t, repo.UpsertUserInput(ctx, &service.TokenAnalysisUserInput{
			ArchiveID: archiveID, EventTime: eventTime, Content: content, ContentSHA256: sha, Chars: len(content),
		}))
	}

	upsert("I will read the file", "sha-old")
	_, err := integrationDB.ExecContext(ctx, `
UPDATE token_analysis_user_inputs
SET quality_score = 80, quality_findings = '{"clarity":1}', quality_version = 'v1', evaluated_at = NOW()
WHERE archive_id = $1`, archiveID)
	require.NoError(t, err)

	upsert("I will read the file", "sha-old")
	got, err := repo.GetUserInput(ctx, archiveID)
	require.NoError(t, err)
	require.NotNil(t, got.QualityScore)
	require.EqualValues(t, 80, *got.QualityScore)
	require.Equal(t, "v1", got.QualityVersion)
	require.NotNil(t, got.EvaluatedAt)
	require.NotEmpty(t, got.QualityFindings)

	upsert("fix the login bug", "sha-new")
	got, err = repo.GetUserInput(ctx, archiveID)
	require.NoError(t, err)
	require.Equal(t, "fix the login bug", got.Content)
	require.Equal(t, "sha-new", got.ContentSHA256)
	require.Nil(t, got.QualityScore)
	require.Empty(t, got.QualityFindings)
	require.Empty(t, got.QualityVersion)
	require.Nil(t, got.EvaluatedAt)

	require.NoError(t, repo.DeleteUserInput(ctx, archiveID))
	got, err = repo.GetUserInput(ctx, archiveID)
	require.NoError(t, err)
	require.Nil(t, got)
}
