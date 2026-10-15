//go:build !windows && !linux

package localipc

func zombie(int) bool { return false }
