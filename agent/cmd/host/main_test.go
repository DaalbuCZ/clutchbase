package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
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
	if _, err := call("create", "--port", "27016", "overlap"); err == nil {
		t.Fatal("CLI accepted overlapping ports")
	}
	if _, err := call("start", "../outside"); err == nil {
		t.Fatal("CLI accepted a path as an instance name")
	}
}
