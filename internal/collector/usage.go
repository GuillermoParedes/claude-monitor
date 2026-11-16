package collector

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

const (
	cacheVersion = 3
	keepDays     = 30
)

// Agg acumula tokens y costo.
type Agg struct {
	Tokens int64   `json:"t"`
	Cost   float64 `json:"c"`
}

type dayAgg struct {
	Agg
	Hours     [24]float64     `json:"h"`
	BySession map[string]*Agg `json:"s"`
	ByProject map[string]*Agg `json:"p"`
	ByModel   map[string]*Agg `json:"m"`
}

type fileMark struct {
	Offset int64 `json:"o"`
	Size   int64 `json:"s"`
}

// usageCache es incremental: por cada .jsonl recordamos hasta donde lo leimos.
type usageCache struct {
	V     int                  `json:"v"`
	Files map[string]*fileMark `json:"files"`
	Seen  map[string]string    `json:"seen"` // message.id -> dia (dedupe global)
	Days  map[string]*dayAgg   `json:"days"`
	dirty bool
}

func newCache() *usageCache {
	return &usageCache{V: cacheVersion, Files: map[string]*fileMark{}, Seen: map[string]string{}, Days: map[string]*dayAgg{}, dirty: true}
}

func loadCache() *usageCache {
	b, err := os.ReadFile(paths.CacheFile())
	if err != nil {
		return newCache()
	}
	var c usageCache
	if json.Unmarshal(b, &c) != nil || c.V != cacheVersion || c.Files == nil || c.Days == nil || c.Seen == nil {
		return newCache()
	}
	return &c
}

func (c *usageCache) save() {
	if !c.dirty {
		return
	}
	days := make([]string, 0, len(c.Days))
	for d := range c.Days {
		days = append(days, d)
	}
	sort.Strings(days)
	if len(days) > keepDays {
		for _, d := range days[:len(days)-keepDays] {
			delete(c.Days, d)
		}
		oldest := days[len(days)-keepDays]
		for id, d := range c.Seen {
			if d < oldest {
				delete(c.Seen, id)
			}
		}
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	tmp := paths.CacheFile() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, paths.CacheFile()) == nil {
		c.dirty = false
	}
}

func (c *usageCache) reset() {
	*c = *newCache()
}

// line es lo minimo que parseamos de cada linea del transcript.
type line struct {
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Cwd       string `json:"cwd"`
	Message   *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *usage `json:"usage"`
	} `json:"message"`
}

func bump(m map[string]*Agg, k string, tok int64, cost float64) {
	a := m[k]
	if a == nil {
		a = &Agg{}
		m[k] = a
	}
	a.Tokens += tok
	a.Cost += cost
}

func (c *usageCache) account(l *line, prices *priceTable) {
	if l.Message == nil || l.Message.Usage == nil {
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		ts = time.Now()
	}
	ts = ts.Local()
	day := ts.Format("2006-01-02")

	// Dedupe: Claude Code escribe una linea por bloque de contenido, todas con
	// el MISMO usage; y un --resume copia el historial a otro transcript.
	id := l.Message.ID
	if id == "" {
		id = l.RequestID
	}
	if id != "" {
		if _, ok := c.Seen[id]; ok {
			return
		}
		c.Seen[id] = day
	}

	u := *l.Message.Usage
	tok := u.total()
	if tok == 0 {
		return
	}
	cost := prices.cost(u, l.Message.Model)

	d := c.Days[day]
	if d == nil {
		d = &dayAgg{BySession: map[string]*Agg{}, ByProject: map[string]*Agg{}, ByModel: map[string]*Agg{}}
		c.Days[day] = d
	}
	d.Tokens += tok
	d.Cost += cost
	d.Hours[ts.Hour()] += cost
	if l.SessionID != "" {
		bump(d.BySession, l.SessionID, tok, cost)
	}
	proj := "?"
	if l.Cwd != "" {
		proj = filepath.Base(l.Cwd)
	}
	bump(d.ByProject, proj, tok, cost)
	bump(d.ByModel, ShortModel(l.Message.Model), tok, cost)
	c.dirty = true
}

var errRebuild = errors.New("rebuild")

var usageKey = []byte(`"usage"`)

func (c *usageCache) scanFile(path string, prices *priceTable) error {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	mark := c.Files[path]
	var off int64
	if mark != nil {
		if st.Size() < mark.Size {
			return errRebuild // truncado o reescrito
		}
		off = mark.Offset
	}
	if st.Size() <= off {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil
	}

	r := bufio.NewReaderSize(f, 256*1024)
	read := off
	for {
		b, err := r.ReadBytes('\n')
		if err != nil {
			break // linea incompleta (o EOF): se relee en la proxima pasada
		}
		read += int64(len(b))
		if !bytes.Contains(b, usageKey) { // filtro barato antes del JSON
			continue
		}
		var l line
		if json.Unmarshal(b, &l) == nil {
			c.account(&l, prices)
		}
	}
	c.Files[path] = &fileMark{Offset: read, Size: st.Size()}
	c.dirty = true
	return nil
}

func (c *usageCache) scanAll(prices *priceTable) error {
	return filepath.WalkDir(paths.ProjectsDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".jsonl") {
			return c.scanFile(p, prices)
		}
		return nil
	})
}

func (c *usageCache) update(prices *priceTable) {
	if err := c.scanAll(prices); errors.Is(err, errRebuild) {
		c.reset()
		_ = c.scanAll(prices)
	}
	c.save()
}

func (c *usageCache) day(t time.Time) *dayAgg {
	if d := c.Days[t.Format("2006-01-02")]; d != nil {
		return d
	}
	return &dayAgg{BySession: map[string]*Agg{}, ByProject: map[string]*Agg{}, ByModel: map[string]*Agg{}}
}

// top devuelve los n mayores por costo.
func top(m map[string]*Agg, n int) []Named {
	out := make([]Named, 0, len(m))
	for k, v := range m {
		out = append(out, Named{Name: k, Tokens: v.Tokens, Cost: round2(v.Cost)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
