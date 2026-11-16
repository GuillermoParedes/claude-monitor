package tray

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type strs struct {
	openPanel, sessions, goTo, folder, terminal, notifications, dataFolder, quit string
	noServer, plan, planNone, week, today, permission, sessionsCount             string
	working, waiting, idle, off, staleFmt                                        string
	staleMsg, waitTitle, waitMsg, doneTitle, doneMsg, ctxTitle, ctxMsg           string
	win5, winWeek, limTitle, resetsIn, widget                                    string
}

var en = strs{
	openPanel: "Open panel", sessions: "Sessions", goTo: "Go to session", folder: "Open folder",
	terminal: "Open new terminal here", notifications: "Notifications", dataFolder: "Open data folder", quit: "Quit",
	noServer: "Claude Monitor: collector not reachable", plan: "Plan", planNone: "Plan: no data yet",
	week: "week", today: "Today", permission: "PERMISSION", sessionsCount: "%d sessions, %d active",
	working: "working", waiting: "NEEDS PERMISSION", idle: "idle", off: "no sessions", staleFmt: "no data for %dm",
	staleMsg:  "The collector has not updated for %d min. Run: claude-monitor doctor",
	waitTitle: "%s: needs permission", waitMsg: "Claude Code is waiting for your approval.",
	doneTitle: "%s: finished", doneMsg: "Claude Code finished (%d min) and is waiting for you.",
	ctxTitle: "%s: context at %d%%", ctxMsg: "Consider /compact or a fresh session soon.",
	win5: "5-hour window", winWeek: "weekly limit", limTitle: "Plan: %s at %d%%", resetsIn: "Resets in %s.",
	widget: "Show / hide desktop widget",
}

var es = strs{
	openPanel: "Abrir panel", sessions: "Sesiones", goTo: "Ir a la sesión", folder: "Abrir carpeta",
	terminal: "Abrir terminal nueva aquí", notifications: "Notificaciones", dataFolder: "Abrir carpeta de datos", quit: "Salir",
	noServer: "Claude Monitor: no se puede contactar al colector", plan: "Plan", planNone: "Plan: sin datos aún",
	week: "sem", today: "Hoy", permission: "PERMISO", sessionsCount: "%d sesiones, %d activas",
	working: "trabajando", waiting: "ESPERA PERMISO", idle: "en reposo", off: "sin sesiones", staleFmt: "sin datos hace %dm",
	staleMsg:  "El colector no actualiza hace %d min. Ejecuta: claude-monitor doctor",
	waitTitle: "%s: espera permiso", waitMsg: "Claude Code necesita tu aprobación para continuar.",
	doneTitle: "%s: terminó", doneMsg: "Claude Code terminó (%d min) y espera tu siguiente mensaje.",
	ctxTitle: "%s: contexto al %d%%", ctxMsg: "Considera /compact o una sesión nueva pronto.",
	win5: "ventana de 5h", winWeek: "límite semanal", limTitle: "Plan: %s al %d%%", resetsIn: "Se reinicia en %s.",
	widget: "Mostrar / ocultar widget de escritorio",
}

func (s strs) status(st string, age time.Duration) string {
	switch st {
	case "working":
		return s.working
	case "waiting":
		return s.waiting
	case "idle":
		return s.idle
	case "stale":
		return fmt.Sprintf(s.staleFmt, int(age.Minutes()))
	}
	return s.off
}

// lang elige el idioma: config > sistema > ingles.
func lang(forced string) strs {
	l := strings.ToLower(forced)
	if l == "" {
		l = strings.ToLower(os.Getenv("LC_ALL") + os.Getenv("LC_MESSAGES") + os.Getenv("LANG"))
		if l == "" || strings.HasPrefix(l, "c.") || l == "c" || l == "posix" {
			l = systemLang()
		}
	}
	if strings.HasPrefix(l, "es") {
		return es
	}
	return en
}
