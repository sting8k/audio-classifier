// Package reveal shows a file in the OS file manager (Finder / Explorer; on
// Linux, which has no common "select this file" call, its folder).
package reveal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// File reveals path, which must be absolute and exist.
func File(path string) error {
	// Absolute also keeps a path from being read as an option by `open`.
	if !filepath.IsAbs(path) {
		return fmt.Errorf("đường dẫn không hợp lệ: %q", path)
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("không tìm thấy file: %w", err)
	}
	return open(path)
}

// launch starts a file-manager process without waiting for it: they either
// stay running or (explorer.exe) report failure on success.
func launch(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap
	return nil
}
