//go:build !windows

package focus

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

// En WSL la app de terminal vive en Windows: la traemos al frente con
// PowerShell via interop (no hay otra forma desde Linux).
const wslRaiseScript = `
Add-Type -Namespace W -Name U -MemberDefinition '
[System.Runtime.InteropServices.DllImport("user32.dll")] public static extern bool SetForegroundWindow(System.IntPtr h);
[System.Runtime.InteropServices.DllImport("user32.dll")] public static extern bool ShowWindowAsync(System.IntPtr h, int c);
[System.Runtime.InteropServices.DllImport("user32.dll")] public static extern bool IsIconic(System.IntPtr h);'
$p = Get-Process -Name %s -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if ($p) { if ([W.U]::IsIconic($p.MainWindowHandle)) { [W.U]::ShowWindowAsync($p.MainWindowHandle, 9) | Out-Null }; [W.U]::SetForegroundWindow($p.MainWindowHandle) | Out-Null }
`

var wslProcess = map[string]string{
	"windows-terminal": "WindowsTerminal",
	"vscode":           "Code",
}

var macApp = map[string]string{
	"iterm":    "iTerm",
	"terminal": "Terminal",
	"wezterm":  "WezTerm",
	"ghostty":  "Ghostty",
	"vscode":   "Visual Studio Code",
}

func raise(app string) bool {
	switch {
	case paths.IsWSL():
		name, ok := wslProcess[app]
		if !ok {
			return false
		}
		script := sprintf(wslRaiseScript, name)
		return startDetached("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script) == nil
	case runtime.GOOS == "darwin":
		name, ok := macApp[app]
		if !ok {
			return false
		}
		return exec.Command("osascript", "-e", `tell application "`+name+`" to activate`).Run() == nil
	}
	return false // Linux de escritorio: depende del WM; no lo intentamos
}

func AllowForeground() {}

func hideWindow(cmd *exec.Cmd) {}

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

func startInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
