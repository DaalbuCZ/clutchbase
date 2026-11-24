package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"clutchbase/agent/internal/manager"
	"clutchbase/agent/internal/runner"
	"clutchbase/internal/protocol"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	simulate := flag.Bool("simulate", false, "Report simulated process state; never launch CS2")
	unit := flag.String("unit", "clutchbase-cs2.service", "Locally configured systemd user service")
	config := flag.String("config", "", "Absolute path to game/csgo/cfg/clutchbase.cfg")
	instance := flag.String("instance", "", "Managed instance name; installs missing CS2 files on first start")
	machine := flag.Bool("machine", false, "Manage all dashboard-created instances on this machine")
	root := flag.String("root", "", "Managed instance storage directory")
	steamcmd := flag.String("steamcmd", "steamcmd", "SteamCMD executable for managed instances")
	flag.Parse()
	base := strings.TrimRight(os.Getenv("CLUTCHBASE_API_URL"), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return errors.New("CLUTCHBASE_API_URL must be an origin URL without credentials or a path")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return errors.New("agent requires HTTPS except on loopback")
	}
	id := os.Getenv("CLUTCHBASE_HOST_ID")
	if *machine {
		id = os.Getenv("CLUTCHBASE_MACHINE_ID")
	}
	token := os.Getenv("CLUTCHBASE_AGENT_TOKEN")
	if id == "" || strings.ContainsAny(id, "/ ?#") || len(token) < 32 {
		return errors.New("the host/machine ID and CLUTCHBASE_AGENT_TOKEN are required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	if *machine {
		if *instance != "" || *config != "" {
			return errors.New("--machine cannot be combined with --instance or --config")
		}
		// A GSLT belongs to one instance; never distribute one process-wide token to a fleet.
		if os.Getenv("CLUTCHBASE_GSLT") != "" {
			return errors.New("machine mode requires per-instance GSLTs in private local instance configs; unset CLUTCHBASE_GSLT")
		}
		m, err := manager.New(*root, *steamcmd)
		if err != nil {
			return err
		}
		fleet, err := runner.NewFleet(m, *simulate)
		if err != nil {
			return err
		}
		defer fleet.Close()
		if *simulate {
			log.Print("SIMULATION enabled: no CS2 process will be started")
		}
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			var desired protocol.MachineDesired
			err := exchange(ctx, client, base+"/api/machine-agent/"+url.PathEscape(id)+"/heartbeat", token, fleet.Report(ctx), &desired, 524288)
			if err != nil {
				fleet.ConnectionFailed()
				log.Printf("Machine heartbeat failed: %v", err)
			} else if err = fleet.Apply(ctx, desired); err != nil {
				fleet.ConnectionFailed()
				log.Printf("Machine reconciliation failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	}
	var r interface {
		Status(context.Context) protocol.Report
		Apply(context.Context, protocol.Desired) error
	}
	if *instance != "" {
		if *simulate || *config != "" {
			return errors.New("--instance cannot be combined with --simulate or --config")
		}
		m, err := manager.New(*root, *steamcmd)
		if err != nil {
			return err
		}
		if _, err = m.Dir(*instance); err != nil {
			return err
		}
		unlock, err := m.AcquireAgent(*instance)
		if err != nil {
			return err
		}
		defer unlock()
		managed := &runner.Managed{Manager: m, Name: *instance}
		defer managed.Close()
		r = managed
	} else {
		local := &runner.Runner{Simulate: *simulate, Unit: *unit, ConfigPath: *config}
		if err := local.Validate(); err != nil {
			return err
		}
		r = local
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	if *simulate {
		log.Print("SIMULATION enabled: no CS2 process will be started")
	}
	var lastError string
	for {
		report := r.Status(ctx)
		if lastError != "" {
			report.Error = lastError
		}
		desired, err := heartbeat(ctx, client, base+"/api/agent/"+url.PathEscape(id)+"/heartbeat", token, report)
		if err != nil {
			log.Printf("Heartbeat failed: %v", err)
		} else {
			if err := r.Apply(ctx, desired); err != nil {
				lastError = err.Error()
				if len(lastError) > 1000 {
					lastError = lastError[:1000]
				}
				log.Printf("Reconciliation failed: %s", lastError)
			} else {
				lastError = ""
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func heartbeat(ctx context.Context, client *http.Client, endpoint, token string, report protocol.Report) (protocol.Desired, error) {
	var desired protocol.Desired
	err := exchange(ctx, client, endpoint, token, report, &desired, 16384)
	return desired, err
}
func exchange(ctx context.Context, client *http.Client, endpoint, token string, report any, desired any, limit int64) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return errors.New("API unreachable; check connection and URL")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, limit)).Decode(desired)
}
