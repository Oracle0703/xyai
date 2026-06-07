package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// requestArchiveDirEquals 判断两个归档目录路径是否等价:
// Clean 归一化后比较, Windows 文件系统大小写不敏感故用 EqualFold。
func requestArchiveDirEquals(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// ValidateRequestArchiveDir 校验后台自定义归档目录是否可用:
//  1. 必须是绝对路径(进程工作目录不可靠, 相对路径含义随部署方式漂移);
//  2. 所在磁盘/卷必须存在(典型场景: Windows 盘符填错或新盘未挂载);
//  3. 路径不能是已存在的文件; 目录不存在时尝试创建;
//  4. 写探针: 创建并删除临时文件, 确认进程对目录有写权限。
//
// 校验失败返回 BadRequest 级错误, 由设置接口透传给前端展示。
func ValidateRequestArchiveDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return infraerrors.BadRequest("REQUEST_ARCHIVE_DIR_NOT_ABSOLUTE",
			fmt.Sprintf("archive dir must be an absolute path, got %q", dir))
	}
	if volume := filepath.VolumeName(dir); volume != "" {
		if _, err := os.Stat(volume + string(filepath.Separator)); err != nil {
			return infraerrors.BadRequest("REQUEST_ARCHIVE_DIR_VOLUME_NOT_FOUND",
				fmt.Sprintf("disk/volume %q does not exist or is not mounted: %v", volume, err))
		}
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return infraerrors.BadRequest("REQUEST_ARCHIVE_DIR_IS_FILE",
			fmt.Sprintf("archive dir %q points to an existing file", dir))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return infraerrors.BadRequest("REQUEST_ARCHIVE_DIR_CREATE_FAILED",
			fmt.Sprintf("create archive dir %q failed: %v", dir, err))
	}
	probe, err := os.CreateTemp(dir, ".archive-dir-probe-*")
	if err != nil {
		return infraerrors.BadRequest("REQUEST_ARCHIVE_DIR_NOT_WRITABLE",
			fmt.Sprintf("archive dir %q is not writable: %v", dir, err))
	}
	probePath := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probePath)
	return nil
}
