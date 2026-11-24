package control

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This exercises the real agent and HTTP control loop with fake local services, not CS2.
func TestRestartWithRealAgentProcess(t *testing.T) {
	if os.Getenv("CLUTCHBASE_RESTART_PROCESS_TEST") != "1" {
		t.Skip("set CLUTCHBASE_RESTART_PROCESS_TEST=1 to run the agent process test")
	}
	a := integrationAPI(t)
	ctx := context.Background()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{
		"systemctl": `#!/bin/sh
case " $* " in
 *" show "*) echo loaded ;;
 *" is-active "*) if [ -f "$CB_JOB_DIR/running" ]; then echo active; else echo inactive; fi ;;
 *" start "*) echo start >> "$CB_JOB_DIR/actions"; touch "$CB_JOB_DIR/running" ;;
 *" stop "*) echo stop >> "$CB_JOB_DIR/actions"; rm -f "$CB_JOB_DIR/running" ;;
esac
`,
		"steamcmd": `#!/bin/sh
if [ "$1" = "+force_install_dir" ]; then
 echo install >> "$CB_JOB_DIR/installs"
 mkdir -p "$2/game/csgo/cfg" "$2/game/bin/linuxsteamrt64"
 printf '#!/bin/sh\nexit 0\n' > "$2/game/cs2.sh"
 chmod +x "$2/game/cs2.sh"
 cp "$2/game/cs2.sh" "$2/game/bin/linuxsteamrt64/cs2"
fi
`,
		"journalctl": "#!/bin/sh\nexit 0\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(dir, "agent")
	build := exec.Command("go", "build", "-o", binary, "./agent/cmd/agent")
	build.Dir = "../../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v: %s", err, output)
	}
	udp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	port := udp.LocalAddr().(*net.UDPAddr).Port
	go func() {
		buf := make([]byte, 1500)
		for {
			_, from, err := udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if _, err = os.Stat(filepath.Join(dir, "running")); err == nil {
				reply := append([]byte{0xff, 0xff, 0xff, 0xff, 0x49, 24}, []byte("clutchbase\x00de_mirage\x00csgo\x00CS2\x00")...)
				reply = append(reply, 0, 0, 0, 10, 0, 'd', 'l', 0, 0)
				_, _ = udp.WriteToUDP(reply, from)
			}
		}
	}()
	machine, host, token := rand.Text(), rand.Text(), strings.Repeat("b", 32)
	if _, err = a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver) VALUES($1,'Machine','127.0.0.1',$2,'managed')`, machine, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err = a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver) VALUES($1,'Server','127.0.0.1',$2,'unused',$3,'running','stopped','managed')`, host, port, machine); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	agent := exec.Command(binary, "--machine", "--root", filepath.Join(dir, "data"), "--steamcmd", filepath.Join(binDir, "steamcmd"))
	agent.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"), "XDG_CONFIG_HOME="+filepath.Join(dir, "config"), "CB_JOB_DIR="+dir, "CLUTCHBASE_API_URL="+server.URL, "CLUTCHBASE_MACHINE_ID="+machine, "CLUTCHBASE_AGENT_TOKEN="+token, "CLUTCHBASE_GSLT=")
	logPath := filepath.Join(dir, "agent.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	agent.Stdout = logFile
	agent.Stderr = logFile
	if err = agent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = agent.Process.Kill(); _ = agent.Wait() }()
	wait := func(label string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(55 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		logs, _ := os.ReadFile(logPath)
		t.Fatalf("%s did not converge; agent log: %s", label, logs)
	}
	var revision int64
	wait("initial start", func() bool {
		return a.DB.QueryRow(ctx, `SELECT h.revision FROM hosts h JOIN machines m ON m.id=h.machine_id WHERE h.id=$1 AND h.observed_state='running' AND h.desired_state='running' AND m.supports_restart`, host).Scan(&revision) == nil
	})
	req, err := http.NewRequest("POST", server.URL+"/api/hosts/"+host+"/restart", strings.NewReader(fmt.Sprintf(`{"revision":%d}`, revision)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("restart returned %d", res.StatusCode)
	}
	wait("restart", func() bool {
		var phase, desired, observed string
		err := a.DB.QueryRow(ctx, `SELECT r.phase,h.desired_state,h.observed_state FROM server_restarts r JOIN hosts h ON h.id=r.host_id WHERE h.id=$1`, host).Scan(&phase, &desired, &observed)
		return err == nil && phase == "complete" && desired == "running" && observed == "running"
	})
	actions, err := os.ReadFile(filepath.Join(dir, "actions"))
	if err != nil {
		t.Fatal(err)
	}
	if string(actions) != "start\nstop\nstart\n" {
		t.Fatalf("unexpected service actions: %q", actions)
	}
	if err := a.DB.QueryRow(ctx, `SELECT revision FROM hosts WHERE id=$1`, host).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	req, err = http.NewRequest("POST", server.URL+"/api/hosts/"+host+"/game-update", strings.NewReader(fmt.Sprintf(`{"revision":%d,"stopFirst":true}`, revision)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("stop and update returned %d", res.StatusCode)
	}
	wait("stop and update", func() bool {
		var status, desired, observed string
		err := a.DB.QueryRow(ctx, `SELECT u.status,h.desired_state,h.observed_state FROM game_updates u JOIN hosts h ON h.id=u.host_id WHERE h.id=$1`, host).Scan(&status, &desired, &observed)
		return err == nil && status == "complete" && desired == "stopped" && observed == "stopped"
	})
	actions, err = os.ReadFile(filepath.Join(dir, "actions"))
	if err != nil || string(actions) != "start\nstop\nstart\nstop\n" {
		t.Fatalf("unexpected stop-and-update service actions: %q %v", actions, err)
	}
	installs, err := os.ReadFile(filepath.Join(dir, "installs"))
	if err != nil || string(installs) != "install\ninstall\n" {
		t.Fatalf("unexpected install operations: %q %v", installs, err)
	}
}
