package manager

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
)

func TestRestartFenceRequiresStopAndAuthorization(t *testing.T) {
	id := rand.Text()
	f := restartFence{ID: id, Phase: "stopping"}
	for _, event := range []string{"running", "start_authorized"} {
		if _, err := f.advance(id, event); err == nil {
			t.Fatalf("accepted %s before stopped", event)
		}
	}
	if _, err := f.advance(rand.Text(), "stopped"); err == nil {
		t.Fatal("accepted another request's acknowledgement")
	}
	var err error
	for _, event := range []string{"stopped", "start_authorized", "running"} {
		f, err = f.advance(id, event)
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.Phase != "complete" {
		t.Fatalf("unexpected phase %s", f.Phase)
	}
	if _, err = f.advance(id, "stopped"); err == nil {
		t.Fatal("replayed completed request")
	}
}

func TestRestartFenceCancelPreventsStart(t *testing.T) {
	id := rand.Text()
	for _, phase := range []string{"stopping", "stopped", "starting"} {
		f, err := (restartFence{ID: id, Phase: phase}).advance(id, "cancel")
		if err != nil || f.Phase != "cancelled" {
			t.Fatalf("cancel %s: %+v %v", phase, f, err)
		}
		if _, err := f.advance(id, "start_authorized"); err == nil {
			t.Fatalf("cancelled %s restarted", phase)
		}
	}
}

func TestRestartFencePersistsAcrossAgentRestart(t *testing.T) {
	dir := t.TempDir()
	id := rand.Text()
	original := restartFence{ID: id, Phase: "stopped"}
	if err := saveRestartFence(dir, original); err != nil {
		t.Fatal(err)
	}
	restored, err := readRestartFence(dir)
	if err != nil || restored != original {
		t.Fatalf("restored %+v: %v", restored, err)
	}
	if _, err := restored.advance(rand.Text(), "start_authorized"); err == nil {
		t.Fatal("unrelated restart authorized")
	}
	if err := saveRestartFence(dir, restartFence{ID: id, Phase: "invented"}); err == nil {
		t.Fatal("invalid phase saved")
	}
	restored, err = readRestartFence(dir)
	if err != nil || restored != original {
		t.Fatalf("invalid update changed durable state: %+v %v", restored, err)
	}
}

func TestRestartFenceSerializesCompetingRequests(t *testing.T) {
	m := fixture(t)
	create(t, m, "friends", 27015)
	ctx := context.Background()
	ids := []string{rand.Text(), rand.Text()}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range ids {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errs[i] = m.recordRestartFence(ctx, "friends", ids[i], "begin") }(i)
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("expected exactly one restart: %v", errs)
	}
	winner := 0
	if errs[0] != nil {
		winner = 1
	}
	if _, err := m.recordRestartFence(ctx, "friends", ids[1-winner], "stopped"); err == nil {
		t.Fatal("other request advanced restart")
	}
	for _, event := range []string{"stopped", "start_authorized", "running"} {
		if _, err := m.recordRestartFence(ctx, "friends", ids[winner], event); err != nil {
			t.Fatal(err)
		}
	}
	duplicate, err := m.recordRestartFence(ctx, "friends", ids[winner], "begin")
	if err != nil || duplicate.Phase != "complete" {
		t.Fatalf("duplicate restarted service: %+v %v", duplicate, err)
	}
	if _, err := m.recordRestartFence(ctx, "friends", ids[1-winner], "begin"); err != nil {
		t.Fatal(err)
	}
}
