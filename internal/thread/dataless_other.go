//go:build !darwin

package thread

import "io/fs"

func isDatalessFile(info fs.FileInfo) bool { return false }
