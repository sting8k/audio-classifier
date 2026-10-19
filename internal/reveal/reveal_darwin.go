package reveal

import "os/exec"

func open(path string) error { return launch(exec.Command("open", "-R", path)) }
