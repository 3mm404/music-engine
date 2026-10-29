package control

import (
	"runtime"
	"runtime/metrics"
	"time"
)

// CPU is normalized to the machine's logical processors. HeapBytes measures
// live Go allocations, not total system memory or the process working set.
type DesktopPerformance struct {
	CPUPercent    *float64 `json:"cpu_percent"`
	HeapBytes     uint64   `json:"heap_bytes"`
	UptimeSeconds int64    `json:"uptime_seconds"`
}

type performanceSampler struct {
	started  time.Time
	previous time.Time
	cpu      time.Duration
}

func (s *performanceSampler) sample(now time.Time) *DesktopPerformance {
	if s.started.IsZero() {
		s.started = now
	}
	samples := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(samples)
	result := &DesktopPerformance{HeapBytes: samples[0].Value.Uint64(), UptimeSeconds: int64(now.Sub(s.started).Seconds())}
	if cpu, ok := processCPUTime(); ok {
		if !s.previous.IsZero() && now.After(s.previous) {
			percent := cpuPercentage(cpu-s.cpu, now.Sub(s.previous), runtime.NumCPU())
			result.CPUPercent = &percent
		}
		s.cpu = cpu
		s.previous = now
	}
	return result
}

func cpuPercentage(cpu, elapsed time.Duration, processors int) float64 {
	if elapsed <= 0 || cpu < 0 || processors < 1 {
		return 0
	}
	return min(100, 100*float64(cpu)/float64(elapsed)/float64(processors))
}
