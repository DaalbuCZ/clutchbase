package hostlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTailFileKeepsWholeTruncatedLinesAndRedacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.log")
	lines := []string{}
	for i := 0; i < 400; i++ {
		lines = append(lines, "Update state (0x61) downloading, press Ctrl+C to interrupt")
	}
	lines = append(lines, "sv_password \"hunter2\" echoed by the engine")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	tail := TailFile(path, 1024)
	if tail == "" {
		t.Fatal("tail of an existing file was empty")
	}
	for _, line := range strings.Split(tail, "\n") {
		if line != "[redacted]" && !strings.HasPrefix(line, "Update state (0x61) downloading") {
			t.Fatalf("tail contains a partial line: %q", line)
		}
	}
	if strings.Contains(tail, "hunter2") || !strings.Contains(tail, "[redacted]") {
		t.Fatalf("secrets were not redacted: %q", tail)
	}
	if int64(len(tail)) > 1024 {
		t.Fatalf("tail exceeded its cap: %d", len(tail))
	}
	if TailFile(filepath.Join(t.TempDir(), "missing.log"), 1024) != "" {
		t.Fatal("a missing file must yield an empty tail")
	}
	if Redact("sv_setsteamaccount ABC") != "[redacted]" {
		t.Fatal("GSLT lines must be redacted")
	}
	if Redact("plain line") != "plain line" {
		t.Fatal("ordinary lines must be preserved")
	}
}

func TestJournalUsesLocalUserUnitQueries(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\nprintf 'CS2 console line\\nsv_password \"hunter2\"\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, "journalctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	tail := Journal("clutchbase-test.service", 30, 4096)
	if !strings.Contains(tail, "CS2 console line") || strings.Contains(tail, "hunter2") {
		t.Fatalf("journal tail was not redacted: %q", tail)
	}
	// Without a journalctl on PATH the tail is simply empty.
	if err := os.Remove(filepath.Join(bin, "journalctl")); err != nil {
		t.Fatal(err)
	}
	if Journal("clutchbase-test.service", 30, 4096) != "" {
		t.Fatal("a missing journalctl must yield an empty tail")
	}
}

func TestCappedUnicodeLogsDoNotExpandInJSON(t *testing.T) {
	tail := redact(strings.Repeat("map load 中\n", 30), 100)
	if len(tail) > 100 || !utf8.ValidString(tail) {
		t.Fatal("invalid capped UTF-8 log")
	}
	data, err := json.Marshal(tail)
	if err != nil {
		t.Fatal(err)
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) > 100 {
		t.Fatal("JSON encoding expanded a split UTF-8 character beyond the report cap")
	}
	if got := redact("sv_passwo\xffrd secret", 100); got != "[redacted]" {
		t.Fatal("invalid UTF-8 bypassed redaction")
	}
}

func TestBoundedJournalTailDropsTruncatedSecretLine(t *testing.T) {
	tail := &boundedTail{limit: 32}
	_, _ = tail.Write([]byte(strings.Repeat("x", 100) + " sv_password secret\n"))
	_, _ = tail.Write([]byte("server ready\n"))
	got := redact(dropPartialFirstLine(string(tail.data), tail.truncated), 32)
	if strings.Contains(got, "secret") || !strings.Contains(got, "server ready") || len(tail.data) > 32 {
		t.Fatalf("unsafe or incomplete tail: %q", got)
	}
}
