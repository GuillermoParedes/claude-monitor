package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ordered es un objeto JSON que conserva el orden de sus claves, para no
// reordenar el settings.json del usuario al reescribirlo.
type ordered struct {
	keys []string
	vals map[string]json.RawMessage
}

func newOrdered() *ordered { return &ordered{vals: map[string]json.RawMessage{}} }

func (o *ordered) UnmarshalJSON(b []byte) error {
	o.vals = map[string]json.RawMessage{}
	o.keys = nil
	dec := json.NewDecoder(bytes.NewReader(b))
	t, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("expected object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		k := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return err
		}
		if _, dup := o.vals[k]; !dup {
			o.keys = append(o.keys, k)
		}
		o.vals[k] = v
	}
	return nil
}

func (o *ordered) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func (o *ordered) get(k string) (json.RawMessage, bool) { v, ok := o.vals[k]; return v, ok }

func (o *ordered) set(k string, v any) {
	b, _ := marshal(v)
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = b
}

func (o *ordered) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// marshal es json.Marshal sin escapar &, < y > (que Go convierte en \u0026...):
// el settings.json del usuario debe quedar legible.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// --- hooks -------------------------------------------------------------------

type hookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type hookEntry struct {
	Matcher string    `json:"matcher,omitempty"`
	Hooks   []hookCmd `json:"hooks"`
}

// Eventos de Claude Code -> estado que registra `claude-monitor hook`.
var hookEvents = []struct{ event, matcher, state string }{
	{"SessionStart", "", "idle"},
	{"UserPromptSubmit", "", "thinking"},
	{"PreToolUse", "", "running"},
	{"PostToolUse", "", "thinking"},
	{"PostToolUseFailure", "", "thinking"},
	{"PermissionRequest", "", "waiting"},
	{"Notification", "permission_prompt", "waiting"},
	{"PreCompact", "", "compacting"},
	{"Stop", "", "idle"},
	{"StopFailure", "", "idle"},
	{"SessionEnd", "", "end"},
}

// isOurs reconoce comandos instalados por claude-monitor (en cualquier ruta).
func isOurs(cmd string) bool {
	c := strings.ToLower(strings.ReplaceAll(cmd, `\`, "/"))
	return strings.Contains(c, "claude-monitor") &&
		(strings.Contains(c, " hook ") || strings.HasSuffix(c, " statusline"))
}

func quote(p string) string { return `"` + filepath.ToSlash(p) + `"` }

// stripOurs quita nuestros hooks de todos los eventos (y los eventos vacios).
func stripOurs(hooks *ordered) {
	for _, ev := range append([]string(nil), hooks.keys...) {
		var entries []json.RawMessage
		if json.Unmarshal(hooks.vals[ev], &entries) != nil {
			continue
		}
		var keep []json.RawMessage
		for _, raw := range entries {
			var e struct {
				Matcher string            `json:"matcher"`
				Hooks   []json.RawMessage `json:"hooks"`
			}
			if json.Unmarshal(raw, &e) != nil {
				keep = append(keep, raw)
				continue
			}
			var rest []json.RawMessage
			for _, h := range e.Hooks {
				var c hookCmd
				if json.Unmarshal(h, &c) == nil && isOurs(c.Command) {
					continue
				}
				rest = append(rest, h)
			}
			if len(rest) == len(e.Hooks) {
				keep = append(keep, raw)
				continue
			}
			if len(rest) == 0 {
				continue
			}
			var full map[string]json.RawMessage
			_ = json.Unmarshal(raw, &full)
			b, _ := marshal(rest)
			full["hooks"] = b
			nb, _ := marshal(full)
			keep = append(keep, nb)
		}
		if len(keep) == 0 {
			hooks.del(ev)
		} else {
			hooks.set(ev, keep)
		}
	}
}

// applySettings instala (install=true) o retira nuestros hooks y statusLine.
// prevStatus es el statusLine que el usuario tenia (para envolverlo / restaurarlo).
func applySettings(file, bin string, install bool, prevStatus string) (wrapped string, err error) {
	root := newOrdered()
	raw, err := os.ReadFile(file)
	if err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, root); err != nil {
			return "", fmt.Errorf("%s no es JSON valido: %w", file, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	hooks := newOrdered()
	if v, ok := root.get("hooks"); ok {
		_ = json.Unmarshal(v, hooks)
	}
	stripOurs(hooks)

	if install {
		for _, h := range hookEvents {
			var entries []json.RawMessage
			if v, ok := hooks.get(h.event); ok {
				_ = json.Unmarshal(v, &entries)
			}
			e, _ := marshal(hookEntry{Matcher: h.matcher, Hooks: []hookCmd{{
				Type: "command", Command: quote(bin) + " hook " + h.state, Timeout: 10,
			}}})
			hooks.set(h.event, append(entries, e))
		}
	}
	if len(hooks.keys) > 0 {
		root.set("hooks", hooks)
	} else {
		root.del("hooks")
	}

	// statusLine: el nuestro captura los limites del plan y ejecuta el previo.
	var sl map[string]any
	if v, ok := root.get("statusLine"); ok {
		_ = json.Unmarshal(v, &sl)
	}
	current, _ := sl["command"].(string)
	if install {
		if current != "" && !isOurs(current) {
			wrapped = current
		} else {
			wrapped = prevStatus
		}
		root.set("statusLine", statusLine(quote(bin)+" statusline", sl, 0))
	} else if isOurs(current) {
		if prevStatus != "" {
			root.set("statusLine", statusLine(prevStatus, sl, nil))
		} else {
			root.del("statusLine")
		}
	}

	out, err := marshal(root)
	if err != nil {
		return "", err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, out, "", "  "); err != nil {
		return "", err
	}
	pretty.WriteByte('\n')

	if raw != nil {
		_ = os.WriteFile(file+".claude-monitor.bak", raw, 0o644)
	}
	return wrapped, writeThroughSymlink(file, pretty.Bytes())
}

// statusLine arma el objeto con el orden habitual (type, command, padding) y
// conserva el padding que tenia el usuario.
func statusLine(command string, prev map[string]any, defPadding any) *ordered {
	o := newOrdered()
	o.set("type", "command")
	o.set("command", command)
	if p, ok := prev["padding"]; ok {
		o.set("padding", p)
	} else if defPadding != nil {
		o.set("padding", defPadding)
	}
	return o
}

// writeThroughSymlink escribe de forma atomica sobre el destino real, sin
// reemplazar el symlink (muchos usuarios versionan settings.json en dotfiles).
func writeThroughSymlink(file string, data []byte) error {
	target := file
	if r, err := filepath.EvalSymlinks(file); err == nil {
		target = r
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
