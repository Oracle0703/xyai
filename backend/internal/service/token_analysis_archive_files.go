package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 归档文件状态: 供页面打标签, 删除动作本身仍由运维在服务器上手动执行。
const (
	// TokenAnalysisArchiveFileWriting 今日文件, 写入端正持有句柄, 不可删。
	TokenAnalysisArchiveFileWriting = "writing"
	// TokenAnalysisArchiveFileIndexing 索引水位尚未追平文件大小。
	TokenAnalysisArchiveFileIndexing = "indexing"
	// TokenAnalysisArchiveFileDeletable 已全部入库且无失败行, 可安全 gzip/删除。
	TokenAnalysisArchiveFileDeletable = "deletable"
	// TokenAnalysisArchiveFileAttention 已读完但存在失败行, 删除即放弃重处理机会。
	TokenAnalysisArchiveFileAttention = "attention"
	// TokenAnalysisArchiveFileCompressed 已压缩文件(.gz), 不参与索引。
	TokenAnalysisArchiveFileCompressed = "compressed"
)

type TokenAnalysisArchiveFile struct {
	Name          string    `json:"name"`
	SizeBytes     int64     `json:"size_bytes"`
	ModTime       time.Time `json:"mod_time"`
	IndexedOffset int64     `json:"indexed_offset"`
	ProcessedRows int64     `json:"processed_rows"`
	FailedRows    int64     `json:"failed_rows"`
	LastError     string    `json:"last_error"`
	Status        string    `json:"status"`
}

// ListArchiveFiles 列出当前生效归档目录下的 JSONL(及 .gz)文件,
// 与 token_analysis_index_state 的索引水位比对后给出可删除标签。
// 切换过归档目录时旧目录文件不在列表内(与索引器同源, 只看当前目录)。
func (s *TokenAnalysisService) ListArchiveFiles(ctx context.Context) ([]TokenAnalysisArchiveFile, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("token analysis service is not configured")
	}
	dir := s.archiveDir(ctx)
	patterns := []string{"*.jsonl", "*.jsonl.gz"}
	paths := make([]string, 0)
	for _, pattern := range patterns {
		matched, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, fmt.Errorf("list archive files: %w", err)
		}
		paths = append(paths, matched...)
	}

	today := time.Now().Format("2006-01-02")
	files := make([]TokenAnalysisArchiveFile, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		file := TokenAnalysisArchiveFile{
			Name:      filepath.Base(path),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime(),
		}
		if strings.HasSuffix(path, ".gz") {
			file.Status = TokenAnalysisArchiveFileCompressed
			files = append(files, file)
			continue
		}
		state, err := s.repo.GetIndexState(ctx, path)
		if err != nil {
			return nil, err
		}
		if state != nil {
			file.IndexedOffset = state.LastOffset
			file.ProcessedRows = state.ProcessedRows
			file.FailedRows = state.FailedRows
			file.LastError = state.LastError
		}
		file.Status = tokenAnalysisArchiveFileStatus(file, today)
		files = append(files, file)
	}
	// 文件名即日期, 倒序 = 新文件在前。
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return files, nil
}

func tokenAnalysisArchiveFileStatus(file TokenAnalysisArchiveFile, today string) string {
	if strings.TrimSuffix(file.Name, ".jsonl") == today {
		return TokenAnalysisArchiveFileWriting
	}
	if file.IndexedOffset < file.SizeBytes {
		return TokenAnalysisArchiveFileIndexing
	}
	if file.FailedRows > 0 || strings.TrimSpace(file.LastError) != "" {
		return TokenAnalysisArchiveFileAttention
	}
	return TokenAnalysisArchiveFileDeletable
}
