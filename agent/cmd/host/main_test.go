package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clutchbase/agent/internal/manager"
)

func TestCLISetupAndPrivateCredentials(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("CLUTCHBASE_SERVER_PASSWORD", "friends-test-password")
	t.Setenv("CLUTCHBASE_GSLT", "")
	base := []string{"--root", filepath.Join(root, "instances")}
	call := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		err := run(context.Background(), append(append([]string{}, base...), args...), &out)
		return out.String(), err
	}
	if _, err := call("create", "--port", "27015", "friday"); err != nil {
		t.Fatal(err)
	}
	data, err := call("list")
	if err != nil {
		t.Fatal(err)
	}
	var list []manager.Info
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Port != 27015 || list[0].State != "uninstalled" {
		t.Fatalf("wrong CLI listing: %s", data)
	}
	if data, err := call("credentials", "friday"); err != nil || data != "Join password: friends-test-password\n" {
		t.Fatalf("credentials: %q, %v", data, err)
	}
	data, err = call("reporter", "friday")
	if err != nil || !strings.Contains(data, `"status":"unavailable"`) || strings.Contains(data, "friends-test-password") {
		t.Fatalf("reporter status: %q, %v", data, err)
	}
	if _, err := call("reporter", "--install", "friday"); err == nil {
		t.Fatal("installed an unavailable staged reporter")
	}
	if _, err := call("create", "--port", "27016", "overlap"); err == nil {
		t.Fatal("CLI accepted overlapping ports")
	}
	if _, err := call("start", "../outside"); err == nil {
		t.Fatal("CLI accepted a path as an instance name")
	}
}

func TestCLIWhitelistInput(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	file := filepath.Join(root, "players.txt")
	for _, input := range []string{"", "\n\t", strings.Repeat("1", 8193), "76561198154367261;quit", "76561198154367261\n76561198154367261"} {
		if err := os.WriteFile(file, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := run(context.Background(), []string{"--root", root, "whitelist", "--file", file, "friends"}, &out); err == nil {
			t.Fatalf("accepted invalid whitelist input %q", input)
		}
		if out.Len() != 0 {
			t.Fatal("reported success for rejected whitelist")
		}
	}
	for _, args := range [][]string{
		{"whitelist", "friends"},
		{"whitelist", "--file", file, "--disable", "friends"},
		{"whitelist", "--disable"},
	} {
		var out bytes.Buffer
		err := run(context.Background(), append([]string{"--root", root}, args...), &out)
		if err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("expected usage error: %v", err)
		}
	}
}
