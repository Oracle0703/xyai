// Package diskspace 提供跨平台的磁盘容量查询, 用于归档目录切换前
// 在管理后台展示目标磁盘的总容量与剩余空间。
package diskspace

// Usage 返回 path 所在磁盘/卷的总字节数与可用字节数。
// path 需要指向已存在的目录(或文件), 否则返回错误。
func Usage(path string) (total uint64, free uint64, err error) {
	return usage(path)
}
