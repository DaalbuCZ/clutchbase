package access

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestEvidenceBoundToLaunchProcessRosterAndTime(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	request := Request{Nonce: "launch", RosterHash: Hash([]byte("roster")), ProcessGroup: syscall.Getpgrp()}
	good := Evidence{Request: request, ProcessID: os.Getpid(), ReportedAt: now.UnixMilli(), Status: "verified"}
	for _, mutation := range []func(*Evidence){
		func(e *Evidence) { e.Nonce = "old launch" },
		func(e *Evidence) { e.RosterHash = Hash(nil) },
		func(e *Evidence) { e.ProcessID = -1 },
		func(e *Evidence) { e.ProcessID = 999999999 },
		func(e *Evidence) { e.ProcessGroup++ },
		func(e *Evidence) { e.ReportedAt = now.Add(-Freshness - time.Second).UnixMilli() },
		func(e *Evidence) { e.ReportedAt = now.Add(2 * time.Second).UnixMilli() },
		func(e *Evidence) { e.Status = "blocked" },
	} {
		evidence := good
		mutation(&evidence)
		if err := Write(filepath.Join(dir, "evidence.json"), evidence); err != nil {
			t.Fatal(err)
		}
		if Verify(dir, request, now) == nil {
			t.Fatal("accepted invalid evidence")
		}
	}
	if err := Write(filepath.Join(dir, "evidence.json"), good); err != nil {
		t.Fatal(err)
	}
	if err := Verify(dir, request, now); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(dir, "evidence.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("evidence is not private")
	}
}

// TestGuardChild is a fake game process, isolated in its own process group.
func TestGuardChild(t *testing.T) {
	dir := os.Getenv("CLUTCHBASE_GUARD_TEST")
	if dir == "" {
		return
	}
	for {
		var request Request
		if Read(filepath.Join(dir, "request.json"), &request) == nil && request.ProcessGroup > 0 {
			status := "verified"
			if _, err := os.Stat(filepath.Join(dir, "block")); err == nil {
				status = "blocked"
			}
			_ = Write(filepath.Join(dir, "evidence.json"), Evidence{Request: request, ProcessID: os.Getpid(), ReportedAt: time.Now().UnixMilli(), Status: status})
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSupervisorStopsUnverifiedGame(t *testing.T) {
	dir := t.TempDir()
	if err := Write(filepath.Join(dir, "request.json"), Request{RosterHash: Hash([]byte("roster"))}); err != nil {
		t.Fatal(err)
	}
	err := supervise(context.Background(), dir, []string{"/bin/sh", "-c", "sleep 30"}, io.Discard, io.Discard, 100*time.Millisecond, 10*time.Millisecond)
	if err == nil || !Failure(dir) {
		t.Fatal("unverified game did not fail and latch")
	}
	var request Request
	if err := Read(filepath.Join(dir, "request.json"), &request); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-request.ProcessGroup, 0); err == nil {
		t.Fatal("unverified process group survived")
	}
}

func TestSupervisorLossAndCancellation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "evidence_lost"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLUTCHBASE_GUARD_TEST", dir)
			if err := Write(filepath.Join(dir, "request.json"), Request{RosterHash: Hash([]byte("roster"))}); err != nil {
				t.Fatal(err)
			}
			exe, _ := os.Executable()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- supervise(ctx, dir, []string{exe, "-test.run=^TestGuardChild$"}, io.Discard, io.Discard, 5*time.Second, 10*time.Millisecond)
			}()
			deadline := time.Now().Add(4 * time.Second)
			for {
				var r Request
				if Read(filepath.Join(dir, "request.json"), &r) == nil && Verify(dir, r, time.Now()) == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fake game did not report")
				}
				time.Sleep(10 * time.Millisecond)
			}
			// Let the watchdog consume one valid report before injecting failure.
			time.Sleep(40 * time.Millisecond)
			if fail {
				if err := os.WriteFile(filepath.Join(dir, "block"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if (err != nil) != fail || Failure(dir) != fail {
					t.Fatalf("wrong shutdown result: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("watchdog did not stop game")
			}
		})
	}
}
