package install

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/collector"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

const unitName = "claude-monitor.service"

func home() string { h, _ := os.UserHomeDir(); return h }

func unitPath() string { return filepath.Join(home(), ".config", "systemd", "user", unitName) }

func desktopPath() string {
	return filepath.Join(home(), ".config", "autostart", "claude-monitor.desktop")
}

// Headless: WSL (la bandeja vive en Windows) o Linux sin sesion grafica.
func Headless() bool {
	return paths.IsWSL() || (os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "")
}

func hasSystemd() bool {
	return exec.Command("systemctl", "--user", "show-environment").Run() == nil
}

func enableAutostart(bin string, o Options) error {
	if Headless() {
		if !hasSystemd() {
			o.say("! no hay systemd de usuario: arranca a mano con `claude-monitor run --headless &`")
			if !o.NoStart {
				return startDetached(bin, "run", "--headless")
			}
			return nil
		}
		unit := fmt.Sprintf(`[Unit]
Description=Claude Monitor (collector + local server)

[Service]
ExecStart=%s run --headless
Restart=on-failure
RestartSec=5
Nice=10

[Install]
WantedBy=default.target
`, bin)
		if err := os.MkdirAll(filepath.Dir(unitPath()), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(unitPath(), []byte(unit), 0o644); err != nil {
			return err
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		args := []string{"--user", "enable", unitName}
		if !o.NoStart {
			args = []string{"--user", "enable", "--now", unitName}
		}
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl: %v: %s", err, out)
		}
		if !o.NoStart {
			_ = exec.Command("systemctl", "--user", "restart", unitName).Run()
		}
		o.say("✓ inicio     servicio systemd de usuario (%s)", unitName)
		return nil
	}

	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Claude Monitor
Comment=Claude Code usage, sessions and plan limits in the tray
Exec=%s run
X-GNOME-Autostart-enabled=true
NoDisplay=false
`, bin)
	if err := os.MkdirAll(filepath.Dir(desktopPath()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(desktopPath(), []byte(desktop), 0o644); err != nil {
		return err
	}
	o.say("✓ inicio     %s", desktopPath())
	if !o.NoStart {
		return startDetached(bin, "run")
	}
	return nil
}

func startDetached(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if f, err := os.OpenFile(paths.LogFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func disableAutostart(o Options) {
	if _, err := os.Stat(unitPath()); err == nil {
		_ = exec.Command("systemctl", "--user", "disable", "--now", unitName).Run()
		_ = os.Remove(unitPath())
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		o.say("✓ servicio systemd retirado")
	}
	if _, err := os.Stat(desktopPath()); err == nil {
		_ = os.Remove(desktopPath())
		o.say("✓ autostart retirado")
	}
}

func stopRunning() {
	_ = exec.Command("systemctl", "--user", "stop", unitName).Run()
	self := os.Getpid()
	out, _ := exec.Command("pgrep", "-f", "claude-monitor (run|tray)").Output()
	for _, f := range strings.Fields(string(out)) {
		var pid int
		fmt.Sscan(f, &pid)
		if pid > 0 && pid != self {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	time.Sleep(300 * time.Millisecond)
}

// --- WSL: bandeja en el lado Windows -----------------------------------------

// winEnv devuelve una variable de entorno de Windows (via cmd.exe).
func winEnv(name string) (string, error) {
	cmd := exec.Command("cmd.exe", "/c", "echo %"+name+"%")
	cmd.Dir = "/mnt/c"
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(out))
	if v == "" || v == "%"+name+"%" {
		return "", fmt.Errorf("%s no definido", name)
	}
	return v, nil
}

func wslpath(flag, p string) (string, error) {
	out, err := exec.Command("wslpath", flag, p).Output()
	return strings.TrimSpace(string(out)), err
}

func runWin(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = "/mnt/c"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func windowsTrayPaths() (winExe, linuxExe string, err error) {
	la, err := winEnv("LOCALAPPDATA")
	if err != nil {
		return "", "", err
	}
	winExe = la + `\Programs\claude-monitor\claude-monitor.exe`
	linuxExe, err = wslpath("-u", winExe)
	return winExe, linuxExe, err
}

func installWindowsTray(o Options) error {
	winExe, linuxExe, err := windowsTrayPaths()
	if err != nil {
		return err
	}
	_ = runWin("taskkill.exe", "/IM", "claude-monitor.exe", "/F")
	time.Sleep(500 * time.Millisecond)

	if o.WindowsExe != "" {
		if err := copyFile(o.WindowsExe, linuxExe); err != nil {
			return err
		}
	} else if err := download(ReleaseURL, linuxExe); err != nil {
		return fmt.Errorf("descargando la bandeja de Windows: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d", collector.LoadConfig().PortOrDefault())
	runValue := fmt.Sprintf(`"%s" tray --connect %s`, winExe, url)
	if err := runWin("reg.exe", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		"/v", "ClaudeMonitor", "/t", "REG_SZ", "/d", runValue, "/f"); err != nil {
		return err
	}
	o.say("✓ Windows    bandeja en %s (arranca con Windows)", winExe)
	if !o.NoStart {
		if err := runWin("powershell.exe", "-NoProfile", "-Command",
			fmt.Sprintf(`Start-Process -FilePath '%s' -ArgumentList 'tray','--connect','%s'`, winExe, url)); err != nil {
			return err
		}
	}
	return nil
}

func uninstallWindowsTray(o Options) {
	_ = runWin("taskkill.exe", "/IM", "claude-monitor.exe", "/F")
	_ = runWin("reg.exe", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "ClaudeMonitor", "/f")
	if _, linuxExe, err := windowsTrayPaths(); err == nil {
		time.Sleep(500 * time.Millisecond)
		_ = os.Remove(linuxExe)
		_ = os.Remove(linuxExe + ".old")
	}
	o.say("✓ bandeja de Windows retirada")
}

func download(url, dst string) error {
	c := &http.Client{Timeout: 2 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".download"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	_ = os.Remove(dst + ".old")
	_ = os.Rename(dst, dst+".old")
	return os.Rename(tmp, dst)
}
