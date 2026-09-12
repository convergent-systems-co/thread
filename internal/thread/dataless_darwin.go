//go:build darwin

package thread

import (
	"io/fs"
	"syscall"
)

// Modern FileProvider storage evicts files in place. Their names and sizes
// remain local, but st_blocks is zero until reading materializes their content.
func isDatalessFile(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Size() > 0 && stat.Blocks == 0
}
