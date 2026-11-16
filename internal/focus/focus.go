// Package focus lleva al usuario a la sesion ORIGINAL de Claude Code.
//
//  1. Si la sesion corre en un panel de herdr, enfoca ese panel.
//  2. Trae al frente la app de terminal que la aloja (Windows Terminal,
//     VS Code, iTerm, ...), segun lo que registro el hook.
//
// Tambien ofrece abrir la carpeta del proyecto o una terminal nueva ahi.
package focus

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GuillermoParedes/claude-monitor/internal/collector"
	"github.com/GuillermoParedes/claude-monitor/internal/hook"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
	"github.com/GuillermoParedes/claude-monitor/internal/proc"
)

// Result describe que se hizo.
type Result struct {
	Herdr  string `json:"herdr,omitempty"`  // panel enfocado
	Raised string `json:"raised,omitempty"` // app traida al frente
}

var ErrNoSession = errors.New("session not found")

// Session enfoca la sesion indicada.
func Session(id string) (Result, error) {
	s, err := collector.LoadSession(id)
	if err != nil {
		return Result{}, ErrNoSession
	}
	var r Result
	if pane := herdrFocus(s); pane != "" {
		r.Herdr = pane
	}
	r.Raised = raiseHost(s)
	return r, nil
}

// --- herdr ------------------------------------------------------------------

func herdrBin() string {
	if p, err := exec.LookPath("herdr"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".local", "bin", "herdr")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

type herdrAgent struct {
	Agent         string `json:"agent"`
	PaneID        string `json:"pane_id"`
	Cwd           string `json:"cwd"`
	ForegroundCwd string `json:"foreground_cwd"`
	AgentSession  struct {
		Value string `json:"value"`
	} `json:"agent_session"`
}

func related(a herdrAgent, cwd string) bool {
	if cwd == "" {
		return false
	}
	for _, d := range []string{a.Cwd, a.ForegroundCwd} {
		if d == "" {
			continue
		}
		if d == cwd || strings.HasPrefix(cwd, d+"/") || strings.HasPrefix(d, cwd+"/") {
			return true
		}
	}
	return false
}

// herdrFocus devuelve el panel enfocado o "".
func herdrFocus(s *hook.Session) string {
	bin := herdrBin()
	if bin == "" {
		return ""
	}
	pane, sock := s.HerdrPane, s.HerdrSocket
	// El entorno del proceso claude es la fuente mas fiable: herdr puede listar
	// el mismo session id en dos paneles (p. ej. tras un --resume).
	if v := proc.Environ(s.Pid, "HERDR_PANE_ID"); v != "" {
		pane = v
		if v := proc.Environ(s.Pid, "HERDR_SOCKET_PATH"); v != "" {
			sock = v
		}
	}
	if pane == "" && sock == "" && os.Getenv("HERDR_SOCKET_PATH") == "" {
		// Sin rastro de herdr en esta sesion: no la buscamos por carpeta, para
		// no saltar a otra sesion que solo comparte proyecto.
		return ""
	}
	env := os.Environ()
	if sock != "" {
		env = append(env, "HERDR_SOCKET_PATH="+sock)
	}
	run := func(args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		return cmd.Output()
	}

	out, err := run("agent", "list")
	if err != nil {
		return ""
	}
	var resp struct {
		Result struct {
			Agents []herdrAgent `json:"agents"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &resp) != nil {
		return ""
	}
	agents := resp.Result.Agents

	pick := func(f func(herdrAgent) bool) []herdrAgent {
		var r []herdrAgent
		for _, a := range agents {
			if f(a) {
				r = append(r, a)
			}
		}
		return r
	}
	var target string
	if byPane := pick(func(a herdrAgent) bool { return pane != "" && a.PaneID == pane }); len(byPane) > 0 {
		target = byPane[0].PaneID
	} else if bySid := pick(func(a herdrAgent) bool { return a.AgentSession.Value == s.SessionID }); len(bySid) == 1 {
		target = bySid[0].PaneID
	} else if both := pick(func(a herdrAgent) bool { return a.AgentSession.Value == s.SessionID && related(a, s.Cwd) }); len(both) == 1 {
		target = both[0].PaneID
	} else if byCwd := pick(func(a herdrAgent) bool { return a.Agent == "claude" && related(a, s.Cwd) }); len(byCwd) == 1 {
		target = byCwd[0].PaneID
	}
	if target == "" {
		return ""
	}
	if _, err := run("agent", "focus", target); err != nil {
		return ""
	}
	return target
}

// --- traer al frente la app que aloja la sesion -----------------------------

// hostApp deduce la app de terminal a partir de lo que registro el hook.
func hostApp(s *hook.Session) string {
	switch {
	case s.VSCode:
		return "vscode"
	case s.WTSession, s.HerdrPane != "" && (runtime.GOOS == "windows" || paths.IsWSL()):
		return "windows-terminal"
	}
	switch s.TermProgram {
	case "iTerm.app":
		return "iterm"
	case "Apple_Terminal":
		return "terminal"
	case "WezTerm":
		return "wezterm"
	case "ghostty":
		return "ghostty"
	case "vscode":
		return "vscode"
	}
	if runtime.GOOS == "windows" || paths.IsWSL() {
		return "windows-terminal"
	}
	return ""
}

func raiseHost(s *hook.Session) string {
	app := hostApp(s)
	if app == "" {
		return ""
	}
	if raise(app) {
		return app
	}
	return ""
}

// --- carpeta / terminal nueva ------------------------------------------------

// OpenFolder abre la carpeta del proyecto en el explorador del sistema.
func OpenFolder(id string) error {
	s, err := collector.LoadSession(id)
	if err != nil {
		return ErrNoSession
	}
	switch {
	case paths.IsWSL():
		unc := `\\wsl.localhost\` + paths.WSLDistro() + strings.ReplaceAll(s.Cwd, "/", `\`)
		return startDetached("explorer.exe", unc)
	case runtime.GOOS == "windows":
		return startDetached("explorer.exe", s.Cwd)
	case runtime.GOOS == "darwin":
		return startDetached("open", s.Cwd)
	default:
		return startDetached("xdg-open", s.Cwd)
	}
}

// OpenTerminal abre una terminal NUEVA en la carpeta del proyecto.
func OpenTerminal(id string) error {
	s, err := collector.LoadSession(id)
	if err != nil {
		return ErrNoSession
	}
	switch {
	case paths.IsWSL():
		return startDetached("wt.exe", "-w", "0", "nt", "--title", s.Project, "wsl.exe", "-d", paths.WSLDistro(), "--cd", s.Cwd)
	case runtime.GOOS == "windows":
		if _, err := exec.LookPath("wt.exe"); err == nil {
			return startDetached("wt.exe", "-w", "0", "nt", "-d", s.Cwd)
		}
		return startDetached("cmd.exe", "/C", "start", "", "/D", s.Cwd, "cmd.exe")
	case runtime.GOOS == "darwin":
		return startDetached("open", "-a", "Terminal", s.Cwd)
	default:
		for _, t := range [][]string{
			{"x-terminal-emulator", "--working-directory=" + s.Cwd},
			{"gnome-terminal", "--working-directory=" + s.Cwd},
			{"konsole", "--workdir", s.Cwd},
		} {
			if _, err := exec.LookPath(t[0]); err == nil {
				return startDetached(t[0], t[1:]...)
			}
		}
		return errors.New("no terminal emulator found")
	}
}

func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if runtime.GOOS != "windows" {
		cmd.Dir = os.TempDir()
		if paths.IsWSL() {
			cmd.Dir = "/mnt/c" // los .exe de Windows no aceptan un cwd UNC
		}
	}
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
