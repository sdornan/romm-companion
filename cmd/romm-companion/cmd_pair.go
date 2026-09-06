package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/romm"
)

func cmdPair(ctx context.Context, args []string) error {
	fs := newFlagSet("pair")
	name := fs.String("name", "", "device name shown in RomM (default: hostname)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: pair <server-url> <code>")
	}
	server, code := fs.Arg(0), fs.Arg(1)

	tok, err := romm.ExchangePairCode(ctx, server, code)
	if err != nil {
		return fmt.Errorf("pairing failed: %w", err)
	}
	host, _ := os.Hostname()
	if *name == "" {
		*name = host
	}
	client := romm.New(server, tok.RawToken, version)
	dev, err := client.RegisterDevice(ctx, romm.DeviceCreate{
		Name:          *name,
		Platform:      runtime.GOOS,
		Client:        romm.ClientName,
		ClientVersion: version,
		Hostname:      host,
		AllowExisting: true,
	})
	if err != nil {
		return fmt.Errorf("device registration failed: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.ServerURL, cfg.Token, cfg.DeviceID = server, tok.RawToken, dev.DeviceID
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	fmt.Printf("Paired with %s as device %q (%s).\nSettings saved to %s\n", server, dev.Name, dev.DeviceID, p)
	return nil
}
