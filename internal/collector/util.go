package collector

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

var (
	reDate    = regexp.MustCompile(`-\d{8}$`)
	reVersion = regexp.MustCompile(`-(\d+)-(\d+)$`)
)

// ShortModel: "claude-opus-5-5-20261001" -> "opus-5.5".
func ShortModel(id string) string {
	m := strings.ToLower(strings.TrimSpace(id))
	if m == "" {
		return "?"
	}
	m = strings.TrimPrefix(m, "claude-")
	m = reDate.ReplaceAllString(m, "")
	m = reVersion.ReplaceAllString(m, "-$1.$2")
	return m
}

// FmtDuration: 2h05, 45m, 3d 4h.
func FmtDuration(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	m := int(math.Round(d.Minutes()))
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	h := m / 60
	if h < 24 {
		return fmt.Sprintf("%dh%02d", h, m%60)
	}
	return fmt.Sprintf("%dd %dh", h/24, h%24)
}

// FmtTokens: 1.2M, 340k.
func FmtTokens(n int64) string {
	f := float64(n)
	switch {
	case n == 0:
		return "0"
	case f >= 1e9:
		return fmt.Sprintf("%.2fG", f/1e9)
	case f >= 1e7:
		return fmt.Sprintf("%.1fM", f/1e6)
	case f >= 1e6:
		return fmt.Sprintf("%.2fM", f/1e6)
	case f >= 1e3:
		return fmt.Sprintf("%dk", int64(math.Round(f/1e3)))
	}
	return fmt.Sprint(n)
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

func contextLimit(model string) int64 {
	if strings.Contains(strings.ToLower(model), "haiku") {
		return 200_000
	}
	return 1_000_000
}
