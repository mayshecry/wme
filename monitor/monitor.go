package monitor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HostStats struct {
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryTotal uint64    `json:"memory_total"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryFree  uint64    `json:"memory_free"`
	MemoryUsage float64   `json:"memory_usage"`
	DiskTotal   uint64    `json:"disk_total"`
	DiskUsed    uint64    `json:"disk_used"`
	DiskFree    uint64    `json:"disk_free"`
	DiskUsage   float64   `json:"disk_usage"`
	NetRx       uint64    `json:"net_rx"`
	NetTx       uint64    `json:"net_tx"`
	NetRxRate  float64   `json:"net_rx_rate"`
	NetTxRate  float64   `json:"net_tx_rate"`
	LoadAvg     [3]float64 `json:"load_avg"`
	Uptime      uint64    `json:"uptime"`
	Processes   int       `json:"processes"`
	Timestamp   time.Time `json:"timestamp"`
}

type VMStats struct {
	Name       string    `json:"name"`
	CPUUsage   float64   `json:"cpu_usage"`
	MemoryUsed uint64    `json:"memory_used"`
	MemoryMax  uint64    `json:"memory_max"`
	NetRx      uint64    `json:"net_rx"`
	NetTx      uint64    `json:"net_tx"`
	DiskRead   uint64    `json:"disk_read"`
	DiskWrite  uint64    `json:"disk_write"`
	Timestamp  time.Time `json:"timestamp"`
}

type Monitor struct {
	mu          sync.RWMutex
	lastCPUTime uint64
	lastCPUIdle uint64
	lastNetRx   map[string]uint64
	lastNetTx   map[string]uint64
	lastTime    time.Time
}

func NewMonitor() *Monitor {
	return &Monitor{
		lastNetRx: make(map[string]uint64),
		lastNetTx: make(map[string]uint64),
		lastTime:  time.Now(),
	}
}

func (m *Monitor) GetHostStats() (HostStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var stats HostStats
	stats.Timestamp = time.Now()

	cpuTotal, cpuIdle, err := readCPUStats()
	if err != nil {
		return stats, err
	}

	now := time.Now()
	dt := now.Sub(m.lastTime).Seconds()
	if dt > 0 {
		totalDiff := cpuTotal - m.lastCPUTime
		idleDiff := cpuIdle - m.lastCPUIdle
		if totalDiff > 0 {
			stats.CPUUsage = (1 - float64(idleDiff)/float64(totalDiff)) * 100
		}
	}
	m.lastCPUTime = cpuTotal
	m.lastCPUIdle = cpuIdle
	m.lastTime = now

	memTotal, _, memAvail, err := readMemStats()
	if err != nil {
		return stats, err
	}
	stats.MemoryTotal = memTotal
	stats.MemoryFree = memAvail
	stats.MemoryUsed = memTotal - memAvail
	if memTotal > 0 {
		stats.MemoryUsage = float64(stats.MemoryUsed) / float64(memTotal) * 100
	}

	diskTotal, diskUsed, err := readDiskStats()
	if err != nil {
		return stats, err
	}
	stats.DiskTotal = diskTotal
	stats.DiskUsed = diskUsed
	stats.DiskFree = diskTotal - diskUsed
	if diskTotal > 0 {
		stats.DiskUsage = float64(diskUsed) / float64(diskTotal) * 100
	}

	netRx, netTx, err := readNetStats()
	if err != nil {
		return stats, err
	}
	stats.NetRx = netRx
	stats.NetTx = netTx
	if prevRx, ok := m.lastNetRx["total"]; ok && dt > 0 {
		stats.NetRxRate = float64(netRx-prevRx) / dt
	}
	if prevTx, ok := m.lastNetTx["total"]; ok && dt > 0 {
		stats.NetTxRate = float64(netTx-prevTx) / dt
	}
	m.lastNetRx["total"] = netRx
	m.lastNetTx["total"] = netTx

	load, err := readLoadAvg()
	if err == nil {
		stats.LoadAvg = load
	}

	uptime, err := readUptime()
	if err == nil {
		stats.Uptime = uptime
	}

	procs, err := readProcessCount()
	if err == nil {
		stats.Processes = procs
	}

	return stats, nil
}

func (m *Monitor) GetVMStats(name string) (VMStats, error) {
	var stats VMStats
	stats.Name = name
	stats.Timestamp = time.Now()

	cpuUsage, err := readVMCPUUsage(name)
	if err == nil {
		stats.CPUUsage = cpuUsage
	}

	memUsed, memMax, err := readVMMemory(name)
	if err == nil {
		stats.MemoryUsed = memUsed
		stats.MemoryMax = memMax
	}

	netRx, netTx, err := readVMNet(name)
	if err == nil {
		stats.NetRx = netRx
		stats.NetTx = netTx
	}

	diskRead, diskWrite, err := readVMDisk(name)
	if err == nil {
		stats.DiskRead = diskRead
		stats.DiskWrite = diskWrite
	}

	return stats, nil
}

func readCPUStats() (uint64, uint64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0, 0, fmt.Errorf("empty /proc/stat")
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 5 {
		return 0, 0, fmt.Errorf("invalid /proc/stat")
	}
	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		if i == 4 {
			idle = val
		}
	}
	return total, idle, nil
}

func readMemStats() (uint64, uint64, uint64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer file.Close()

	var total, free, avail uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = val * 1024
		case "MemFree:":
			free = val * 1024
		case "MemAvailable:":
			avail = val * 1024
		}
	}
	if avail == 0 {
		avail = free
	}
	return total, free, avail, nil
}

func readDiskStats() (uint64, uint64, error) {
	cmd := exec.Command("df", "-B1", "/")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("invalid df output")
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, 0, fmt.Errorf("invalid df output")
	}
	total, _ := strconv.ParseUint(fields[1], 10, 64)
	used, _ := strconv.ParseUint(fields[2], 10, 64)
	return total, used, nil
}

func readNetStats() (uint64, uint64, error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	var rx, tx uint64
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rxVal, _ := strconv.ParseUint(fields[0], 10, 64)
		txVal, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += rxVal
		tx += txVal
	}
	return rx, tx, nil
}

func readLoadAvg() ([3]float64, error) {
	var load [3]float64
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return load, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return load, fmt.Errorf("invalid loadavg")
	}
	for i := 0; i < 3; i++ {
		load[i], _ = strconv.ParseFloat(fields[i], 64)
	}
	return load, nil
}

func readUptime() (uint64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid uptime")
	}
	secs, _ := strconv.ParseFloat(fields[0], 64)
	return uint64(secs), nil
}

func readProcessCount() (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			if _, err := strconv.Atoi(e.Name()); err == nil {
				count++
			}
		}
	}
	return count, nil
}

func readVMCPUUsage(name string) (float64, error) {
	cmd := exec.Command("virsh", "domstats", name, "--cpu-total")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "cpu.time") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				return float64(val) / 1e9, nil
			}
		}
	}
	return 0, nil
}

func readVMMemory(name string) (uint64, uint64, error) {
	cmd := exec.Command("virsh", "dommemstat", name)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}
	var unused, max uint64
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "unused":
			unused = val * 1024
		case "available":
			max = val * 1024
		}
	}
	var used uint64
	if max > unused {
		used = max - unused
	}
	return used, max, nil
}

func readVMNet(name string) (uint64, uint64, error) {
	cmd := exec.Command("virsh", "domifstat", name, "vnet0")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}
	var rx, tx uint64
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		val, _ := strconv.ParseUint(fields[2], 10, 64)
		switch fields[1] {
		case "rx_bytes":
			rx = val
		case "tx_bytes":
			tx = val
		}
	}
	return rx, tx, nil
}

func readVMDisk(name string) (uint64, uint64, error) {
	cmd := exec.Command("virsh", "domstats", name, "--block")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}
	var read, write uint64
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "rd.reqs") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				read, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
		if strings.Contains(line, "wr.reqs") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				write, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}
	return read, write, nil
}