package collector

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes = kernel32.NewProc("GetSystemTimes")
	procGlobalMemory   = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type cpuSampler struct{ total, idle uint64 }

func ft(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

func (c *cpuSampler) sample() Perf {
	p := Perf{Available: true, Label: "Windows"}
	var idle, kernel, user windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))); r != 0 {
		total := ft(kernel) + ft(user) // kernel incluye idle
		id := ft(idle)
		if c.total > 0 && total > c.total {
			p.CPU = int(100 - (id-c.idle)*100/(total-c.total))
		}
		c.total, c.idle = total, id
	}
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemory.Call(uintptr(unsafe.Pointer(&m))); r != 0 && m.TotalPhys > 0 {
		used := float64(m.TotalPhys - m.AvailPhys)
		p.RAMUsedGB = round2(used / (1 << 30))
		p.RAMTotalGB = round2(float64(m.TotalPhys) / (1 << 30))
		p.RAMPct = int(m.MemoryLoad)
	}
	return p
}
