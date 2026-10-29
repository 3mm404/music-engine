package control

import (
	"testing"
	"time"
)

func TestCPUPercentageIsNormalizedAndBounded(t *testing.T) {
	if cpuPercentage(time.Second, time.Second, 4) != 25 {
		t.Fatal("CPU is not normalized across processors")
	}
	if cpuPercentage(5*time.Second, time.Second, 1) != 100 {
		t.Fatal("CPU exceeded 100 percent")
	}
	if cpuPercentage(time.Second, 0, 4) != 0 {
		t.Fatal("invalid sampling interval accepted")
	}
}

func TestPerformanceSamplesHeapAndUptime(t *testing.T) {
	sampler := performanceSampler{}
	now := time.Now()
	first := sampler.sample(now)
	second := sampler.sample(now.Add(2 * time.Second))
	if first.HeapBytes == 0 || first.UptimeSeconds != 0 || second.UptimeSeconds != 2 {
		t.Fatal("invalid performance sample")
	}
}
