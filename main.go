package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/csmith/containuum"
	"github.com/csmith/envflag/v2"
	"github.com/csmith/slogflags"
)

var (
	listen      = flag.String("listen", ":8080", "TCP address to listen on")
	routeExtras = flag.String("route-extras", "", "Lines to include in every route block")
	proxytag    = flag.String("proxytag", "", "Only process containers with matching com.chameth.proxytag label")
)

func main() {
	envflag.Parse()
	_ = slogflags.Logger(
		slogflags.WithAddSource(true),
		slogflags.WithSetDefault(true),
	)

	server := NewServer(*listen)
	if err := server.Start(); err != nil {
		slog.Error("Failed to start server", "err", err)
		os.Exit(1)
	}

	var filter = containuum.LabelExists(labelVhost)
	if *proxytag != "" {
		filter = containuum.All(
			containuum.LabelEquals("com.chameth.proxytag", *proxytag),
			filter,
		)
	}

	slog.Info("Starting container monitoring")
	err := containuum.Run(
		context.Background(),
		func(containers []containuum.Container) {
			slog.Info("Container change detected", "count", len(containers))

			config := GenerateConfig(containers, *routeExtras)

			slog.Debug("Generated config", "config", config)

			server.Broadcast(config)
		},
		containuum.WithFilter(filter),
		containuum.WithAutoReconnect(1*time.Second, 30*time.Second, 4),
	)

	if err != nil {
		slog.Error("Containuum failed", "err", err)
		os.Exit(1)
	}
}
