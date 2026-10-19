//go:build !darwin && !windows

package reveal

import (
	"os/exec"
	"path/filepath"
)

// open shows the containing folder; Linux file managers have no common
// "select this file" call.
func open(path string) error { return launch(exec.Command("xdg-open", filepath.Dir(path))) }
