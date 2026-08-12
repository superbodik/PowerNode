package api

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// hostMemoryMB reads the host's real total and currently-available RAM from
// /proc/meminfo (MemAvailable, not MemFree -- MemFree alone excludes
// reclaimable page cache and dramatically understates what's actually
// usable, which would make a healthy box look memory-starved). Linux-only,
// same as the rest of this installer/daemon's assumptions; returns zeros
// (silently) if unreadable rather than failing the health check over it.
func hostMemoryMB() (totalMB, availableMB int64) {
	return parseMeminfo("/proc/meminfo")
}

func parseMeminfo(path string) (totalMB, availableMB int64) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		var target *int64
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			target = &totalMB
		case strings.HasPrefix(line, "MemAvailable:"):
			target = &availableMB
		default:
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		*target = kb / 1024
	}
	return totalMB, availableMB
}
