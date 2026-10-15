package localipc

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestChildEndsWithParent(t *testing.T) {
	switch os.Getenv("LOCALIPC_TEST_ROLE") {
	case "child":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=TestChildEndsWithParent")
		child.Env = append(os.Environ(), "LOCALIPC_TEST_ROLE=child")
		PrepareChild(child)
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		BindChild(child)
		os.Stdout.WriteString(strconv.Itoa(child.Process.Pid) + "\n")
		os.Exit(0)
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("no parent-death binding on this system")
	}
	parent := exec.Command(os.Args[0], "-test.run=TestChildEndsWithParent")
	parent.Env = append(os.Environ(), "LOCALIPC_TEST_ROLE=parent")
	out, err := parent.Output()
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parent output %q", out)
	}
	deadline := time.Now().Add(5 * time.Second)
	for running(pid) {
		if time.Now().After(deadline) {
			t.Fatal("child outlived its parent")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
