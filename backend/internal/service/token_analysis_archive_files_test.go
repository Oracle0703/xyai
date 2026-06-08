package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisListArchiveFilesTagsDeletable(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("2006-01-02")
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	todayPath := write(today+".jsonl", "live\n")
	donePath := write("2026-01-01.jsonl", strings.Repeat("x", 100))
	partialPath := write("2026-01-02.jsonl", strings.Repeat("x", 100))
	failedPath := write("2026-01-03.jsonl", strings.Repeat("x", 100))
	write("2026-01-04.jsonl.gz", "gz")
	_ = todayPath

	repo := &tokenAnalysisRepoStub{indexStates: map[string]TokenAnalysisIndexState{
		donePath:    {SourceFile: donePath, LastOffset: 100, ProcessedRows: 10},
		partialPath: {SourceFile: partialPath, LastOffset: 40, ProcessedRows: 4},
		failedPath:  {SourceFile: failedPath, LastOffset: 100, ProcessedRows: 9, FailedRows: 1},
	}}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
	}, nil)

	files, err := svc.ListArchiveFiles(context.Background())
	require.NoError(t, err)
	require.Len(t, files, 5)

	byName := make(map[string]TokenAnalysisArchiveFile, len(files))
	for _, f := range files {
		byName[f.Name] = f
	}
	// 今日文件写入中, 永远不可删。
	require.Equal(t, TokenAnalysisArchiveFileWriting, byName[today+".jsonl"].Status)
	// 水位追平 + 无失败 = 可删除。
	require.Equal(t, TokenAnalysisArchiveFileDeletable, byName["2026-01-01.jsonl"].Status)
	require.Equal(t, int64(100), byName["2026-01-01.jsonl"].IndexedOffset)
	// 水位未追平 = 待索引。
	require.Equal(t, TokenAnalysisArchiveFileIndexing, byName["2026-01-02.jsonl"].Status)
	// 读完但有失败行 = 谨慎删除。
	require.Equal(t, TokenAnalysisArchiveFileAttention, byName["2026-01-03.jsonl"].Status)
	// gz 不参与索引。
	require.Equal(t, TokenAnalysisArchiveFileCompressed, byName["2026-01-04.jsonl.gz"].Status)

	// 新文件在前(文件名即日期, 倒序)。
	require.Equal(t, today+".jsonl", files[0].Name)
}
