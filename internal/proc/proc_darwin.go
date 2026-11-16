package proc

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func info(pid int) (name string, ppid int, ok bool) {
	out, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", 0, false
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) < 2 {
		return "", 0, false
	}
	ppid, _ = strconv.Atoi(f[0])
	return strings.Join(f[1:], " "), ppid, true
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// Environ no esta disponible en macOS sin privilegios.
func Environ(pid int, key string) string { return "" }
