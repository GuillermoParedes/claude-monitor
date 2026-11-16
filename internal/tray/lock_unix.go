//go:build !windows

package tray

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

var lockFile *os.File

// singleInstance devuelve false si ya hay otra bandeja corriendo (flock).
func singleInstance() bool {
	_ = paths.EnsureDirs()
	f, err := os.OpenFile(filepath.Join(paths.DataDir(), "tray.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true // sin lock posible: mejor arrancar que no mostrar nada
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	lockFile = f // el lock dura mientras el proceso tenga el archivo abierto
	return true
}
