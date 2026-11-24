package runner

import (
	"bytes"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"clutchbase/internal/protocol"
)

type cpuSample struct {
	total, idle uint64
	at          time.Time
}
type resourceSampler struct{ previous *cpuSample }

func readProc(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return nil
	}
	return data
}

func parseCPU(data []byte, at time.Time) *cpuSample {
	line, _, _ := bytes.Cut(data, []byte("\n"))
	fields := strings.Fields(string(line))
	if len(fields) < 5 || fields[0] != "cpu" {
		return nil
	}
	sample := &cpuSample{at: at}
	// Guest time is already included in user/nice; only the first eight counters count.
	for i := 1; i < len(fields) && i <= 8; i++ {
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil || v > math.MaxUint64-sample.total {
			return nil
		}
		sample.total += v
		if i == 4 || i == 5 {
			sample.idle += v
		}
	}
	return sample
}

func parseMemory(data []byte) *protocol.MachineResources {
	values := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || (fields[0] != "MemTotal:" && fields[0] != "MemAvailable:") {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return nil
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || v < 0 || v > math.MaxInt64/1024 {
			return nil
		}
		if _, exists := values[fields[0]]; exists {
			return nil
		}
		values[fields[0]] = v * 1024
	}
	total, hasTotal := values["MemTotal:"]
	available, hasAvailable := values["MemAvailable:"]
	if !hasTotal || !hasAvailable {
		return nil
	}
	out := &protocol.MachineResources{MemoryTotalBytes: total, MemoryAvailableBytes: available}
	if !out.Valid() {
		return nil
	}
	return out
}

func (s *resourceSampler) sample(root string, now time.Time) *protocol.MachineResources {
	current := parseCPU(readProc(filepath.Join(root, "stat")), now)
	previous := s.previous
	s.previous = current
	out := parseMemory(readProc(filepath.Join(root, "meminfo")))
	if out == nil {
		return nil
	}
	if previous == nil || current == nil || now.Sub(previous.at) < 100*time.Millisecond || now.Sub(previous.at) > 30*time.Second || current.total <= previous.total || current.idle < previous.idle {
		return out
	}
	total, idle := current.total-previous.total, current.idle-previous.idle
	if idle > total {
		return out
	}
	percent := float64(total-idle) / float64(total) * 100
	out.CPUPercent = &percent
	return out
}
