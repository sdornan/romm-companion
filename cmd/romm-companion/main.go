// Command romm-companion pairs a desktop with a RomM server, adds RomM games to
// Steam, and launches them with local emulators.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// version is set by the release build via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "romm-companion:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "pair":
		return cmdPair(ctx, args[1:])
	case "steam":
		return cmdSteam(ctx, args[1:])
	case "launch":
		return cmdLaunch(ctx, args[1:])
	case "capabilities":
		return cmdCapabilities(ctx, args[1:])
	case "run":
		return cmdRun(ctx, args[1:])
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: romm-companion <command> [flags]

Commands:
  pair <server-url> <code>   Pair with a RomM server using a code from its /pair page
  steam paths                Show the Steam installation and user data directories found
  steam list                 List the shortcuts RomM Companion owns in shortcuts.vdf
  capabilities               Show which emulator would open each platform on this PC
  launch --rom <id>          Download if needed, launch, and report the play session
  run [--once]               Stay running and apply shortcut changes from RomM
  version                    Print the version

Config file: $ROMM_COMPANION_CONFIG or the OS config dir under romm-companion/.
`)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}
