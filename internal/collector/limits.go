package collector

import (
	"encoding/json"
	"os"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

// RateWindow tal como lo entrega Claude Code al statusline.
type RateWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"` // epoch s
}

type RateLimits struct {
	FiveHour *RateWindow `json:"five_hour,omitempty"`
	SevenDay *RateWindow `json:"seven_day,omitempty"`
}

// LimitsFile es lo que persiste `claude-monitor statusline`.
type LimitsFile struct {
	Ts         int64      `json:"ts"` // ms
	RateLimits RateLimits `json:"rate_limits"`
}

// Window es la vista para la UI.
type Window struct {
	Pct      int    `json:"pct"`
	ResetsAt int64  `json:"resetsAt,omitempty"`
	ResetIn  string `json:"resetIn,omitempty"`
}

type Limits struct {
	FiveHour *Window `json:"fiveHour,omitempty"`
	SevenDay *Window `json:"sevenDay,omitempty"`
	AgeSec   int64   `json:"ageSec"`
}

func readLimits(now time.Time) *Limits {
	b, err := os.ReadFile(paths.LimitsFile())
	if err != nil {
		return nil
	}
	var f LimitsFile
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	win := func(w *RateWindow) *Window {
		if w == nil {
			return nil
		}
		if w.ResetsAt > 0 && time.Unix(w.ResetsAt, 0).Before(now) {
			// La ventana ya se reinicio y aun no hubo respuesta nueva: 0%.
			return &Window{Pct: 0}
		}
		out := &Window{Pct: int(w.UsedPercentage + 0.5), ResetsAt: w.ResetsAt}
		if w.ResetsAt > 0 {
			out.ResetIn = FmtDuration(time.Unix(w.ResetsAt, 0).Sub(now))
		}
		return out
	}
	l := &Limits{FiveHour: win(f.RateLimits.FiveHour), SevenDay: win(f.RateLimits.SevenDay)}
	if f.Ts > 0 {
		l.AgeSec = int64(now.Sub(time.UnixMilli(f.Ts)).Seconds())
	}
	if l.FiveHour == nil && l.SevenDay == nil {
		return nil
	}
	return l
}
