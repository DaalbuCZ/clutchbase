package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"clutchbase/agent/internal/access"
)

func main() {
	dir := flag.String("access-dir", "", "Private runtime evidence directory")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *dir == "" || len(flag.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "access directory and game executable required")
		os.Exit(1)
	}
	if err := access.Supervise(ctx, *dir, flag.Args(), os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
