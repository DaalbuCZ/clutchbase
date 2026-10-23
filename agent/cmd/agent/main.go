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
	token := os.Getenv("CLUTCHBASE_AGENT_TOKEN")
	if id == "" || strings.ContainsAny(id, "/ ?#") || len(token) < 32 {
		return errors.New("CLUTCHBASE_HOST_ID and CLUTCHBASE_AGENT_TOKEN are required")
	}
	r := &runner.Runner{Simulate: *simulate, Unit: *unit, ConfigPath: *config}
	if err := r.Validate(); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
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
	data, err := json.Marshal(report)
	if err != nil {
		return desired, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return desired, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return desired, errors.New("API unreachable; check connection and URL")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return desired, fmt.Errorf("API returned HTTP %d", res.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&desired)
	return desired, err
}
