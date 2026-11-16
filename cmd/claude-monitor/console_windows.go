package main

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// El .exe se compila como app GUI (-H windowsgui) para que la bandeja y los
// hooks no abran ventanas de consola. Cuando se ejecuta desde una terminal
// (install, status, doctor...), nos enganchamos a la consola del padre para
// poder imprimir. Si stdout ya es un pipe (hooks, statusline), no tocamos nada.
func attachConsole() {
	if t, err := windows.GetFileType(windows.Handle(os.Stdout.Fd())); err == nil && t != windows.FILE_TYPE_UNKNOWN {
		return
	}
	const attachParent = ^uint32(0) // ATTACH_PARENT_PROCESS
	if r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole").Call(uintptr(attachParent)); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = f
	}
}

func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
