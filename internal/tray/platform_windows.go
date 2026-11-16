package tray

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/GuillermoParedes/claude-monitor/internal/focus"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
	procMonitorFromPoint  = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW   = user32.NewProc("GetMonitorInfoW")
	procGetUserUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

const (
	panelW = 380
	panelH = 760
)

// openPanelWindow abre el panel como ventana de app (Edge/Chrome --app),
// anclada sobre la bandeja del monitor donde esta el cursor.
func openPanelWindow(url string) {
	x, y := -1, -1
	var pt struct{ X, Y int32 }
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r != 0 {
		hmon, _, _ := procMonitorFromPoint.Call(uintptr(*(*int64)(unsafe.Pointer(&pt))), 2) // MONITOR_DEFAULTTONEAREST
		mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		if r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			w := mi.Work
			x, y = int(w.Right)-panelW-12, int(w.Bottom)-panelH-12
			if w.Top > mi.Monitor.Top { // barra de tareas arriba
				y = int(w.Top) + 12
			}
			if w.Left > mi.Monitor.Left { // barra a la izquierda
				x = int(w.Left) + 12
			}
		}
	}

	browser := findBrowser()
	if browser == "" {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
		return
	}
	profile := filepath.Join(os.Getenv("LOCALAPPDATA"), "claude-monitor", "panel-profile")
	args := []string{
		"--app=" + url,
		"--user-data-dir=" + profile,
		fmt.Sprintf("--window-size=%d,%d", panelW, panelH),
		"--no-first-run", "--no-default-browser-check", "--disable-sync",
		"--disable-features=Translate,msEdgeSidebarV2,msHubApps",
	}
	if x >= 0 {
		args = append(args, fmt.Sprintf("--window-position=%d,%d", x, y))
	}
	cmd := exec.Command(browser, args...)
	_ = cmd.Start()
	go cmd.Wait()
}

func findBrowser() string {
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func allowForeground() { focus.AllowForeground() }

func openPath(p string) { _ = exec.Command("explorer.exe", p).Start() }

func systemLang() string {
	r, _, _ := procGetUserUILanguage.Call()
	if r&0x3ff == 0x0a { // LANG_SPANISH
		return "es"
	}
	return "en"
}

// registerNotifier registra el AppUserModelID para que Windows muestre los
// toasts de una app sin instalador MSIX (HKCU, sin admin).
func registerNotifier() {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+AppID, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.SetStringValue("DisplayName", "Claude Monitor")
	icon := filepath.Join(paths.DataDir(), "icon.png")
	_ = k.SetStringValue("IconUri", icon)
	_ = k.SetStringValue("IconBackgroundColor", "FF181A20")
}

// promoteIcon saca el icono del desborde (^) de Windows 11 la PRIMERA vez que
// corre la app. Windows crea la entrada en NotifyIconSettings unos segundos
// despues de mostrar el icono, asi que reintentamos un rato. Si luego el
// usuario lo esconde, se respeta (queda marcado en la config).
func promoteIcon(done func() bool, mark func()) {
	if done() {
		return
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	for i := 0; i < 40; i++ {
		root, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\NotifyIconSettings`, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			return // Windows 10: no aplica
		}
		names, _ := root.ReadSubKeyNames(-1)
		root.Close()
		for _, n := range names {
			k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\NotifyIconSettings\`+n, registry.QUERY_VALUE|registry.SET_VALUE)
			if err != nil {
				continue
			}
			path, _, _ := k.GetStringValue("ExecutablePath")
			if path != "" && equalFoldPath(path, self) {
				_ = k.SetDWordValue("IsPromoted", 1)
				k.Close()
				mark()
				return
			}
			k.Close()
		}
		time.Sleep(3 * time.Second)
	}
}

func equalFoldPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
