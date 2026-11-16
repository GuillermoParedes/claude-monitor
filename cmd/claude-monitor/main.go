// claude-monitor: uso, limites del plan y sesiones de Claude Code en la
// bandeja del sistema (Windows, macOS, Linux y WSL).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/collector"
	"github.com/GuillermoParedes/claude-monitor/internal/hook"
	"github.com/GuillermoParedes/claude-monitor/internal/install"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
	"github.com/GuillermoParedes/claude-monitor/internal/server"
	"github.com/GuillermoParedes/claude-monitor/internal/statusline"
	"github.com/GuillermoParedes/claude-monitor/internal/tray"
)

var version = "dev"

const usage = `claude-monitor %s — Claude Code usage, plan limits and sessions in your system tray

Usage:
  claude-monitor install [--windows-exe PATH] [--no-start]   set up hooks, statusline and autostart
  claude-monitor uninstall [--purge]                         undo install (--purge also deletes data)
  claude-monitor run [--headless] [--port N]                 collector + local server + tray icon
  claude-monitor tray --connect URL                          tray only, talking to a remote collector (WSL)
  claude-monitor focus|folder|terminal <session-id>          go to a session / open its folder / new terminal
  claude-monitor status [--json]                             print the current state
  claude-monitor open [--panel] [--connect URL]              open the dashboard (--panel: floating window)
  claude-monitor doctor                                      diagnose the setup
  claude-monitor version

Internal (called by Claude Code):
  claude-monitor hook <idle|thinking|running|waiting|compacting|end>
  claude-monitor statusline
`

func main() {
	collector.Version = version
	attachConsole()
	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		return
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "hook":
		state := "idle"
		if len(args) > 0 {
			state = args[0]
		}
		hook.Run(state, os.Stdin)
	case "statusline":
		statusline.Run(os.Stdin, os.Stdout)
	case "run":
		cmdRun(args)
	case "tray":
		cmdTray(args)
	case "install":
		cmdInstall(args)
	case "uninstall":
		cmdUninstall(args)
	case "status":
		cmdStatus(args)
	case "open":
		cmdOpen(args)
	case "focus", "folder", "terminal":
		cmdAction(cmd, args)
	case "doctor":
		cmdDoctor()
	case "version", "--version", "-v":
		fmt.Println("claude-monitor", version, runtime.GOOS+"/"+runtime.GOARCH)
	case "help", "--help", "-h":
		fmt.Printf(usage, version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		fmt.Printf(usage, version)
		os.Exit(2)
	}
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	headless := fs.Bool("headless", false, "no tray icon (collector + server only)")
	port := fs.Int("port", 0, "local server port")
	_ = fs.Parse(args)

	cfg := collector.LoadConfig()
	p := cfg.PortOrDefault()
	if *port > 0 {
		p = *port
	}
	_ = paths.EnsureDirs()
	collector.LogTo(paths.LogFile())

	col := collector.New()
	stop := make(chan struct{})
	go col.Run(3*time.Second, stop)

	if err := server.New(col, p).Listen(); err != nil {
		log.Printf("run: port %d busy (another instance?): %v", p, err)
		fmt.Fprintf(os.Stderr, "claude-monitor is already running (port %d busy)\n", p)
		os.Exit(1)
	}
	log.Printf("run: listening on 127.0.0.1:%d (version %s)", p, version)

	if *headless || (runtime.GOOS == "linux" && install.Headless()) {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		close(stop)
		time.Sleep(200 * time.Millisecond)
		return
	}
	tray.Run(fmt.Sprintf("http://127.0.0.1:%d", p), func() { close(stop); time.Sleep(200 * time.Millisecond) })
}

func cmdTray(args []string) {
	fs := flag.NewFlagSet("tray", flag.ExitOnError)
	connect := fs.String("connect", "", "collector URL, e.g. http://localhost:47821")
	_ = fs.Parse(args)
	if *connect == "" {
		*connect = fmt.Sprintf("http://127.0.0.1:%d", collector.LoadConfig().PortOrDefault())
	}
	_ = paths.EnsureDirs()
	collector.LogTo(paths.LogFile())
	tray.Run(*connect, nil)
}

func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	winExe := fs.String("windows-exe", "", "WSL: local path to claude-monitor.exe for the Windows tray (default: download)")
	noStart := fs.Bool("no-start", false, "do not start now")
	noWin := fs.Bool("no-windows", false, "WSL: do not install the Windows tray")
	_ = fs.Parse(args)
	if err := install.Install(install.Options{WindowsExe: *winExe, NoStart: *noStart, NoWindows: *noWin, Out: os.Stdout}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdUninstall(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	purge := fs.Bool("purge", false, "also delete collected data")
	_ = fs.Parse(args)
	if err := install.Uninstall(install.Options{Purge: *purge, Out: os.Stdout}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// fetchState usa el servidor si corre; si no, calcula una foto local.
func fetchState() (*collector.State, bool) {
	c := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/state", collector.LoadConfig().PortOrDefault())
	if resp, err := c.Get(url); err == nil {
		defer resp.Body.Close()
		var s collector.State
		if resp.StatusCode == 200 && json.NewDecoder(resp.Body).Decode(&s) == nil {
			return &s, true
		}
	}
	return collector.New().Tick(), false
}

func cmdStatus(args []string) {
	s, live := fetchState()
	if len(args) > 0 && args[0] == "--json" {
		b, _ := json.MarshalIndent(s, "", "  ")
		fmt.Println(string(b))
		return
	}
	src := "local snapshot (collector not running)"
	if live {
		src = "live"
	}
	fmt.Printf("Claude Monitor %s — %s\n\n", version, src)
	fmt.Printf("Status    %s  (%d sessions, %d active)\n", s.Status, s.Active, s.Working)
	if l := s.Limits; l != nil {
		if w := l.FiveHour; w != nil {
			fmt.Printf("Plan 5h   %d%%  resets in %s\n", w.Pct, w.ResetIn)
		}
		if w := l.SevenDay; w != nil {
			fmt.Printf("Plan week %d%%  resets in %s\n", w.Pct, w.ResetIn)
		}
	} else {
		fmt.Println("Plan      no data yet (filled by the statusline while you use Claude Code)")
	}
	fmt.Printf("Today     %s tokens · $%.2f (API equivalent)\n", collector.FmtTokens(s.TodayTokens), s.TodayCost)
	fmt.Printf("7 days    %s tokens · $%.2f\n", collector.FmtTokens(s.WeekTokens), s.WeekCost)
	if len(s.Sessions) > 0 {
		fmt.Println("\nSessions")
		for _, x := range s.Sessions {
			label := x.State
			if x.Tool != "" {
				label = x.Tool
			}
			fmt.Printf("  %-22s %-12s ctx %3d%%  $%.2f  %s\n", x.Name, label, x.Ctx, x.Cost, x.Cwd)
		}
	}
}

func cmdDoctor() {
	ok := func(good bool, msg string, a ...any) {
		mark := "✓"
		if !good {
			mark = "✗"
		}
		fmt.Printf("%s %s\n", mark, fmt.Sprintf(msg, a...))
	}
	fmt.Printf("claude-monitor %s (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
	if paths.IsWSL() {
		fmt.Printf(" — WSL %s", paths.WSLDistro())
	}
	fmt.Println()
	_, err := os.Stat(paths.ProjectsDir())
	ok(err == nil, "Claude Code transcripts: %s", paths.ProjectsDir())
	b, _ := os.ReadFile(paths.SettingsFile())
	settings := string(b)
	ok(strings.Contains(settings, "claude-monitor") && strings.Contains(settings, " hook "), "hooks in %s", paths.SettingsFile())
	ok(strings.Contains(settings, "claude-monitor") && strings.Contains(settings, " statusline"), "statusLine (captures plan limits)")
	if w := collector.LoadConfig().WrapStatusLine; w != "" {
		fmt.Printf("  wrapping your previous statusLine: %s\n", w)
	}
	ok(install.Running(), "collector running on 127.0.0.1:%d", collector.LoadConfig().PortOrDefault())
	if st, err := os.Stat(paths.LimitsFile()); err == nil {
		ok(time.Since(st.ModTime()) < 24*time.Hour, "plan limits captured %s ago", time.Since(st.ModTime()).Round(time.Second))
	} else {
		ok(false, "plan limits not captured yet (needs a subscription and one response in Claude Code)")
	}
	entries, _ := os.ReadDir(paths.SessionsDir())
	fmt.Printf("  %d session records in %s\n", len(entries), paths.SessionsDir())
	fmt.Printf("  log: %s\n", paths.LogFile())
}

func cmdOpen(args []string) {
	fs := flag.NewFlagSet("open", flag.ExitOnError)
	panel := fs.Bool("panel", false, "open the compact floating panel instead of a browser tab")
	connect := fs.String("connect", "", "collector URL (default: local)")
	_ = fs.Parse(args)
	base := *connect
	if base == "" {
		base = fmt.Sprintf("http://127.0.0.1:%d", collector.LoadConfig().PortOrDefault())
	}
	if *panel {
		tray.OpenPanel(base)
		return
	}
	openBrowser(strings.TrimRight(base, "/") + "/")
}

// cmdAction pide al colector (local o --connect) una accion sobre una sesion.
func cmdAction(action string, args []string) {
	fs := flag.NewFlagSet(action, flag.ExitOnError)
	connect := fs.String("connect", "", "collector URL (default: local)")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: claude-monitor %s <session-id>\n", action)
		os.Exit(2)
	}
	base := *connect
	if base == "" {
		base = fmt.Sprintf("http://127.0.0.1:%d", collector.LoadConfig().PortOrDefault())
	}
	tray.AllowForeground()
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+"/api/"+action+"/"+url.PathEscape(fs.Arg(0)), nil)
	req.Header.Set("X-Claude-Monitor", "1")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Fprintln(os.Stderr, "error:", resp.Status)
		os.Exit(1)
	}
}
