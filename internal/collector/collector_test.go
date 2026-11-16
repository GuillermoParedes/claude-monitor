package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Una respuesta de Claude Code aparece en varias lineas (una por bloque de
// contenido) con el MISMO usage; y un --resume la copia a otro transcript.
const transcriptA = `{"type":"assistant","timestamp":"%TS%","sessionId":"s1","cwd":"/work/alpha","requestId":"r1","message":{"id":"m1","model":"claude-opus-5-5","usage":{"input_tokens":100,"output_tokens":1000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
{"type":"assistant","timestamp":"%TS%","sessionId":"s1","cwd":"/work/alpha","requestId":"r1","message":{"id":"m1","model":"claude-opus-5-5","usage":{"input_tokens":100,"output_tokens":1000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
{"type":"user","timestamp":"%TS%","message":{"content":"hola"}}
{"type":"assistant","timestamp":"%TS%","sessionId":"s1","cwd":"/work/alpha","requestId":"r2","message":{"id":"m2","model":"claude-haiku-5-5","usage":{"input_tokens":1000000,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
`

const transcriptResume = `{"type":"assistant","timestamp":"%TS%","sessionId":"s2","cwd":"/work/alpha","requestId":"r1","message":{"id":"m1","model":"claude-opus-5-5","usage":{"input_tokens":100,"output_tokens":1000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
`

func writeTranscript(t *testing.T, dir, name, body string) {
	t.Helper()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	b := []byte(replaceAll(body, "%TS%", ts))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceAll(s, old, new string) string {
	out := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + new
		s = s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestDedupeAndCost(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CLAUDE_MONITOR_DIR", filepath.Join(claude, "monitor"))
	proj := filepath.Join(claude, "projects", "-work-alpha")
	writeTranscript(t, proj, "s1.jsonl", transcriptA)
	writeTranscript(t, proj, "s2.jsonl", transcriptResume)

	s := New().Tick()

	// m1 cuenta UNA vez: 100 in + 1000 out con opus ($5/$25 por M) = 0.0255
	// m2 (haiku, $1/M in): 1M in = $1.00
	wantTokens := int64(1100 + 1_000_000)
	if s.TodayTokens != wantTokens {
		t.Fatalf("tokens: got %d want %d (dedupe failed?)", s.TodayTokens, wantTokens)
	}
	if s.TodayCost != 1.03 { // 1.0255 redondeado
		t.Fatalf("cost: got %v want 1.03", s.TodayCost)
	}
	if len(s.Projects) != 1 || s.Projects[0].Name != "alpha" {
		t.Fatalf("projects: %+v", s.Projects)
	}
	if len(s.Models) != 2 {
		t.Fatalf("models: %+v", s.Models)
	}
	if len(s.Days) != 7 || s.Days[6].Cost != 1.03 {
		t.Fatalf("days: %+v", s.Days)
	}

	// Pasada incremental: sin cambios no recuenta nada.
	c := New()
	c.Tick()
	if again := c.Tick(); again.TodayTokens != wantTokens {
		t.Fatalf("incremental pass changed totals: %d", again.TodayTokens)
	}
}

func TestShortModel(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5-5":           "opus-5.5",
		"claude-haiku-5-5-20261001": "haiku-5.5",
		"":                          "?",
	}
	for in, want := range cases {
		if got := ShortModel(in); got != want {
			t.Errorf("ShortModel(%q) = %q, want %q", in, got, want)
		}
	}
}
