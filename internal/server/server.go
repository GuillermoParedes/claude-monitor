// Package server expone el estado en http://127.0.0.1:<port>:
//
//	GET  /                  panel web (?panel=1 => modo ventana flotante)
//	GET  /api/state         State en JSON
//	GET  /state.txt         State en KEY=value (skin de Rainmeter)
//	POST /api/focus/{id}    ir a la sesion (herdr + traer la terminal al frente)
//	POST /api/folder/{id}   abrir la carpeta del proyecto
//	POST /api/terminal/{id} abrir una terminal nueva en el proyecto
//	POST /api/panel/close   pedir al panel abierto que se cierre
//	GET  /api/panel/seen    ms desde el ultimo latido del panel
//
// Seguridad: solo escucha en loopback, rechaza Host ajenos (DNS rebinding) y
// las acciones exigen la cabecera X-Claude-Monitor, que un sitio web no puede
// enviar a otro origen sin un preflight CORS que nunca aprobamos.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/collector"
	"github.com/GuillermoParedes/claude-monitor/internal/focus"
)

//go:embed web
var webFS embed.FS

type Server struct {
	col  *collector.Collector
	port int

	mu         sync.Mutex
	panelSeen  time.Time
	panelClose bool
}

func New(col *collector.Collector, port int) *Server { return &Server{col: col, port: port} }

// Listen arranca el servidor. Devuelve error si el puerto esta ocupado.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(sub))
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /state.txt", s.handleText)
	mux.HandleFunc("POST /api/focus/{id}", s.action(func(id string) (any, error) { return focus.Session(id) }))
	mux.HandleFunc("POST /api/folder/{id}", s.action(func(id string) (any, error) { return nil, focus.OpenFolder(id) }))
	mux.HandleFunc("POST /api/terminal/{id}", s.action(func(id string) (any, error) { return nil, focus.OpenTerminal(id) }))
	mux.HandleFunc("POST /api/panel/close", s.handlePanelClose)
	mux.HandleFunc("GET /api/panel/seen", s.handlePanelSeen)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"ok": "1", "version": collector.Version})
	})
	srv := &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	return nil
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-Claude-Monitor") != "1" {
			http.Error(w, "missing header", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// El panel tiene acciones (ir a sesion, abrir terminal): que ningun
		// sitio lo pueda cargar en un iframe para engañar clics.
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("panel") == "1" {
		s.mu.Lock()
		s.panelSeen = time.Now()
		if s.panelClose {
			s.panelClose = false
			w.Header().Set("X-Close-Panel", "1")
		}
		s.mu.Unlock()
	}
	st := s.col.State()
	if st == nil {
		http.Error(w, "warming up", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, st)
}

func (s *Server) handleText(w http.ResponseWriter, r *http.Request) {
	st := s.col.State()
	if st == nil {
		http.Error(w, "warming up", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(st.Text()))
}

func (s *Server) handlePanelClose(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.panelClose = true
	s.mu.Unlock()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handlePanelSeen(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ms := int64(-1)
	if !s.panelSeen.IsZero() {
		ms = time.Since(s.panelSeen).Milliseconds()
	}
	s.mu.Unlock()
	writeJSON(w, map[string]int64{"ms": ms})
}

func (s *Server) action(f func(id string) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" || strings.ContainsAny(id, `/\.`) {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		res, err := f(id)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		if res == nil {
			res = map[string]bool{"ok": true}
		}
		writeJSON(w, res)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
