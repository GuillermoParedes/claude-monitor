package collector

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

// Price en USD por millon de tokens. cache read = In x 0.1, cache write = In x 1.25.
type Price struct {
	Match string  `json:"match"`
	In    float64 `json:"in"`
	Out   float64 `json:"out"`
}

// DefaultPrices: el primer `match` contenido en el id del modelo gana.
// Editables en <DataDir>/prices.json (se recargan solos).
var DefaultPrices = []Price{
	{"fable", 10, 50},
	{"mythos", 10, 50},
	{"opus", 5, 25},
	{"sonnet", 3, 15},
	{"haiku", 1, 5},
}

var fallbackPrice = Price{"", 5, 25}

type priceTable struct {
	prices []Price
	mtime  int64
}

// reload devuelve true si la tabla cambio respecto a la ya cargada.
func (t *priceTable) reload() bool {
	st, err := os.Stat(paths.PricesFile())
	if err != nil {
		b, _ := json.MarshalIndent(DefaultPrices, "", "  ")
		_ = os.WriteFile(paths.PricesFile(), append(b, '\n'), 0o600)
		if st, err = os.Stat(paths.PricesFile()); err == nil {
			t.mtime = st.ModTime().UnixNano()
		}
		if t.prices == nil {
			t.prices = DefaultPrices
		}
		return false
	}
	if st.ModTime().UnixNano() == t.mtime {
		return false
	}
	first := t.prices == nil
	t.mtime = st.ModTime().UnixNano()
	var p []Price
	b, _ := os.ReadFile(paths.PricesFile())
	if json.Unmarshal(b, &p) == nil && len(p) > 0 {
		t.prices = p
	} else if t.prices == nil {
		t.prices = DefaultPrices
	}
	return !first
}

func (t *priceTable) forModel(model string) Price {
	m := strings.ToLower(model)
	for _, p := range t.prices {
		if p.Match != "" && strings.Contains(m, strings.ToLower(p.Match)) {
			return p
		}
	}
	return fallbackPrice
}

type usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

func (u usage) total() int64 {
	return u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

func (t *priceTable) cost(u usage, model string) float64 {
	p := t.forModel(model)
	return (float64(u.InputTokens)*p.In +
		float64(u.OutputTokens)*p.Out +
		float64(u.CacheReadInputTokens)*p.In*0.1 +
		float64(u.CacheCreationInputTokens)*p.In*1.25) / 1e6
}
