// Package hostlog collects short local log tails for agent reports. Join
// passwords and GSLT tokens must stay in host files, so every line that could
// echo one is redacted before the tail leaves the machine.
package hostlog

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TailFile returns at most max bytes from the end of path, redacted. A missing
// or unreadable file yields an empty string; tails are advisory, never fatal.
func TailFile(path string, max int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		return ""
	}
	offset := int64(0)
	if stat.Size() > max {
		offset = stat.Size() - max
	}
	data, err := io.ReadAll(io.NewSectionReader(file, offset, stat.Size()-offset))
	if err != nil {
		return ""
	}
	return redact(dropPartialFirstLine(string(data), offset > 0), max)
}

// Journal returns recent lines of a user service journal, redacted and capped.
func Journal(unit string, lines int, max int64) string {
	if max <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tail := &boundedTail{limit: int(max)}
	cmd := exec.CommandContext(ctx, "journalctl", "--user", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "-o", "cat")
	cmd.Stdout = tail
	if err := cmd.Run(); err != nil {
		return ""
	}
	return redact(dropPartialFirstLine(string(tail.data), tail.truncated), max)
}

// boundedTail retains the end of the stream without buffering all journal output.
type boundedTail struct {
	data      []byte
	limit     int
	truncated bool
}

func (t *boundedTail) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= t.limit {
		t.truncated = t.truncated || len(t.data) > 0 || n > t.limit
		t.data = append(t.data[:0], p[len(p)-t.limit:]...)
		return n, nil
	}
	if len(t.data)+n > t.limit {
		drop := len(t.data) + n - t.limit
		copy(t.data, t.data[drop:])
		t.data = t.data[:len(t.data)-drop]
		t.truncated = true
	}
	t.data = append(t.data, p...)
	return n, nil
}

// Redact replaces whole lines that may echo join passwords or GSLT tokens.
func Redact(text string) string {
	return redact(text, 1<<20)
}

func redact(text string, max int64) string {
	text = strings.ToValidUTF8(text, "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "password") || strings.Contains(lower, "setsteamaccount") {
			line = "[redacted]"
		}
		lines[i] = line
	}
	text = strings.Trim(strings.Join(lines, "\n"), "\n")
	if int64(len(text)) > max {
		text = dropPartialFirstLine(text, true)
	}
	if int64(len(text)) > max {
		text = text[int64(len(text))-max:]
	}
	return strings.TrimSpace(strings.ToValidUTF8(text, ""))
}

// dropPartialFirstLine removes a possibly truncated line when the tail starts
// mid-file, so displayed output only contains whole lines.
func dropPartialFirstLine(text string, truncated bool) string {
	if !truncated {
		return text
	}
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[index+1:]
	}
	return ""
}
