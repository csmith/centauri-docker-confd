package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/csmith/containuum/v2"
	"github.com/csmith/envflag/v2"
	"github.com/csmith/slogflags"
)

var (
	listen      = flag.String("listen", ":8080", "TCP address to listen on")
	routeExtras = flag.String("route-extras", "", "Lines to include in every route block")
	proxytag    = flag.String("proxytag", "*", "Only process containers whose com.chameth.proxytag label matches: * ignores the label, an empty value selects containers with a missing or empty label, anything else must match exactly")
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

	if *proxytag != "*" {
		slog.Info("Filtering containers by label", "label", labelProxytag, "value", *proxytag)
	}

	slog.Info("Starting container monitoring")
	err := containuum.Run(
		context.Background(),
		configHandler(server, *routeExtras),
		containuum.WithFilter(containerFilter(*proxytag)),
		containuum.WithAutoReconnect(1*time.Second, 30*time.Second, 4),
	)

	if err != nil {
		slog.Error("Containuum failed", "err", err)
		os.Exit(1)
	}
}

// containerFilter builds the filter that selects which containers to proxy. Containers must declare a
// vhost label. A proxytag of "*" ignores the com.chameth.proxytag label entirely, an empty proxytag
// selects containers with a missing or empty label, and any other value must match the label exactly.
func containerFilter(proxytag string) containuum.Filter {
	filter := containuum.LabelExists(labelVhost)
	if proxytag != "*" {
		filter = containuum.All(
			containuum.LabelEquals(labelProxytag, proxytag),
			filter,
		)
	}
	return filter
}

// configHandler builds the callback invoked whenever the set of matching containers changes. It
// regenerates the Centauri config and broadcasts it to all connected clients.
func configHandler(server *Server, routeExtras string) containuum.Callback {
	return func(containers []containuum.Container) {
		slog.Info("Container change detected", "count", len(containers))

		config := GenerateConfig(containers, routeExtras)

		if len(config) == 0 {
			slog.Warn("No suitable containers to proxy")
		} else {
			slog.Debug("Generated config", "config", config)
		}

		server.Broadcast(config)
	}
}
