package install

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func enableAutostart(bin string, o Options) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue("ClaudeMonitor", `"`+bin+`" run`); err != nil {
		return err
	}
	o.say("✓ inicio     se abre al iniciar sesion (HKCU\\...\\Run\\ClaudeMonitor)")
	if !o.NoStart {
		cmd := exec.Command(bin, "run")
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | NEW_PROCESS_GROUP
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
	}
	return nil
}

func disableAutostart(o Options) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue("ClaudeMonitor")
		k.Close()
	}
	o.say("✓ inicio automatico retirado")
}

// stopRunning cierra otras instancias de claude-monitor.exe (no a si mismo).
func stopRunning() {
	self := uint32(os.Getpid())
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		if name != "claude-monitor.exe" || e.ProcessID == self {
			continue
		}
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, e.ProcessID); err == nil {
			_ = windows.TerminateProcess(h, 0)
			_, _ = windows.WaitForSingleObject(h, 3000)
			windows.CloseHandle(h)
		}
	}
}

func installWindowsTray(o Options) error { return nil }
func uninstallWindowsTray(o Options)     {}

// Headless solo aplica a Linux/WSL.
func Headless() bool { return false }
