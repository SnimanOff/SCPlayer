package localipc

import (
	"os/exec"
	"syscall"
)

func PrepareChild(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM} }

func BindChild(*exec.Cmd) {}
