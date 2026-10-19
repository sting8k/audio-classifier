package batch

import (
	"io/fs"
	"strings"
	"syscall"
)

// isHidden: dot-files, or the Windows "hidden" attribute (read from the
// directory listing, no extra syscall).
func isHidden(d fs.DirEntry) bool {
	if strings.HasPrefix(d.Name(), ".") {
		return true
	}
	info, err := d.Info()
	if err != nil {
		return false
	}
	a, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && a.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
