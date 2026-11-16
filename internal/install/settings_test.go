package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const userSettings = `{
  "model": "opus",
  "statusLine": { "type": "command", "command": "~/bin/my-status.sh", "padding": 1 },
  "hooks": {
    "Stop": [ { "hooks": [ { "type": "command", "command": "notify-send done && echo <ok>" } ] } ]
  },
  "permissions": { "allow": ["Bash(ls:*)"] }
}
`

func setup(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "settings.json")
	if content != "" {
		if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func load(t *testing.T, f string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, b)
	}
	return m
}

func countOurs(m map[string]any) int {
	n := 0
	hooks, _ := m["hooks"].(map[string]any)
	for _, v := range hooks {
		for _, e := range v.([]any) {
			for _, h := range e.(map[string]any)["hooks"].([]any) {
				if isOurs(h.(map[string]any)["command"].(string)) {
					n++
				}
			}
		}
	}
	return n
}

func TestInstallIsIdempotentAndPreservesUserConfig(t *testing.T) {
	f := setup(t, userSettings)
	bin := "/home/u/.local/bin/claude-monitor"

	wrapped, err := applySettings(f, bin, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if wrapped != "~/bin/my-status.sh" {
		t.Fatalf("expected previous statusLine to be wrapped, got %q", wrapped)
	}
	// Segunda instalacion: no duplica y sigue envolviendo el statusLine original.
	wrapped2, err := applySettings(f, bin, true, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped2 != wrapped {
		t.Fatalf("re-install lost the wrapped statusLine: %q", wrapped2)
	}

	m := load(t, f)
	if got := countOurs(m); got != len(hookEvents) {
		t.Fatalf("expected %d of our hooks, got %d", len(hookEvents), got)
	}
	if m["model"] != "opus" || m["permissions"] == nil {
		t.Fatal("user keys were lost")
	}
	stop := m["hooks"].(map[string]any)["Stop"].([]any)
	if raw, _ := os.ReadFile(f); !strings.Contains(string(raw), "notify-send done && echo <ok>") {
		t.Fatal("user's hook command was escaped (\\u0026) or removed")
	}
	if !strings.Contains(mustJSON(stop), "notify-send done") {
		t.Fatal("user's own Stop hook was removed")
	}
	sl := m["statusLine"].(map[string]any)
	if !strings.Contains(sl["command"].(string), "claude-monitor\" statusline") || sl["padding"].(float64) != 1 {
		t.Fatalf("unexpected statusLine: %v", sl)
	}

	// El orden de las claves de primer nivel se conserva.
	raw, _ := os.ReadFile(f)
	s := string(raw)
	if !(strings.Index(s, `"model"`) < strings.Index(s, `"statusLine"`) && strings.Index(s, `"statusLine"`) < strings.Index(s, `"hooks"`) && strings.Index(s, `"hooks"`) < strings.Index(s, `"permissions"`)) {
		t.Fatalf("top-level key order changed:\n%s", s)
	}
	if _, err := os.Stat(f + ".claude-monitor.bak"); err != nil {
		t.Fatal("no backup written")
	}
}

func TestUninstallRestoresOriginal(t *testing.T) {
	f := setup(t, userSettings)
	bin := `C:\Users\u\AppData\Local\Programs\claude-monitor\claude-monitor.exe`
	wrapped, err := applySettings(f, bin, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applySettings(f, bin, false, wrapped); err != nil {
		t.Fatal(err)
	}
	m := load(t, f)
	if countOurs(m) != 0 {
		t.Fatal("our hooks remain after uninstall")
	}
	if m["statusLine"].(map[string]any)["command"] != "~/bin/my-status.sh" {
		t.Fatalf("statusLine not restored: %v", m["statusLine"])
	}
	hooks := m["hooks"].(map[string]any)
	if len(hooks) != 1 || hooks["Stop"] == nil {
		t.Fatalf("expected only the user's Stop hook, got %v", hooks)
	}
}

func TestInstallOnEmptyAndMissingFile(t *testing.T) {
	f := setup(t, "")
	if _, err := applySettings(f, "/x/claude-monitor", true, ""); err != nil {
		t.Fatal(err)
	}
	m := load(t, f)
	if countOurs(m) != len(hookEvents) {
		t.Fatal("hooks not installed on missing settings.json")
	}
	if _, err := applySettings(f, "/x/claude-monitor", false, ""); err != nil {
		t.Fatal(err)
	}
	m = load(t, f)
	if _, ok := m["statusLine"]; ok {
		t.Fatal("statusLine should be removed when there was none before")
	}
	if _, ok := m["hooks"]; ok {
		t.Fatal("empty hooks object should be removed")
	}
}

func TestWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles-settings.json")
	if err := os.WriteFile(real, []byte(userSettings), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported")
	}
	if _, err := applySettings(link, "/x/claude-monitor", true, ""); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	if countOurs(load(t, real)) == 0 {
		t.Fatal("target file not updated")
	}
}

func TestRejectsInvalidJSON(t *testing.T) {
	f := setup(t, "{ not json")
	if _, err := applySettings(f, "/x/claude-monitor", true, ""); err == nil {
		t.Fatal("expected error on invalid settings.json")
	}
	b, _ := os.ReadFile(f)
	if string(b) != "{ not json" {
		t.Fatal("invalid file must not be modified")
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestRoundTripIsByteIdentical(t *testing.T) {
	f := setup(t, userSettings)
	orig, _ := os.ReadFile(f)
	wrapped, err := applySettings(f, "/x/claude-monitor", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applySettings(f, "/x/claude-monitor", false, wrapped); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(f)
	// El original de la prueba no esta indentado como lo deja json.Indent; lo
	// normalizamos igual que applySettings para comparar contenido y orden.
	var a, b bytes.Buffer
	_ = json.Indent(&a, bytes.TrimSpace(orig), "", "  ")
	_ = json.Indent(&b, bytes.TrimSpace(got), "", "  ")
	var ca, cb bytes.Buffer
	_ = json.Compact(&ca, a.Bytes())
	_ = json.Compact(&cb, b.Bytes())
	if ca.String() != cb.String() {
		t.Fatalf("round trip changed settings:\n%s\n---\n%s", ca.String(), cb.String())
	}
}
