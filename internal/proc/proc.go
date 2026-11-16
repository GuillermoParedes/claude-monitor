// Package proc expone lo minimo de procesos que necesitamos de forma portable:
// saber si un pid sigue vivo y encontrar el proceso `claude` que lanzo un hook.
package proc

import (
	"os"
	"strings"
)

// FindClaudeAncestor sube por la cadena de padres desde el proceso actual y
// devuelve el pid del primer ancestro que parezca Claude Code (0 si no hay).
func FindClaudeAncestor() int {
	pid := os.Getppid()
	for i := 0; i < 8 && pid > 1; i++ {
		name, ppid, ok := info(pid)
		if !ok {
			return 0
		}
		if isClaude(name) {
			return pid
		}
		pid = ppid
	}
	return 0
}

func isClaude(name string) bool {
	n := strings.ToLower(name)
	n = strings.TrimSuffix(n, ".exe")
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	return n == "claude"
}

// Alive indica si el pid sigue existiendo. pid 0 => desconocido => true.
func Alive(pid int) bool {
	if pid <= 0 {
		return true
	}
	return alive(pid)
}
