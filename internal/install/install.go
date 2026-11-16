// Package install configura claude-monitor en el sistema:
//
//  1. Copia el binario a una ruta estable (los hooks la referencian).
//  2. Agrega hooks y statusLine a ~/.claude/settings.json (con backup).
//  3. Registra el arranque automatico (Run key / LaunchAgent / autostart / systemd).
//  4. En WSL, instala ademas la bandeja de Windows en modo --connect.
package install

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/collector"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

// ReleaseURL es de donde WSL descarga la bandeja de Windows si no se le pasa una.
const ReleaseURL = "https://github.com/GuillermoParedes/claude-monitor/releases/latest/download/claude-monitor_windows_amd64.exe"

type Options struct {
	WindowsExe string // WSL: ruta local al .exe de Windows (si no, se descarga)
	NoStart    bool
	NoWindows  bool // WSL: no instalar la bandeja de Windows
	Purge      bool // uninstall: borrar tambien los datos
	Out        io.Writer
}

func (o Options) say(f string, a ...any) { fmt.Fprintf(o.Out, f+"\n", a...) }

// Install hace todo el proceso. Es idempotente.
func Install(o Options) error {
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)
	bin := filepath.Join(paths.InstallDir(), paths.BinaryName())

	stopRunning()
	if !samePath(self, bin) {
		if err := copyFile(self, bin); err != nil {
			return fmt.Errorf("no pude copiar el binario a %s: %w", bin, err)
		}
		o.say("✓ binario    %s", bin)
	} else {
		o.say("✓ binario    %s (ya instalado)", bin)
	}

	cfg := collector.LoadConfig()
	wrapped, err := applySettings(paths.SettingsFile(), bin, true, cfg.WrapStatusLine)
	if err != nil {
		return err
	}
	if wrapped != cfg.WrapStatusLine {
		cfg.WrapStatusLine = wrapped
		if err := collector.SaveConfig(cfg); err != nil {
			return err
		}
	}
	o.say("✓ hooks      %s (backup: settings.json.claude-monitor.bak)", paths.SettingsFile())
	if wrapped != "" {
		o.say("✓ statusLine se mantiene el tuyo (%s) y se le agregan los limites del plan", wrapped)
	} else {
		o.say("✓ statusLine instalado (muestra modelo, contexto y limites del plan)")
	}

	if err := enableAutostart(bin, o); err != nil {
		return err
	}

	if paths.IsWSL() && !o.NoWindows {
		if err := installWindowsTray(o); err != nil {
			o.say("! bandeja de Windows: %v", err)
			o.say("  puedes instalarla luego con: claude-monitor install --windows-exe <ruta al .exe>")
		}
	}

	if !o.NoStart {
		if err := waitHealthy(10 * time.Second); err != nil {
			o.say("! el colector no respondio todavia: %v (revisa: claude-monitor doctor)", err)
		} else {
			o.say("✓ corriendo  http://127.0.0.1:%d", collector.LoadConfig().PortOrDefault())
		}
	}
	o.say("\nListo. Las sesiones de Claude Code abiertas antes de instalar aparecen al usar su siguiente herramienta.")
	return nil
}

// Uninstall revierte Install.
func Uninstall(o Options) error {
	cfg := collector.LoadConfig()
	bin := filepath.Join(paths.InstallDir(), paths.BinaryName())
	if _, err := applySettings(paths.SettingsFile(), bin, false, cfg.WrapStatusLine); err != nil {
		return err
	}
	o.say("✓ hooks y statusLine retirados de %s", paths.SettingsFile())
	disableAutostart(o)
	stopRunning()
	if paths.IsWSL() {
		uninstallWindowsTray(o)
	}
	if o.Purge {
		_ = os.RemoveAll(paths.DataDir())
		o.say("✓ datos borrados (%s)", paths.DataDir())
	} else {
		o.say("  datos conservados en %s (usa --purge para borrarlos)", paths.DataDir())
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(bin)
		o.say("✓ binario borrado (%s)", bin)
	} else {
		o.say("  borra la carpeta %s para terminar", paths.InstallDir())
	}
	return nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return equalFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 32
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// En Windows no se puede reemplazar un .exe en uso: lo apartamos primero.
	if runtime.GOOS == "windows" {
		_ = os.Remove(dst + ".old")
		_ = os.Rename(dst, dst+".old")
	}
	return os.Rename(tmp, dst)
}

func healthURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/api/health", collector.LoadConfig().PortOrDefault())
}

func waitHealthy(timeout time.Duration) error {
	c := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		resp, err := c.Get(healthURL())
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		last = err
		time.Sleep(300 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}

// Running indica si hay una instancia sirviendo en el puerto configurado.
func Running() bool {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get(healthURL())
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}
