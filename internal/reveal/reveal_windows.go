package reveal

import (
	"os/exec"
	"syscall"
)

// open runs `explorer.exe /select,"<path>"`. The command line is passed
// verbatim in CmdLine on purpose: through Args, Go would backslash-escape the
// quotes (syscall.EscapeArg), explorer's own parser has no such escape, and
// the file is not found, so it opens the parent folder instead. A path cannot
// contain `"` on Windows, so quoting it ourselves is safe; filepath.Clean in
// File gave it backslashes, which /select requires.
func open(path string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	return launch(cmd)
}
