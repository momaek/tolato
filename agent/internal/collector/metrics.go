package collector

import (
	"net"
	"runtime"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// SystemInfo contains static system information collected once at startup.
type SystemInfo struct {
	Hostname      string
	OS            string
	Kernel        string
	IP            string
	CPUCores      int
	MemoryTotalMB int
	DiskTotalGB   int
}

// Metrics contains dynamic system metrics collected periodically.
type Metrics struct {
	CPU     float64
	Memory  float64
	Disk    float64
	Uptime  int64
	LoadAvg [3]float64
}

// Collector gathers system information and metrics.
type Collector struct{}

// NewCollector creates a new Collector.
func NewCollector() *Collector {
	return &Collector{}
}

// GetSystemInfo collects static system information.
func (c *Collector) GetSystemInfo() *SystemInfo {
	info := &SystemInfo{
		CPUCores: runtime.NumCPU(),
	}

	// Hostname
	if h, err := host.Info(); err == nil {
		info.Hostname = h.Hostname
		info.OS = h.Platform + " " + h.PlatformVersion
		info.Kernel = h.KernelVersion
	}

	// Memory total
	if m, err := mem.VirtualMemory(); err == nil {
		info.MemoryTotalMB = int(m.Total / 1024 / 1024)
	}

	// Disk total
	if d, err := disk.Usage("/"); err == nil {
		info.DiskTotalGB = int(d.Total / 1024 / 1024 / 1024)
	}

	// IP: first non-loopback IPv4
	info.IP = getLocalIP()

	return info
}

// GetMetrics collects current system metrics.
func (c *Collector) GetMetrics() *Metrics {
	m := &Metrics{}

	// CPU usage (overall, non-blocking with 0 interval uses the delta since last call)
	if percents, err := cpu.Percent(0, false); err == nil && len(percents) > 0 {
		m.CPU = percents[0]
	}

	// Memory usage
	if vm, err := mem.VirtualMemory(); err == nil {
		m.Memory = vm.UsedPercent
	}

	// Disk usage
	if d, err := disk.Usage("/"); err == nil {
		m.Disk = d.UsedPercent
	}

	// Load average
	if l, err := load.Avg(); err == nil {
		m.LoadAvg = [3]float64{l.Load1, l.Load5, l.Load15}
	}

	// Uptime
	if u, err := host.Uptime(); err == nil {
		m.Uptime = int64(u)
	}

	return m
}

// getLocalIP returns the first non-loopback IPv4 address.
func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return ""
}
