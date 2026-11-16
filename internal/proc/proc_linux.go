package proc

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func info(pid int) (name string, ppid int, ok bool) {
	comm, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return "", 0, false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", 0, false
	}
	// /proc/<pid>/stat: "pid (comm) state ppid ..." -- comm puede tener espacios.
	s := string(stat)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return "", 0, false
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return "", 0, false
	}
	ppid, _ = strconv.Atoi(f[1])
	return strings.TrimSpace(string(comm)), ppid, true
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// Environ lee una variable del entorno de otro proceso (Linux).
func Environ(pid int, key string) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, kv := range strings.Split(string(b), "\x00") {
		if strings.HasPrefix(kv, prefix) {
			return kv[len(prefix):]
		}
	}
	return ""
}
