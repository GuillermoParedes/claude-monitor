package collector

import (
	"encoding/json"
	"os"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

// Config es la configuracion persistente (<DataDir>/config.json).
type Config struct {
	// Port del servidor local. 0 => paths.DefaultPort.
	Port int `json:"port,omitempty"`
	// WrapStatusLine es el statusLine que el usuario tenia antes de instalar;
	// lo seguimos ejecutando para que su barra no cambie.
	WrapStatusLine string `json:"wrapStatusLine,omitempty"`
	// Notifications: nil => activadas.
	Notifications *bool `json:"notifications,omitempty"`
	// Lang fuerza el idioma de la bandeja ("en", "es"). Vacio => del sistema.
	Lang string `json:"lang,omitempty"`
	// TrayPromoted: el icono ya se saco del desborde de Windows 11 una vez.
	TrayPromoted bool `json:"trayPromoted,omitempty"`
}

func (c Config) PortOrDefault() int {
	if c.Port > 0 {
		return c.Port
	}
	return paths.DefaultPort
}

func (c Config) NotificationsOn() bool { return c.Notifications == nil || *c.Notifications }

func LoadConfig() Config {
	var c Config
	if b, err := os.ReadFile(paths.ConfigFile()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func SaveConfig(c Config) error {
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(paths.ConfigFile(), append(b, '\n'), 0o600)
}
