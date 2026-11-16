package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := (&Server{}).guard(ok)
	cases := []struct {
		name, method, host, header string
		want                       int
	}{
		{"local GET", "GET", "127.0.0.1:47821", "", 200},
		{"localhost GET", "GET", "localhost:47821", "", 200},
		{"DNS rebinding", "GET", "evil.example:47821", "", 403},
		{"POST without header (CSRF)", "POST", "127.0.0.1:47821", "", 403},
		{"POST with header", "POST", "127.0.0.1:47821", "1", 200},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "/api/focus/x", nil)
		r.Host = c.host
		if c.header != "" {
			r.Header.Set("X-Claude-Monitor", c.header)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: got %d want %d", c.name, w.Code, c.want)
		}
		if w.Code == 200 && w.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: missing anti-framing header", c.name)
		}
	}
}
