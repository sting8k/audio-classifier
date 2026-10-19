//go:build !windows

package batch

import (
	"io/fs"
	"strings"
)

// isHidden: dot-files, which includes macOS AppleDouble "._name" companions.
func isHidden(d fs.DirEntry) bool { return strings.HasPrefix(d.Name(), ".") }
