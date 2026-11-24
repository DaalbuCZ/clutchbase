package access

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Supervise keeps the watchdog alive independently of the outbound agent.
// Password protection remains necessary during startup and the detection window.
func Supervise(ctx context.Context, dir string, args []string, stdout, stderr io.Writer) error {
	return supervise(ctx, dir, args, stdout, stderr, 90*time.Second, time.Second)
}

func supervise(ctx context.Context, dir string, args []string, stdout, stderr io.Writer, startup, interval time.Duration) error {
	if len(args) == 0 {
		return errors.New("game executable required")
	}
	var request Request
	if err := Read(filepath.Join(dir, "request.json"), &request); err != nil || len(request.RosterHash) != 64 {
		return errors.New("invalid local whitelist request")
	}
	request.Nonce = rand.Text()
	request.ProcessGroup = 0
	if err := Write(filepath.Join(dir, "request.json"), request); err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return errors.New("cannot start supervised game")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stop := func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
	}
	request.ProcessGroup = cmd.Process.Pid
	if err := Write(filepath.Join(dir, "request.json"), request); err != nil {
		stop()
		return err
	}
	started := time.Now()
	verified := false
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return nil
		case err := <-done:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			return err
		case now := <-ticker.C:
			if err := Verify(dir, request, now); err == nil {
				verified = true
			} else if verified || now.Sub(started) >= startup {
				// Persist a latch so an agent cannot repeatedly restart an unverified game.
				_ = Write(filepath.Join(dir, "failure.json"), "Runtime whitelist evidence lost; stop the instance before retrying")
				stop()
				return errors.New("runtime whitelist evidence lost; supervised game stopped")
			}
		}
	}
}

// Failure returns only a fixed operator message, never plugin-supplied text.
func Failure(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "failure.json"))
	return !errors.Is(err, os.ErrNotExist)
}
