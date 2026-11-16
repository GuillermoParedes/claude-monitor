// Package hook implementa `claude-monitor hook <estado>`, invocado por los hooks
// de Claude Code en cada evento. Registra que hace cada sesion en
// <DataDir>/sessions/<session_id>.json, que luego lee el colector.
//
// Corre en CADA tool call, asi que debe ser rapido y nunca fallar: cualquier
// error se ignora y se sale con 0 para no interferir con Claude Code.
package hook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
	"github.com/GuillermoParedes/claude-monitor/internal/proc"
)

// Session es el registro que deja el hook para cada sesion.
type Session struct {
	SessionID   string `json:"sessionId"`
	Project     string `json:"project"`
	Cwd         string `json:"cwd"`
	Transcript  string `json:"transcript"`
	State       string `json:"state"` // idle | thinking | running | waiting | compacting
	Tool        string `json:"tool,omitempty"`
	Pid         int    `json:"pid,omitempty"`
	Ts          int64  `json:"ts"` // ms
	HerdrPane   string `json:"herdrPane,omitempty"`
	HerdrSocket string `json:"herdrSocket,omitempty"`
	TermProgram string `json:"termProgram,omitempty"`
	WTSession   bool   `json:"wtSession,omitempty"`
	VSCode      bool   `json:"vscode,omitempty"`
}

type payload struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
	ToolName       string `json:"tool_name"`
}

// Run procesa un evento. Estados: idle, thinking, running, waiting, compacting, end.
func Run(state string, stdin io.Reader) {
	data, err := io.ReadAll(io.LimitReader(stdin, 4<<20))
	if err != nil || len(data) == 0 {
		return
	}
	var p payload
	if json.Unmarshal(data, &p) != nil || p.SessionID == "" {
		return
	}
	_ = paths.EnsureDirs()
	file := filepath.Join(paths.SessionsDir(), safeName(p.SessionID)+".json")

	if state == "end" {
		_ = os.Remove(file)
		return
	}

	// Conservamos el pid ya detectado: subir por la cadena de procesos cuesta
	// (sobre todo en macOS, donde usa `ps`).
	var prev Session
	if b, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(b, &prev)
	}

	s := Session{
		SessionID:   p.SessionID,
		Cwd:         p.Cwd,
		Project:     filepath.Base(p.Cwd),
		Transcript:  p.TranscriptPath,
		State:       state,
		Tool:        p.ToolName,
		Pid:         prev.Pid,
		Ts:          time.Now().UnixMilli(),
		HerdrPane:   os.Getenv("HERDR_PANE_ID"),
		HerdrSocket: os.Getenv("HERDR_SOCKET_PATH"),
		TermProgram: os.Getenv("TERM_PROGRAM"),
		WTSession:   os.Getenv("WT_SESSION") != "",
		VSCode:      os.Getenv("VSCODE_PID") != "" || os.Getenv("TERM_PROGRAM") == "vscode",
	}
	if p.Cwd == "" {
		s.Project = "claude"
	}
	// Al terminar una herramienta volvemos a "pensando"; el nombre de la
	// herramienta solo tiene sentido mientras corre o pide permiso.
	if state != "running" && state != "waiting" {
		s.Tool = ""
	}
	if s.Pid == 0 || !proc.Alive(s.Pid) {
		s.Pid = proc.FindClaudeAncestor()
	}

	b, _ := json.Marshal(s)
	tmp := file + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, file)
	}
}

func safeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '/' || r == '\\' || r == ':' || r == '.' {
			r = '_'
		}
		out = append(out, r)
	}
	return string(out)
}
