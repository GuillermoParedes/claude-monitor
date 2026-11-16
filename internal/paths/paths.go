// Package paths centraliza las rutas que usa claude-monitor en cada plataforma.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultPort es el puerto del servidor local (solo 127.0.0.1).
const DefaultPort = 47821

// ClaudeDir es el directorio de configuracion de Claude Code
// (~/.claude, o $CLAUDE_CONFIG_DIR si esta definido).
func ClaudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// ProjectsDir contiene los transcripts .jsonl de cada sesion.
func ProjectsDir() string { return filepath.Join(ClaudeDir(), "projects") }

// SettingsFile es el settings.json de usuario de Claude Code.
func SettingsFile() string { return filepath.Join(ClaudeDir(), "settings.json") }

// DataDir guarda el estado propio de claude-monitor.
func DataDir() string {
	if d := os.Getenv("CLAUDE_MONITOR_DIR"); d != "" {
		return d
	}
	return filepath.Join(ClaudeDir(), "monitor")
}

func SessionsDir() string { return filepath.Join(DataDir(), "sessions") }
func CacheFile() string   { return filepath.Join(DataDir(), "cache.json") }
func LimitsFile() string  { return filepath.Join(DataDir(), "limits.json") }
func PricesFile() string  { return filepath.Join(DataDir(), "prices.json") }
func ConfigFile() string  { return filepath.Join(DataDir(), "config.json") }
func LogFile() string     { return filepath.Join(DataDir(), "claude-monitor.log") }

// IsWSL indica si corremos dentro de WSL (Linux sobre Windows).
func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// WSLDistro devuelve el nombre de la distro de WSL (o "").
func WSLDistro() string { return os.Getenv("WSL_DISTRO_NAME") }

// InstallDir es donde `install` copia el binario para que los hooks tengan una
// ruta estable.
func InstallDir() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "Programs", "claude-monitor")
		}
		return filepath.Join(home, "AppData", "Local", "Programs", "claude-monitor")
	}
	return filepath.Join(home, ".local", "bin")
}

// BinaryName es el nombre del ejecutable en esta plataforma.
func BinaryName() string {
	if runtime.GOOS == "windows" {
		return "claude-monitor.exe"
	}
	return "claude-monitor"
}

// EnsureDirs crea los directorios de datos.
func EnsureDirs() error {
	return os.MkdirAll(SessionsDir(), 0o700)
}
