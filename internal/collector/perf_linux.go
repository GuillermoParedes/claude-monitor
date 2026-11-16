package collector

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

type cpuSampler struct{ total, idle uint64 }

var reIO = regexp.MustCompile(`some\s+avg10=([\d.]+)`)

func (c *cpuSampler) sample() Perf {
	p := Perf{Available: true, Label: "Linux"}
	if paths.IsWSL() {
		p.Label = "WSL2"
	}
	if b, err := os.ReadFile("/proc/stat"); err == nil {
		first := strings.SplitN(string(b), "\n", 2)[0]
		f := strings.Fields(first)
		var total, idle uint64
		for i, s := range f[1:] {
			v, _ := strconv.ParseUint(s, 10, 64)
			total += v
			if i == 3 || i == 4 { // idle + iowait
				idle += v
			}
		}
		if c.total > 0 && total > c.total {
			p.CPU = int(100 - (idle-c.idle)*100/(total-c.total))
		}
		c.total, c.idle = total, idle
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		get := func(k string) float64 {
			re := regexp.MustCompile(`(?m)^` + k + `:\s+(\d+) kB`)
			m := re.FindStringSubmatch(string(b))
			if m == nil {
				return 0
			}
			v, _ := strconv.ParseFloat(m[1], 64)
			return v
		}
		total, avail := get("MemTotal"), get("MemAvailable")
		if total > 0 {
			p.RAMUsedGB = round2((total - avail) / 1048576)
			p.RAMTotalGB = round2(total / 1048576)
			p.RAMPct = int((total - avail) * 100 / total)
		}
	}
	if b, err := os.ReadFile("/proc/pressure/io"); err == nil {
		if m := reIO.FindStringSubmatch(string(b)); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			p.IOPressure = int(v + 0.5)
		}
	}
	return p
}
