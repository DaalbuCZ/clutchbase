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
	"strings"
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
		return errors.New("usage: clutchbase-host [--root PATH] [--steamcmd PATH] create|install|start|stop|update|list|logs|credentials|whitelist|preset|reporter [options] NAME")
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
	if args[0] == "preset" {
		flags := flag.NewFlagSet("preset", flag.ContinueOnError)
		flags.SetOutput(out)
		mode := flags.String("mode", "", "match, practice or default")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 || (*mode != "match" && *mode != "practice" && *mode != "default") {
			return errors.New("usage: clutchbase-host preset --mode match|practice|default NAME")
		}
		value := *mode
		if value == "default" {
			value = ""
		}
		if err := m.SetPreset(ctx, flags.Arg(0), value); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "Preset configured for the next managed launch.")
		return err
	}
	if args[0] == "reporter" {
		flags := flag.NewFlagSet("reporter", flag.ContinueOnError)
		flags.SetOutput(out)
		install := flags.Bool("install", false, "Install the staged reporter on a stopped instance")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("usage: clutchbase-host reporter [--install] NAME")
		}
		info, err := m.ReporterStatus(flags.Arg(0))
		if err != nil {
			return err
		}
		if *install {
			if info.StagedHash == "" {
				return errors.New("staged Clutchbase Access reporter is unavailable")
			}
			if err := m.UpdateReporter(ctx, flags.Arg(0), info.StagedHash); err != nil {
				return err
			}
			info, err = m.ReporterStatus(flags.Arg(0))
			if err != nil {
				return err
			}
		}
		return json.NewEncoder(out).Encode(info)
	}
	if args[0] == "whitelist" {
		flags := flag.NewFlagSet("whitelist", flag.ContinueOnError)
		flags.SetOutput(out)
		file := flags.String("file", "", "File containing one SteamID64 per line")
		disable := flags.Bool("disable", false, "Return to password-only access")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 || (*file != "" && *disable) || (*file == "" && !*disable) {
			return errors.New("usage: clutchbase-host whitelist (--file FILE | --disable) NAME")
		}
		var ids []string
		if !*disable {
			f, err := os.Open(*file)
			if err != nil {
				return err
			}
			defer f.Close()
			data, err := io.ReadAll(io.LimitReader(f, 8193))
			if err != nil {
				return err
			}
			if len(data) > 8192 || len(strings.Fields(string(data))) == 0 {
				return errors.New("use a nonempty SteamID64 file of at most 8192 bytes; use --disable to return to password-only access")
			}
			ids = strings.Fields(string(data))
		}
		if err := m.SetWhitelist(ctx, flags.Arg(0), ids); err != nil {
			return err
		}
		if *disable {
			_, err = fmt.Fprintln(out, "Whitelist disabled. Join password remains required.")
		} else {
			_, err = fmt.Fprintln(out, "Whitelist configured. Join password remains required; verify allowed and denied real client joins before relying on enforcement. See docs/ACCESS.md.")
		}
		return err
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
		return errors.New("unknown command: use create, install, start, stop, update, list, logs, credentials, whitelist, preset or reporter")
	}
}
