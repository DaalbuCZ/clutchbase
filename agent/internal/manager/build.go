package manager

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var buildID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

// Build IDs are read from Steam's own manifest and the public app-info branch.
// A failed lookup is unknown; callers must never infer that the installation is current.
func (m *Manager) InstalledBuild(name string) string {
	dir, err := m.Dir(name)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "server", "steamapps", "appmanifest_730.acf"))
	if err != nil || len(data) > 1<<20 {
		return ""
	}
	return vdfValue(data, "AppState", "buildid")
}

func (m *Manager) PublicBuild(ctx context.Context) (string, error) {
	job, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	unlock, err := waitLock(job, filepath.Join(m.Root, "steamcmd.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	var out limitedBuffer
	cmd := exec.CommandContext(job, m.SteamCMD, "+login", "anonymous", "+app_info_update", "1", "+app_info_print", "730", "+quit")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", errors.New("SteamCMD app info check failed")
	}
	id := vdfValue(out.Bytes(), "branches", "public", "buildid")
	if id == "" {
		return "", errors.New("SteamCMD public build unavailable")
	}
	return id, nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2<<20 {
		return 0, errors.New("SteamCMD output limit exceeded")
	}
	return b.Buffer.Write(p)
}

// vdfValue follows a named block path in Valve's quoted-key, brace-delimited format.
func vdfValue(data []byte, path ...string) string {
	tokens := regexp.MustCompile(`"(?:\\.|[^"\\])*"|[{}]`).FindAll(data, -1)
	stack := []string{}
	pending := ""
	for _, raw := range tokens {
		token := string(raw)
		switch token {
		case "{":
			stack = append(stack, pending)
			pending = ""
		case "}":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			pending = ""
		default:
			token = strings.Trim(token, `"`)
			if pending == "" {
				pending = token
				continue
			}
			if len(stack) >= len(path)-1 && equalPath(stack[len(stack)-len(path)+1:], path[:len(path)-1]) && pending == path[len(path)-1] && buildID.MatchString(token) {
				return token
			}
			pending = ""
		}
	}
	return ""
}
func equalPath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
