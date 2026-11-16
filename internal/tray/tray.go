// Package tray es el icono de bandeja / barra de menu.
//
//	Clic izquierdo -> panel flotante (toggle)
//	Clic derecho   -> menu (resumen, sesiones, notificaciones, salir)
//
// Siempre habla con el servidor por HTTP, asi funciona igual en modo local
// (todo en un proceso) y en modo --connect (bandeja en Windows, colector en WSL).
package tray

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/gen2brain/beeep"

	"github.com/GuillermoParedes/claude-monitor/internal/collector"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

const (
	maxSessionItems = 8
	staleAfter      = 30 * time.Second
	AppID           = "ClaudeMonitor.Tray"
)

type sessionItem struct {
	root, goTo, folder, term *systray.MenuItem
	id                       string
}

type Tray struct {
	base   string // http://127.0.0.1:47821
	onQuit func()
	client *http.Client
	tr     strs

	mu                                 sync.Mutex
	last                               *collector.State
	iconSig                            string
	miPanel, miSummary, miPlan, miCost *systray.MenuItem
	miSessions                         *systray.MenuItem
	miNotify, miQuit, miData           *systray.MenuItem
	miWidget                           *systray.MenuItem
	panelOpened                        time.Time // ultima vez que lanzamos la ventana
	sess                               [maxSessionItems]*sessionItem
	notifyOn                           bool
	iconFile                           string

	// estado de notificaciones
	first      bool
	prevFlag   map[string]string
	workStart  map[string]time.Time
	ctxWarned  map[string]bool
	lim5, lim7 int
	staleSent  bool
}

// Run bloquea hasta que el usuario elige Salir.
func Run(base string, onQuit func()) {
	if !singleInstance() {
		// Ya hay un icono: no mostramos un segundo. Si nos lanzaron a mano,
		// lo util es abrir el panel de la instancia existente.
		log.Printf("tray: another instance is running; opening its panel instead")
		OpenPanel(base)
		if onQuit != nil {
			onQuit()
		}
		return
	}
	cfg := collector.LoadConfig()
	t := &Tray{
		base: strings.TrimRight(base, "/"), onQuit: onQuit,
		client:   &http.Client{Timeout: 4 * time.Second},
		tr:       lang(cfg.Lang),
		notifyOn: cfg.NotificationsOn(),
		first:    true,
		prevFlag: map[string]string{}, workStart: map[string]time.Time{}, ctxWarned: map[string]bool{},
	}
	beeep.AppName = AppID
	if runtime.GOOS != "windows" {
		beeep.AppName = "Claude Monitor"
	}
	systray.Run(t.onReady, func() {
		if t.onQuit != nil {
			t.onQuit()
		}
	})
}

func (t *Tray) onReady() {
	t.setIcon("off", -1, 0)
	systray.SetTitle("")
	systray.SetTooltip("Claude Monitor")
	registerNotifier()

	// El icono de notificacion necesita un archivo.
	t.iconFile = filepath.Join(paths.DataDir(), "icon.png")
	_ = os.WriteFile(t.iconFile, renderIcon("working", 40, 0), 0o644)

	t.miPanel = systray.AddMenuItem(t.tr.openPanel, "")
	systray.AddSeparator()
	t.miSummary = systray.AddMenuItem("Claude", "")
	t.miSummary.Disable()
	t.miPlan = systray.AddMenuItem("", "")
	t.miPlan.Disable()
	t.miCost = systray.AddMenuItem("", "")
	t.miCost.Disable()
	systray.AddSeparator()
	t.miSessions = systray.AddMenuItem(t.tr.sessions, "")
	for i := range t.sess {
		root := t.miSessions.AddSubMenuItem("", "")
		it := &sessionItem{
			root:   root,
			goTo:   root.AddSubMenuItem(t.tr.goTo, ""),
			folder: root.AddSubMenuItem(t.tr.folder, ""),
			term:   root.AddSubMenuItem(t.tr.terminal, ""),
		}
		root.Hide()
		t.sess[i] = it
		go t.sessionLoop(it)
	}
	systray.AddSeparator()
	if hasDesktopWidget() {
		t.miWidget = systray.AddMenuItem(t.tr.widget, "")
	} else {
		t.miWidget = &systray.MenuItem{ClickedCh: make(chan struct{})}
	}
	t.miNotify = systray.AddMenuItemCheckbox(t.tr.notifications, "", t.notifyOn)
	t.miData = systray.AddMenuItem(t.tr.dataFolder, "")
	systray.AddSeparator()
	t.miQuit = systray.AddMenuItem(t.tr.quit, "")

	systray.SetOnTapped(func() { go t.togglePanel() })
	if runtime.GOOS == "darwin" {
		// En macOS el clic izquierdo abre el menu (convencion de la barra de menu);
		// "Abrir panel" queda como primera opcion.
		systray.SetOnTapped(nil)
	}

	go t.menuLoop()
	go t.pollLoop()
	go promoteIcon(
		func() bool { return collector.LoadConfig().TrayPromoted },
		func() { c := collector.LoadConfig(); c.TrayPromoted = true; _ = collector.SaveConfig(c) },
	)
}

func (t *Tray) menuLoop() {
	for {
		select {
		case <-t.miPanel.ClickedCh:
			go t.openPanel()
		case <-t.miNotify.ClickedCh:
			t.mu.Lock()
			t.notifyOn = !t.notifyOn
			on := t.notifyOn
			t.mu.Unlock()
			if on {
				t.miNotify.Check()
			} else {
				t.miNotify.Uncheck()
			}
			cfg := collector.LoadConfig()
			cfg.Notifications = &on
			_ = collector.SaveConfig(cfg)
		case <-t.miWidget.ClickedCh:
			toggleDesktopWidget()
		case <-t.miData.ClickedCh:
			openPath(paths.DataDir())
		case <-t.miQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (t *Tray) sessionLoop(it *sessionItem) {
	for {
		select {
		case <-it.goTo.ClickedCh:
			t.post("/api/focus/" + t.idOf(it))
		case <-it.root.ClickedCh:
			t.post("/api/focus/" + t.idOf(it))
		case <-it.folder.ClickedCh:
			t.post("/api/folder/" + t.idOf(it))
		case <-it.term.ClickedCh:
			t.post("/api/terminal/" + t.idOf(it))
		}
	}
}

func (t *Tray) idOf(it *sessionItem) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return it.id
}

func (t *Tray) post(path string) {
	allowForeground()
	req, _ := http.NewRequest(http.MethodPost, t.base+path, nil)
	req.Header.Set("X-Claude-Monitor", "1")
	if resp, err := t.client.Do(req); err == nil {
		resp.Body.Close()
	}
}

// --- panel -------------------------------------------------------------------

func (t *Tray) togglePanel() {
	t.mu.Lock()
	opening := time.Since(t.panelOpened) < 4*time.Second
	t.mu.Unlock()
	var seen struct {
		Ms int64 `json:"ms"`
	}
	if resp, err := t.client.Get(t.base + "/api/panel/seen"); err == nil {
		_ = json.NewDecoder(resp.Body).Decode(&seen)
		resp.Body.Close()
		if seen.Ms >= 0 && seen.Ms < 2500 {
			t.post("/api/panel/close") // esta abierto: el clic lo cierra
			return
		}
	}
	if opening {
		return // la ventana se esta abriendo (aun no carga): no abrir otra
	}
	t.openPanel()
}

func (t *Tray) openPanel() {
	t.mu.Lock()
	if time.Since(t.panelOpened) < 4*time.Second {
		t.mu.Unlock()
		return
	}
	t.panelOpened = time.Now()
	t.mu.Unlock()
	allowForeground()
	openPanelWindow(t.base + "/?panel=1")
}

// --- bucle de estado ---------------------------------------------------------

func (t *Tray) pollLoop() {
	for {
		t.refresh()
		time.Sleep(2 * time.Second)
	}
}

func (t *Tray) fetch() (*collector.State, error) {
	resp, err := t.client.Get(t.base + "/api/state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var s collector.State
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (t *Tray) refresh() {
	s, err := t.fetch()
	if err != nil {
		t.mu.Lock()
		last := t.last
		t.mu.Unlock()
		if last == nil {
			t.setIcon("stale", -1, 0)
			systray.SetTooltip(t.tr.noServer)
			t.miSummary.SetTitle(t.tr.noServer)
			return
		}
		s = last // seguimos mostrando lo ultimo; el chequeo de "stale" avisa
	}
	t.mu.Lock()
	t.last = s
	t.mu.Unlock()

	age := time.Since(time.Unix(s.Updated, 0))
	stale := age > staleAfter || err != nil
	status := s.Status
	if stale {
		status = "stale"
	}
	p5, p7 := -1, -1
	r5, r7 := "", ""
	if s.Limits != nil {
		if w := s.Limits.FiveHour; w != nil {
			p5, r5 = w.Pct, w.ResetIn
		}
		if w := s.Limits.SevenDay; w != nil {
			p7, r7 = w.Pct, w.ResetIn
		}
	}
	t.setIcon(status, p5, s.Active)

	stText := t.tr.status(status, age)
	plan := t.tr.planNone
	if p5 >= 0 || p7 >= 0 {
		plan = fmt.Sprintf("%s: 5h %s  ·  %s %s", t.tr.plan, pctStr(p5, r5), t.tr.week, pctStr(p7, r7))
	}
	cost := fmt.Sprintf("%s $%.2f (%s tok)  ·  7d $%.2f", t.tr.today, s.TodayCost, collector.FmtTokens(s.TodayTokens), s.WeekCost)
	systray.SetTooltip(fmt.Sprintf("Claude: %s (%d/%d)\n%s\n%s", stText, s.Working, s.Active, plan, cost))
	t.miSummary.SetTitle(fmt.Sprintf("Claude: %s  —  %s", stText, fmt.Sprintf(t.tr.sessionsCount, s.Active, s.Working)))
	t.miPlan.SetTitle(plan)
	t.miCost.SetTitle(cost)

	t.mu.Lock()
	for i, it := range t.sess {
		if i < len(s.Sessions) {
			x := s.Sessions[i]
			it.id = x.ID
			label := x.State
			if x.Tool != "" {
				label = x.Tool
			}
			if x.Flag == "waiting" {
				label = t.tr.permission
			}
			it.root.SetTitle(fmt.Sprintf("%s  —  %s  —  ctx %d%%  —  $%.2f", x.Name, label, x.Ctx, x.Cost))
			it.root.Show()
		} else {
			it.id = ""
			it.root.Hide()
		}
	}
	t.mu.Unlock()

	t.notifications(s, stale, age, p5, p7, r5, r7)
}

func pctStr(p int, reset string) string {
	if p < 0 {
		return "--"
	}
	if reset != "" {
		return fmt.Sprintf("%d%% (%s)", p, reset)
	}
	return fmt.Sprintf("%d%%", p)
}

func (t *Tray) setIcon(status string, p5, active int) {
	sig := fmt.Sprintf("%s|%d|%d", status, p5, active)
	if sig == t.iconSig {
		return
	}
	t.iconSig = sig
	png := renderIcon(status, p5, active)
	if runtime.GOOS == "windows" {
		systray.SetIcon(pngToICO(png))
	} else {
		systray.SetIcon(png)
	}
}

// --- notificaciones ------------------------------------------------------------

func (t *Tray) notify(title, msg string) {
	t.mu.Lock()
	on := t.notifyOn
	t.mu.Unlock()
	if on {
		go beeep.Notify(title, msg, t.iconFile)
	}
}

func (t *Tray) notifications(s *collector.State, stale bool, age time.Duration, p5, p7 int, r5, r7 string) {
	first := t.first
	t.first = false
	if stale {
		if !t.staleSent && !first {
			t.notify("Claude Monitor", fmt.Sprintf(t.tr.staleMsg, int(age.Minutes())))
		}
		t.staleSent = true
		return
	}
	t.staleSent = false
	now := time.Now()
	for _, x := range s.Sessions {
		prev, seen := t.prevFlag[x.ID]
		if x.Flag == "waiting" && prev != "waiting" && !first {
			t.notify(fmt.Sprintf(t.tr.waitTitle, x.Name), t.tr.waitMsg)
		}
		if x.Flag != "idle" && (!seen || prev == "idle") {
			if _, ok := t.workStart[x.ID]; !ok {
				t.workStart[x.ID] = now
			}
		}
		if x.Flag == "idle" && seen && prev != "idle" {
			if began, ok := t.workStart[x.ID]; ok && now.Sub(began) >= time.Minute && !first {
				t.notify(fmt.Sprintf(t.tr.doneTitle, x.Name), fmt.Sprintf(t.tr.doneMsg, int(now.Sub(began).Minutes())))
			}
			delete(t.workStart, x.ID)
		}
		t.prevFlag[x.ID] = x.Flag
		if x.Ctx >= 80 && !t.ctxWarned[x.ID] {
			t.notify(fmt.Sprintf(t.tr.ctxTitle, x.Name, x.Ctx), t.tr.ctxMsg)
			t.ctxWarned[x.ID] = true
		} else if x.Ctx < 60 {
			delete(t.ctxWarned, x.ID)
		}
	}
	t.lim5 = t.limitCheck(p5, t.lim5, t.tr.win5, r5)
	t.lim7 = t.limitCheck(p7, t.lim7, t.tr.winWeek, r7)
}

func (t *Tray) limitCheck(pct, warned int, name, reset string) int {
	level := 0
	switch {
	case pct >= 95:
		level = 95
	case pct >= 80:
		level = 80
	}
	if level > warned {
		msg := ""
		if reset != "" {
			msg = fmt.Sprintf(t.tr.resetsIn, reset)
		}
		t.notify(fmt.Sprintf(t.tr.limTitle, name, pct), msg)
	}
	return level
}

// OpenPanel abre el panel flotante apuntando a base (para `open --panel`).
func OpenPanel(base string) {
	allowForeground()
	openPanelWindow(strings.TrimRight(base, "/") + "/?panel=1")
}

// AllowForeground deja que el proceso que atiende la accion tome el foco.
func AllowForeground() { allowForeground() }
