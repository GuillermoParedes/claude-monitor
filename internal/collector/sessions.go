package collector

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/hook"
	"github.com/GuillermoParedes/claude-monitor/internal/paths"
	"github.com/GuillermoParedes/claude-monitor/internal/proc"
)

const sessionStale = 12 * time.Hour

// SessionView es lo que ve la UI de cada sesion.
type SessionView struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	State  string  `json:"state"` // idle | thinking | running | waiting | compacting
	Tool   string  `json:"tool,omitempty"`
	Flag   string  `json:"flag"` // idle | working | waiting
	Ctx    int     `json:"ctx"`
	Model  string  `json:"model"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
	Cwd    string  `json:"cwd"`
	Unc    string  `json:"unc,omitempty"` // ruta vista desde Windows (WSL)
	Herdr  bool    `json:"herdr,omitempty"`
	Ts     int64   `json:"ts"`
}

// LoadSession lee el registro de una sesion escrito por el hook.
func LoadSession(id string) (*hook.Session, error) {
	b, err := os.ReadFile(filepath.Join(paths.SessionsDir(), id+".json"))
	if err != nil {
		return nil, err
	}
	var s hook.Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func readSessions(today *dayAgg, now time.Time) []SessionView {
	entries, err := os.ReadDir(paths.SessionsDir())
	if err != nil {
		return nil
	}
	distro := paths.WSLDistro()
	var out []SessionView
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		full := filepath.Join(paths.SessionsDir(), e.Name())
		b, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var s hook.Session
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		if now.Sub(time.UnixMilli(s.Ts)) > sessionStale || !proc.Alive(s.Pid) {
			_ = os.Remove(full) // sesion muerta
			continue
		}
		ctx, model := contextOf(s.Transcript)
		v := SessionView{
			ID: s.SessionID, Name: s.Project, State: s.State, Tool: s.Tool,
			Ctx: ctx, Model: ShortModel(model), Cwd: s.Cwd, Ts: s.Ts,
			Herdr: s.HerdrPane != "",
		}
		switch s.State {
		case "waiting":
			v.Flag = "waiting"
		case "idle", "":
			v.Flag = "idle"
		default:
			v.Flag = "working"
		}
		if a := today.BySession[s.SessionID]; a != nil {
			v.Tokens, v.Cost = a.Tokens, round2(a.Cost)
		}
		if distro != "" && s.Cwd != "" {
			v.Unc = `\\wsl.localhost\` + distro + strings.ReplaceAll(s.Cwd, "/", `\`)
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		wi, wj := out[i].Flag != "idle", out[j].Flag != "idle"
		if wi != wj {
			return wi
		}
		return out[i].Ts > out[j].Ts
	})
	return out
}

// contextOf lee la cola del transcript y devuelve el % de contexto usado en
// la ultima respuesta del hilo principal (ignora subagentes).
func contextOf(transcript string) (int, string) {
	if transcript == "" {
		return 0, ""
	}
	f, err := os.Open(transcript)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, ""
	}
	const tail = 512 * 1024
	start := st.Size() - tail
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return 0, ""
	}
	buf, _ := io.ReadAll(f)
	lines := bytes.Split(buf, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], usageKey) {
			continue
		}
		var l struct {
			IsSidechain bool `json:"isSidechain"`
			Message     *struct {
				Model string `json:"model"`
				Usage *usage `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &l) != nil || l.Message == nil || l.Message.Usage == nil || l.IsSidechain {
			continue
		}
		u := l.Message.Usage
		used := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		pct := int(used * 100 / contextLimit(l.Message.Model))
		if pct > 100 {
			pct = 100
		}
		return pct, l.Message.Model
	}
	return 0, ""
}
