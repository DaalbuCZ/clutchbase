package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResourceSamplerUsesDeltasAndRejectsStaleCounters(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	sampler := resourceSampler{}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("meminfo", "MemTotal: 1000 kB\nMemAvailable: 250 kB\n")
	write("stat", "cpu 10 0 10 80 0 0 0 0 9 0\ncpu0 1 2 3 4\n")
	first := sampler.sample(root, now)
	if first == nil || first.CPUPercent != nil || first.MemoryTotalBytes != 1024000 || first.MemoryAvailableBytes != 256000 {
		t.Fatalf("first sample: %+v", first)
	}
	write("stat", "cpu 20 0 20 160 0 0 0 0 18 0\n")
	second := sampler.sample(root, now.Add(5*time.Second))
	if second == nil || second.CPUPercent == nil || *second.CPUPercent != 20 {
		t.Fatalf("guest time was double counted or delta incorrect: %+v", second)
	}
	write("stat", "cpu 1 0 1 8 0 0 0 0\n")
	if reset := sampler.sample(root, now.Add(10*time.Second)); reset == nil || reset.CPUPercent != nil {
		t.Fatalf("counter reset reported usage: %+v", reset)
	}
	write("stat", "cpu 2 0 2 16 0 0 0 0\n")
	if stale := sampler.sample(root, now.Add(time.Minute)); stale == nil || stale.CPUPercent != nil {
		t.Fatalf("old baseline reported current usage: %+v", stale)
	}
	write("stat", "cpu broken\n")
	if broken := sampler.sample(root, now.Add(65*time.Second)); broken == nil || broken.CPUPercent != nil {
		t.Fatalf("broken CPU became a zero reading: %+v", broken)
	}
	write("meminfo", "MemTotal: 1000 kB\nMemFree: 250 kB\n")
	if sampler.sample(root, now.Add(70*time.Second)) != nil {
		t.Fatal("missing MemAvailable was fabricated from MemFree")
	}
}

func TestResourceParsersRejectMalformedInput(t *testing.T) {
	for _, body := range []string{"", "cpu0 1 2 3 4", "cpu 1 2", "cpu 1 -2 3 4", "cpu 18446744073709551615 1 0 0"} {
		if parseCPU([]byte(body), time.Now()) != nil {
			t.Fatalf("accepted malformed CPU: %s", body)
		}
	}
	for _, body := range []string{"", "MemTotal: 1 MB\nMemAvailable: 0 kB", "MemTotal: 1 kB\nMemAvailable: 2 kB", "MemTotal: 0 kB\nMemAvailable: 0 kB", "MemTotal: 9223372036854775807 kB\nMemAvailable: 0 kB", "MemTotal: 1 kB\nMemTotal: 2 kB\nMemAvailable: 0 kB"} {
		if parseMemory([]byte(body)) != nil {
			t.Fatalf("accepted malformed memory: %s", body)
		}
	}
}
