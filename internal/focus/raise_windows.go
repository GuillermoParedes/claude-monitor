package focus

import (
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procAllowSetForeground       = user32.NewProc("AllowSetForegroundWindow")
)

var exeFor = map[string]string{
	"windows-terminal": "windowsterminal.exe",
	"vscode":           "code.exe",
	"wezterm":          "wezterm-gui.exe",
}

func processNames() map[uint32]string {
	out := map[uint32]string{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return out
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out[e.ProcessID] = strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
	}
	return out
}

// RaiseApp trae al frente la ventana principal de la app (Windows).
func raise(app string) bool {
	exe, ok := exeFor[app]
	if !ok {
		return false
	}
	names := processNames()
	var found uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		if n, _, _ := procGetWindowTextLengthW.Call(hwnd); n == 0 {
			return 1
		}
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if names[pid] == exe {
			found = hwnd
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	if found == 0 {
		return false
	}
	if ic, _, _ := procIsIconic.Call(found); ic != 0 {
		procShowWindow.Call(found, 9) // SW_RESTORE
	}
	r, _, _ := procSetForegroundWindow.Call(found)
	return r != 0
}

// AllowForeground permite que otro proceso (p. ej. el navegador del panel)
// tome el foco: Windows solo deja hacerlo a quien recibio la ultima entrada.
func AllowForeground() { procAllowSetForeground.Call(uintptr(0xFFFFFFFF)) } // ASFW_ANY

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// startInDir abre una consola nueva en dir, sin pasar el path por un shell.
func startInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010} // CREATE_NEW_CONSOLE
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
