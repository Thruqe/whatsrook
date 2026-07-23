// Memory command – displays system memory usage information.
package commands

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func init() {
	Register(&Command{
		Name:        "memory",
		Description: "Show system and process memory usage",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleMemory,
	})
}

func handleMemory(ctx *Context) error {
	// 1. Process Memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	procAlloc := float64(m.Alloc) / 1024 / 1024
	procSys := float64(m.Sys) / 1024 / 1024

	// 2. System Memory
	sysMem, err := getSystemMemory()
	var sysInfo string
	if err != nil {
		sysInfo = fmt.Sprintf("• System Memory: Error reading (%v)\n", err)
	} else {
		totalGB := float64(sysMem.total) / 1024 / 1024
		availableGB := float64(sysMem.available) / 1024 / 1024
		usedGB := totalGB - availableGB
		percent := (usedGB / totalGB) * 100

		sysInfo = fmt.Sprintf(
			"• Total System Memory: %.2f GB\n"+
				"• Used System Memory: %.2f GB (%.1f%%)\n"+
				"• Available System Memory: %.2f GB\n",
			totalGB, usedGB, percent, availableGB)
	}

	res := fmt.Sprintf("Memory Information\n\n"+
		"%s"+
		"• Process Allocated: %.2f MB\n"+
		"• Process System Reserved: %.2f MB",
		sysInfo, procAlloc, procSys)

	return ctx.Reply(res)
}

type systemMemory struct {
	total     uint64
	free      uint64
	available uint64
}

func getSystemMemory() (systemMemory, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return systemMemory{}, err
	}
	defer f.Close()

	var mem systemMemory
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			mem.total = val
		case "MemFree":
			mem.free = val
		case "MemAvailable":
			mem.available = val
		}
	}

	if err := scanner.Err(); err != nil {
		return systemMemory{}, err
	}

	if mem.total == 0 {
		return systemMemory{}, fmt.Errorf("could not parse system memory total")
	}
	// Fallback if MemAvailable is not reported
	if mem.available == 0 {
		mem.available = mem.free
	}

	return mem, nil
}
