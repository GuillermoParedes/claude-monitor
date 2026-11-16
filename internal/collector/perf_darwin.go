package collector

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type cpuSampler struct{}

var (
	reTopCPU  = regexp.MustCompile(`CPU usage:\s+([\d.]+)% user,\s+([\d.]+)% sys`)
	reVMPage  = regexp.MustCompile(`page size of (\d+) bytes`)
	reVMValue = regexp.MustCompile(`(?m)^(Pages (?:active|wired down|occupied by compressor)):\s+(\d+)`)
)

// sample usa herramientas del sistema (sin cgo). Es barato a 3 s por tick.
func (c *cpuSampler) sample() Perf {
	p := Perf{Available: true, Label: "macOS"}
	if out, err := exec.Command("top", "-l", "1", "-n", "0", "-s", "0").Output(); err == nil {
		if m := reTopCPU.FindStringSubmatch(string(out)); m != nil {
			u, _ := strconv.ParseFloat(m[1], 64)
			s, _ := strconv.ParseFloat(m[2], 64)
			p.CPU = int(u + s + 0.5)
		}
	}
	total := 0.0
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		total, _ = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	}
	if out, err := exec.Command("vm_stat").Output(); err == nil && total > 0 {
		page := 4096.0
		if m := reVMPage.FindStringSubmatch(string(out)); m != nil {
			page, _ = strconv.ParseFloat(m[1], 64)
		}
		used := 0.0
		for _, m := range reVMValue.FindAllStringSubmatch(string(out), -1) {
			v, _ := strconv.ParseFloat(m[2], 64)
			used += v * page
		}
		p.RAMUsedGB = round2(used / (1 << 30))
		p.RAMTotalGB = round2(total / (1 << 30))
		p.RAMPct = int(used * 100 / total)
	}
	return p
}
