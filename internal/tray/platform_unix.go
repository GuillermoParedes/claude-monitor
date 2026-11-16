//go:build !windows

package tray

import (
	"os/exec"
	"runtime"
	"strings"
)

// Navegadores con modo "--app" (ventana sin pestañas), en orden de preferencia.
var appBrowsersMac = []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium", "Arc"}
var appBrowsersLinux = []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"}

func openPanelWindow(url string) {
	args := []string{"--app=" + url, "--window-size=380,760"}
	switch runtime.GOOS {
	case "darwin":
		for _, app := range appBrowsersMac {
			if exec.Command("open", "-Ra", app).Run() == nil {
				_ = exec.Command("open", append([]string{"-na", app, "--args"}, args...)...).Start()
				return
			}
		}
		_ = exec.Command("open", url).Start()
	default:
		for _, b := range appBrowsersLinux {
			if p, err := exec.LookPath(b); err == nil {
				_ = exec.Command(p, args...).Start()
				return
			}
		}
		_ = exec.Command("xdg-open", url).Start()
	}
}

func allowForeground() {}

func openPath(p string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", p).Start()
		return
	}
	_ = exec.Command("xdg-open", p).Start()
}

func systemLang() string {
	if runtime.GOOS == "darwin" {
		// AppleLanguages: ( "es-BO", "en" ) -> el primero es el preferido.
		if out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			s := string(out)
			if i := strings.IndexByte(s, '"'); i >= 0 && strings.HasPrefix(s[i+1:], "es") {
				return "es"
			}
		}
	}
	return "en"
}

func registerNotifier() {}

func promoteIcon(done func() bool, mark func()) {}
