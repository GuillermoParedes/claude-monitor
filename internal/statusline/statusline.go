// Package statusline implementa `claude-monitor statusline`.
//
// Claude Code le pasa por stdin un JSON que, para cuentas de suscripcion,
// incluye `rate_limits` (ventana de 5h y semanal): es la UNICA fuente local de
// esos porcentajes. Los guardamos en limits.json para el colector.
//
// Si el usuario ya tenia un statusLine propio, `install` lo guarda en la config
// y aqui lo ejecutamos con el mismo stdin: su barra se ve igual que siempre.
package statusline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/collector"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

type input struct {
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
	RateLimits *collector.RateLimits `json:"rate_limits"`
}

// Run lee el JSON, persiste los limites e imprime la linea de estado.
func Run(stdin io.Reader, stdout io.Writer) {
	data, _ := io.ReadAll(io.LimitReader(stdin, 4<<20))
	var in input
	_ = json.Unmarshal(data, &in)

	if in.RateLimits != nil && (in.RateLimits.FiveHour != nil || in.RateLimits.SevenDay != nil) {
		_ = paths.EnsureDirs()
		b, _ := json.Marshal(collector.LimitsFile{Ts: time.Now().UnixMilli(), RateLimits: *in.RateLimits})
		tmp := paths.LimitsFile() + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			_ = os.Rename(tmp, paths.LimitsFile())
		}
	}

	if wrapped := collector.LoadConfig().WrapStatusLine; wrapped != "" {
		if out, ok := runWrapped(wrapped, data); ok {
			_, _ = stdout.Write(out)
			return
		}
	}
	fmt.Fprint(stdout, line(in))
}

func line(in input) string {
	parts := []string{collector.ShortModel(in.Model.ID)}
	if p := in.ContextWindow.UsedPercentage; p != nil {
		parts = append(parts, fmt.Sprintf("ctx %d%%", int(*p)))
	}
	if rl := in.RateLimits; rl != nil {
		if w := rl.FiveHour; w != nil {
			s := fmt.Sprintf("5h %d%%", int(w.UsedPercentage))
			if w.ResetsAt > 0 {
				s += " (" + collector.FmtDuration(time.Until(time.Unix(w.ResetsAt, 0))) + ")"
			}
			parts = append(parts, s)
		}
		if w := rl.SevenDay; w != nil {
			parts = append(parts, fmt.Sprintf("sem %d%%", int(w.UsedPercentage)))
		}
	}
	return strings.Join(parts, " · ")
}

func runWrapped(command string, stdin []byte) ([]byte, bool) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return out, err == nil || len(out) > 0
}
