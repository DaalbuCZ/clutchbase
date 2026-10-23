package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"clutchbase/agent/internal/manager"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("clutchbase-host", flag.ContinueOnError)
	flags.SetOutput(out)
	root := flags.String("root", "", "Instance storage directory (default ~/.local/share/clutchbase)")
	steamcmd := flags.String("steamcmd", "steamcmd", "SteamCMD executable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		return errors.New("usage: clutchbase-host [--root PATH] [--steamcmd PATH] create|install|start|stop|update|list|logs|credentials [options] NAME")
	}
	m, err := manager.New(*root, *steamcmd)
	if err != nil {
		return err
	}
	if args[0] == "list" {
		if len(args) != 1 {
			return errors.New("list takes no instance name")
		}
		infos, err := m.List(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(infos)
	}
	if args[0] == "create" {
		create := flag.NewFlagSet("create", flag.ContinueOnError)
		create.SetOutput(out)
		port := create.Int("port", 27015, "Game port; the next two ports are reserved too")
		mapName := create.String("map", "de_mirage", "Official startup map")
		if err := create.Parse(args[1:]); err != nil {
			return err
		}
		if create.NArg() != 1 {
			return errors.New("usage: clutchbase-host create --port 27015 --map de_mirage NAME")
		}
		in, err := m.Create(manager.Instance{Name: create.Arg(0), Port: *port, Map: *mapName, Password: os.Getenv("CLUTCHBASE_SERVER_PASSWORD"), GSLT: os.Getenv("CLUTCHBASE_GSLT")})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Created %s on port %d. Join password: %s\nRun start %s to install CS2 and launch it.\n", in.Name, in.Port, in.Password, in.Name)
		return nil
	}
	if len(args) != 2 {
		return errors.New("this command requires exactly one instance name")
	}
	name := args[1]
	if _, err := m.Dir(name); err != nil {
		return err
	}
	switch args[0] {
	case "install":
		fmt.Fprintf(out, "Installing %s. Read output with clutchbase-host logs %s.\n", name, name)
		return m.Install(ctx, name, false)
	case "update":
		fmt.Fprintf(out, "Preparing update for %s. Read output with clutchbase-host logs %s.\n", name, name)
		return m.Install(ctx, name, true)
	case "start":
		fmt.Fprintf(out, "Preparing %s. Read installation output with clutchbase-host logs %s.\n", name, name)
		if err := m.Install(ctx, name, false); err != nil {
			return err
		}
		in, err := m.Load(name)
		if err != nil {
			return err
		}
		return m.Start(ctx, name, in.Map)
	case "stop":
		return m.Stop(ctx, name)
	case "logs":
		return m.Logs(name, out)
	case "credentials":
		in, err := m.Load(name)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Join password: %s\n", in.Password)
		return err
	default:
		return errors.New("unknown command: use create, install, start, stop, update, list, logs or credentials")
	}
}
