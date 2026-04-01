package api

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// GetCPU returns CPU usage percentage by sampling /proc/stat twice.
func GetCPU() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	idle1, total1 := readCPUStat()
	if total1 == 0 {
		return 0
	}
	time.Sleep(500 * time.Millisecond)
	idle2, total2 := readCPUStat()
	if total2 == total1 {
		return 0
	}
	usage := (1.0 - float64(idle2-idle1)/float64(total2-total1)) * 100.0
	return float64(int(usage*10)) / 10
}

func readCPUStat() (idle, total uint64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0
	}
	var values []uint64
	for _, f := range fields[1:] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		values = append(values, v)
	}
	if len(values) < 4 {
		return 0, 0
	}
	for _, v := range values {
		total += v
	}
	idle = values[3]
	return idle, total
}

// GetMemory returns memory usage percentage from /proc/meminfo.
func GetMemory() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var memTotal, memAvailable uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		}
	}
	if memTotal == 0 {
		return 0
	}
	usage := float64(memTotal-memAvailable) / float64(memTotal) * 100.0
	return float64(int(usage*10)) / 10
}

// GetDisk returns root filesystem usage percentage.
func GetDisk() float64 {
	out, err := exec.Command("bash", "-c", "df / | tail -1 | awk '{print $5}' | tr -d '%'").CombinedOutput()
	if err != nil {
		return 0
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return val
}

// GetUptime returns system uptime in seconds.
func GetUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	s := fields[0]
	if idx := strings.Index(s, "."); idx > 0 {
		s = s[:idx]
	}
	val, _ := strconv.ParseInt(s, 10, 64)
	return val
}
