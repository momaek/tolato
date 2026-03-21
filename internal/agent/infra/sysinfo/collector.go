package sysinfo

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type busySource interface {
	IsBusy() bool
}

type Snapshot struct {
	CPU    float64
	Memory float64
	Disk   float64
	Busy   bool
}

type cpuSample struct {
	idle  uint64
	total uint64
}

type Collector struct {
	mu         sync.Mutex
	prev       cpuSample
	hasPrev    bool
	busySource busySource
}

func NewCollector(busy busySource) *Collector {
	return &Collector{busySource: busy}
}

func (c *Collector) Snapshot() (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if runtime.GOOS != "linux" {
		return Snapshot{
			Busy: c.busySource != nil && c.busySource.IsBusy(),
		}, nil
	}

	cpuUsage, err := c.cpuUsage()
	if err != nil {
		return Snapshot{}, err
	}

	memoryUsage, err := memoryUsage()
	if err != nil {
		return Snapshot{}, err
	}

	diskUsage, err := diskUsage("/")
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		CPU:    cpuUsage,
		Memory: memoryUsage,
		Disk:   diskUsage,
		Busy:   c.busySource != nil && c.busySource.IsBusy(),
	}, nil
}

func (c *Collector) cpuUsage() (float64, error) {
	current, err := readCPUSample()
	if err != nil {
		return 0, err
	}

	if !c.hasPrev {
		c.prev = current
		c.hasPrev = true
		return 0, nil
	}

	totalDelta := current.total - c.prev.total
	idleDelta := current.idle - c.prev.idle
	c.prev = current

	if totalDelta == 0 {
		return 0, nil
	}

	usage := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	if usage < 0 {
		return 0, nil
	}
	return usage, nil
}

func readCPUSample() (cpuSample, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return cpuSample{}, err
		}
		return cpuSample{}, fmt.Errorf("missing cpu line in /proc/stat")
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSample{}, fmt.Errorf("invalid cpu line in /proc/stat")
	}

	var total uint64
	values := make([]uint64, 0, len(fields)-1)
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuSample{}, err
		}
		total += value
		values = append(values, value)
	}

	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}

	return cpuSample{idle: idle, total: total}, nil
}

func memoryUsage() (float64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer file.Close()

	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	total := values["MemTotal"]
	if total == 0 {
		return 0, nil
	}

	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}

	used := total - available
	return float64(used) / float64(total) * 100, nil
}

func diskUsage(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}

	if stat.Blocks == 0 {
		return 0, nil
	}

	used := stat.Blocks - stat.Bfree
	return float64(used) / float64(stat.Blocks) * 100, nil
}
