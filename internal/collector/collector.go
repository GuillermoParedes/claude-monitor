// Package collector lee el estado de Claude Code (transcripts, registros de
// los hooks, limites del plan) y lo expone como un State listo para la UI.
package collector

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

// Version la fija main en tiempo de compilacion.
var Version = "dev"

type Named struct {
	Name   string  `json:"name"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
}

type Day struct {
	Date    string  `json:"date"`
	Weekday int     `json:"weekday"` // 0 = domingo
	Tokens  int64   `json:"tokens"`
	Cost    float64 `json:"cost"`
}

type Perf struct {
	Available  bool    `json:"available"`
	Label      string  `json:"label"`
	CPU        int     `json:"cpu"`
	RAMUsedGB  float64 `json:"ramUsedGB"`
	RAMTotalGB float64 `json:"ramTotalGB"`
	RAMPct     int     `json:"ramPct"`
	IOPressure int     `json:"ioPressure"`
}

type Host struct {
	OS     string `json:"os"`
	WSL    bool   `json:"wsl"`
	Distro string `json:"distro,omitempty"`
}

// State es la foto completa que consumen la bandeja, el panel y Rainmeter.
type State struct {
	Status      string        `json:"status"` // working | waiting | idle | off
	Active      int           `json:"active"`
	Working     int           `json:"working"`
	TodayTokens int64         `json:"todayTokens"`
	TodayCost   float64       `json:"todayCost"`
	WeekTokens  int64         `json:"weekTokens"`
	WeekCost    float64       `json:"weekCost"`
	Hours       []float64     `json:"hours"` // costo por hora de hoy
	HourNow     int           `json:"hourNow"`
	Days        []Day         `json:"days"` // ultimos 7, del mas viejo a hoy
	Projects    []Named       `json:"projects"`
	Models      []Named       `json:"models"`
	Limits      *Limits       `json:"limits"`
	Sessions    []SessionView `json:"sessions"`
	Perf        Perf          `json:"perf"`
	Host        Host          `json:"host"`
	Updated     int64         `json:"updated"` // epoch s
	Version     string        `json:"version"`
}

// Collector refresca el State periodicamente.
type Collector struct {
	mu     sync.RWMutex
	state  *State
	cache  *usageCache
	prices priceTable
	cpu    cpuSampler
}

func New() *Collector {
	_ = paths.EnsureDirs()
	c := &Collector{cache: loadCache()}
	c.prices.reload()
	return c
}

// State devuelve la ultima foto (nil si aun no hay).
func (c *Collector) State() *State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Run refresca cada `every` hasta que stop se cierre.
func (c *Collector) Run(every time.Duration, stop <-chan struct{}) {
	c.safeTick()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			c.cache.save()
			return
		case <-t.C:
			c.safeTick()
		}
	}
}

func (c *Collector) safeTick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("collector: panic: %v", r)
		}
	}()
	s := c.Tick()
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
}

// Tick hace una pasada completa y devuelve el State.
func (c *Collector) Tick() *State {
	if c.prices.reload() {
		c.cache.reset() // cambiaron los precios: recalcular historial
	}
	c.cache.update(&c.prices)

	now := time.Now()
	today := c.cache.day(now)
	s := &State{
		Hours:   make([]float64, 24),
		HourNow: now.Hour(),
		Updated: now.Unix(),
		Version: Version,
		Host:    Host{OS: runtime.GOOS, WSL: paths.IsWSL(), Distro: paths.WSLDistro()},
	}
	s.TodayTokens, s.TodayCost = today.Tokens, round2(today.Cost)
	for i, v := range today.Hours {
		s.Hours[i] = round3(v)
	}
	for i := 6; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		a := c.cache.day(d)
		s.Days = append(s.Days, Day{Date: d.Format("2006-01-02"), Weekday: int(d.Weekday()), Tokens: a.Tokens, Cost: round2(a.Cost)})
		s.WeekTokens += a.Tokens
		s.WeekCost += a.Cost
	}
	s.WeekCost = round2(s.WeekCost)
	s.Projects = top(today.ByProject, 3)
	s.Models = top(today.ByModel, 3)
	s.Limits = readLimits(now)
	s.Sessions = readSessions(today, now)
	s.Perf = c.cpu.sample()

	waiting := false
	for _, x := range s.Sessions {
		if x.Flag != "idle" {
			s.Working++
		}
		if x.Flag == "waiting" {
			waiting = true
		}
	}
	s.Active = len(s.Sessions)
	switch {
	case waiting:
		s.Status = "waiting"
	case s.Working > 0:
		s.Status = "working"
	case s.Active > 0:
		s.Status = "idle"
	default:
		s.Status = "off"
	}
	return s
}

// Text serializa el State como KEY=value (formato del skin de Rainmeter).
func (s *State) Text() string {
	var b strings.Builder
	put := func(k string, v any) {
		val := strings.NewReplacer("\r", " ", "\n", " ").Replace(fmt.Sprint(v))
		fmt.Fprintf(&b, "%s=%s\n", k, val)
	}
	put("STATUS", s.Status)
	put("ACTIVE", s.Active)
	put("WORKING", s.Working)
	put("TOK_TODAY", FmtTokens(s.TodayTokens))
	put("COST_TODAY", fmt.Sprintf("%.2f", s.TodayCost))
	put("TOK_WEEK", FmtTokens(s.WeekTokens))
	put("COST_WEEK", fmt.Sprintf("%.2f", s.WeekCost))
	hours := make([]string, len(s.Hours))
	for i, h := range s.Hours {
		hours[i] = fmt.Sprintf("%.3f", h)
	}
	put("HOURS", strings.Join(hours, ","))
	put("HOUR_NOW", s.HourNow)
	days, labels := make([]string, len(s.Days)), make([]string, len(s.Days))
	for i, d := range s.Days {
		days[i] = fmt.Sprintf("%.2f", d.Cost)
		labels[i] = string("SMTWTFS"[d.Weekday])
	}
	put("DAYS", strings.Join(days, ","))
	put("DAY_LABELS", strings.Join(labels, ","))
	for i := 0; i < 3; i++ {
		n, c := "", ""
		if i < len(s.Projects) {
			n, c = s.Projects[i].Name, fmt.Sprintf("%.2f", s.Projects[i].Cost)
		}
		put(fmt.Sprintf("P%d_NAME", i+1), n)
		put(fmt.Sprintf("P%d_COST", i+1), c)
		n, c = "", ""
		if i < len(s.Models) {
			n, c = s.Models[i].Name, fmt.Sprintf("%.2f", s.Models[i].Cost)
		}
		put(fmt.Sprintf("M%d_NAME", i+1), n)
		put(fmt.Sprintf("M%d_COST", i+1), c)
	}
	l5, l5r, l7, l7r, age := -1, "", -1, "", int64(-1)
	if s.Limits != nil {
		if w := s.Limits.FiveHour; w != nil {
			l5, l5r = w.Pct, w.ResetIn
		}
		if w := s.Limits.SevenDay; w != nil {
			l7, l7r = w.Pct, w.ResetIn
		}
		age = s.Limits.AgeSec
	}
	put("LIM5_PCT", l5)
	put("LIM5_RESET", l5r)
	put("LIM7_PCT", l7)
	put("LIM7_RESET", l7r)
	put("LIM_AGE", age)
	put("WSL_CPU", s.Perf.CPU)
	put("WSL_RAM", fmt.Sprintf("%.1f", s.Perf.RAMUsedGB))
	put("WSL_RAMPCT", s.Perf.RAMPct)
	put("WSL_IO", s.Perf.IOPressure)
	for i := 0; i < 4; i++ {
		var x SessionView
		if i < len(s.Sessions) {
			x = s.Sessions[i]
		}
		k := fmt.Sprintf("S%d_", i+1)
		label := x.State
		if x.Tool != "" {
			label = x.Tool
		}
		if x.Flag == "waiting" {
			label = "PERMISSION"
		}
		tok, cost := "", ""
		if x.ID != "" {
			tok, cost = FmtTokens(x.Tokens), fmt.Sprintf("%.2f", x.Cost)
		}
		put(k+"ID", x.ID)
		put(k+"NAME", x.Name)
		put(k+"STATE", label)
		put(k+"FLAG", x.Flag)
		put(k+"CTX", x.Ctx)
		put(k+"MODEL", x.Model)
		put(k+"TOK", tok)
		put(k+"COST", cost)
		put(k+"CWD", x.Cwd)
		put(k+"UNC", x.Unc)
	}
	more := len(s.Sessions) - 4
	if more < 0 {
		more = 0
	}
	put("MORE", more)
	put("DISTRO", s.Host.Distro)
	put("UPDATED", time.Unix(s.Updated, 0).Format("15:04:05"))
	put("UPDATED_TS", s.Updated)
	put("END", 1)
	return b.String()
}

// LogTo redirige el log estandar a un archivo (para procesos sin consola).
func LogTo(file string) {
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err == nil {
		log.SetOutput(f)
	}
}
