package engine

import (
	"os/exec"
	"syscall"
)

// hideWindow stops Windows from flashing a console window for every ffmpeg.exe the GUI starts.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
