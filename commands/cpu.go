package commands

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func init() {
	Register(&Command{
		Name:        "cpu",
		Description: "Show system CPU information and usage",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleCPU,
	})
}

func handleCPU(ctx *Context) error {
	// 1. CPU Model
	model := getCPUModel()

	// 2. Cores
	cores := runtime.NumCPU()

	// 3. Load Average
	loadAvg := getLoadAvg()

	// 4. CPU Usage
	usageStr := "Unknown"
	if u, err := getCPUUsage(); err == nil {
		usageStr = fmt.Sprintf("%.2f%%", u)
	}

	res := fmt.Sprintf("💻 *CPU Information*\n\n"+
		"• *Model:* %s\n"+
		"• *Cores/Threads:* %d\n"+
		"• *Load Average:* %s\n"+
		"• *Current Usage:* %s",
		model, cores, loadAvg, usageStr)

	return ctx.Reply(res)
}

func getCPUModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "Unknown"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "Generic CPU"
}

func getLoadAvg() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "Unknown"
	}
	parts := strings.Fields(string(data))
	if len(parts) >= 3 {
		return strings.Join(parts[:3], ", ")
	}
	return strings.TrimSpace(string(data))
}

type cpuStats struct {
	user, nice, system, idle, iowait, irq, softirq, steal uint64
}

func getCPUStats() (cpuStats, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStats{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "cpu" {
			var s cpuStats
			var err error
			s.user, err = strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return cpuStats{}, err
			}
			s.nice, _ = strconv.ParseUint(fields[2], 10, 64)
			s.system, _ = strconv.ParseUint(fields[3], 10, 64)
			s.idle, _ = strconv.ParseUint(fields[4], 10, 64)
			if len(fields) > 5 {
				s.iowait, _ = strconv.ParseUint(fields[5], 10, 64)
			}
			if len(fields) > 6 {
				s.irq, _ = strconv.ParseUint(fields[6], 10, 64)
			}
			if len(fields) > 7 {
				s.softirq, _ = strconv.ParseUint(fields[7], 10, 64)
			}
			if len(fields) > 8 {
				s.steal, _ = strconv.ParseUint(fields[8], 10, 64)
			}
			return s, nil
		}
	}
	return cpuStats{}, fmt.Errorf("could not parse /proc/stat")
}

func getCPUUsage() (float64, error) {
	s1, err := getCPUStats()
	if err != nil {
		return 0, err
	}
	time.Sleep(200 * time.Millisecond)
	s2, err := getCPUStats()
	if err != nil {
		return 0, err
	}

	idle1 := s1.idle + s1.iowait
	idle2 := s2.idle + s2.iowait

	nonIdle1 := s1.user + s1.nice + s1.system + s1.irq + s1.softirq + s1.steal
	nonIdle2 := s2.user + s2.nice + s2.system + s2.irq + s2.softirq + s2.steal

	total1 := idle1 + nonIdle1
	total2 := idle2 + nonIdle2

	totalDiff := total2 - total1
	idleDiff := idle2 - idle1

	if totalDiff == 0 {
		return 0, nil
	}

	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100, nil
}
