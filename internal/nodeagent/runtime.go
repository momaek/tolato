package nodeagent

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	infraws "github.com/momaek/tolato/internal/server/infra/ws"
)

func collectRuntime(busy bool) infraws.AgentNodeRuntime {
	return infraws.AgentNodeRuntime{
		Busy: busy,
		Metrics: infraws.AgentNodeMetrics{
			// NOTE: cpuLoadFraction and memoryUsageFraction read from /proc and
			// only return meaningful values on Linux. On other platforms they
			// silently return 0. diskUsageFraction uses syscall.Statfs which
			// works on both Linux and macOS.
			CPU:    cpuLoadFraction(),
			Memory: memoryUsageFraction(),
			Disk:   diskUsageFraction("/"),
		},
	}
}

func cpuLoadFraction() float64 {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0
	}
	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || runtime.NumCPU() == 0 {
		return 0
	}
	return clamp(load/float64(runtime.NumCPU()), 0, 1)
}

func memoryUsageFraction() float64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	var total, available float64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseMeminfoValue(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			available = parseMeminfoValue(line)
		}
	}
	if total <= 0 {
		return 0
	}
	return clamp((total-available)/total, 0, 1)
}

func diskUsageFraction(path string) float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil || stat.Blocks == 0 {
		return 0
	}
	used := float64(stat.Blocks - stat.Bavail)
	total := float64(stat.Blocks)
	return clamp(used/total, 0, 1)
}

func parseMeminfoValue(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	value, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0
	}
	return value
}

func clamp(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
