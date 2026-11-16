//go:build !windows

package main

import (
	"os/exec"
	"runtime"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

func attachConsole() {}

func openBrowser(url string) {
	switch {
	case runtime.GOOS == "darwin":
		_ = exec.Command("open", url).Start()
	case paths.IsWSL():
		cmd := exec.Command("cmd.exe", "/c", "start", "", url)
		cmd.Dir = "/mnt/c"
		_ = cmd.Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}
