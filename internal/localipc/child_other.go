//go:build !windows && !linux

package localipc

import "os/exec"

func PrepareChild(*exec.Cmd) {}
func BindChild(*exec.Cmd)    {}
