package main

import (
	"encoding/binary"
	"flag"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/csmith/containuum/v2"
	"github.com/csmith/envflag/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestServer starts a Server on an ephemeral port and returns it along with its address. The
// server is not explicitly stopped: its listener goroutine is cleaned up when the test process exits.
func startTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	server := NewServer("127.0.0.1:0")
	require.NoError(t, server.Start())

	addr := server.Addr()
	require.NotNil(t, addr)
	return server, addr.String()
}

// dial connects to the server and registers cleanup to close the connection.
func dial(t *testing.T, addr string) net.Conn {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readFrame reads a single Centauri wire-protocol frame from conn and returns its payload, failing
// the test if the framing is invalid or nothing arrives within a short deadline.
func readFrame(t *testing.T, conn net.Conn) string {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))

	header := make([]byte, len(magic)+8)
	_, err := io.ReadFull(conn, header)
	require.NoError(t, err)

	assert.Equal(t, magic, string(header[:len(magic)]), "unexpected magic bytes")
	assert.Equal(t, version, binary.BigEndian.Uint32(header[len(magic):len(magic)+4]), "unexpected version")

	length := binary.BigEndian.Uint32(header[len(magic)+4:])
	payload := make([]byte, length)
	_, err = io.ReadFull(conn, payload)
	require.NoError(t, err)

	return string(payload)
}

// TestEndToEnd exercises the whole pipeline without Docker: containers are fed through the same
// filter and callback that main wires up, and a real TCP client reads the generated config off the
// wire and asserts it matches what GenerateConfig produces.
func TestEndToEnd(t *testing.T) {
	server, addr := startTestServer(t)

	conn := dial(t, addr)
	// On connect the client receives the current (empty) config.
	assert.Empty(t, readFrame(t, conn))

	containers := []containuum.Container{
		{Name: "web", Labels: map[string]string{labelVhost: "example.com www.example.com", labelProxy: "80"}},
		{Name: "api", Labels: map[string]string{labelVhost: "api.example.com", labelProxy: "8080"}},
		// Filtered out: no vhost label.
		{Name: "db", Labels: map[string]string{labelProxy: "5432"}},
	}

	handler := configHandler(server, "")
	filter := containerFilter("*")

	handler(filterContainers(filter, containers))

	want := GenerateConfig(filterContainers(filter, containers), "")
	assert.Equal(t, want, readFrame(t, conn))
	assert.Contains(t, want, "route api.example.com")
	assert.Contains(t, want, "route example.com www.example.com")
	assert.NotContains(t, want, "db")
}

// TestProxytagFiltering verifies that when a proxytag is configured only containers carrying a
// matching tag make it through to the generated config.
func TestProxytagFiltering(t *testing.T) {
	server, addr := startTestServer(t)

	conn := dial(t, addr)
	assert.Empty(t, readFrame(t, conn))

	filter := containerFilter("public")
	containers := []containuum.Container{
		{Name: "web", Labels: map[string]string{labelVhost: "example.com", labelProxy: "80", labelProxytag: "public"}},
		{Name: "internal", Labels: map[string]string{labelVhost: "internal.example.com", labelProxy: "80", labelProxytag: "private"}},
		{Name: "untagged", Labels: map[string]string{labelVhost: "untagged.example.com", labelProxy: "80"}},
	}

	configHandler(server, "")(filterContainers(filter, containers))

	config := readFrame(t, conn)
	assert.Contains(t, config, "route example.com")
	assert.NotContains(t, config, "internal.example.com")
	assert.NotContains(t, config, "untagged.example.com")
}

// TestContainerFilter verifies the proxytag selection semantics: "*" ignores proxytag labels,
// an empty value selects containers with a missing or empty label, and any other value requires
// an exact match. Containers without a vhost label are never selected.
func TestContainerFilter(t *testing.T) {
	containers := []containuum.Container{
		{Name: "tagged-public", Labels: map[string]string{labelVhost: "a.example.com", labelProxytag: "public"}},
		{Name: "tagged-private", Labels: map[string]string{labelVhost: "b.example.com", labelProxytag: "private"}},
		{Name: "tagged-empty", Labels: map[string]string{labelVhost: "c.example.com", labelProxytag: ""}},
		{Name: "untagged", Labels: map[string]string{labelVhost: "d.example.com"}},
		{Name: "no-vhost", Labels: map[string]string{labelProxytag: "public"}},
	}

	tests := []struct {
		name     string
		proxytag string
		want     []string
	}{
		{
			name:     "wildcard ignores proxytag labels",
			proxytag: "*",
			want:     []string{"tagged-public", "tagged-private", "tagged-empty", "untagged"},
		},
		{
			name:     "empty value selects containers with missing or empty labels",
			proxytag: "",
			want:     []string{"tagged-empty", "untagged"},
		},
		{
			name:     "other values require an exact match",
			proxytag: "public",
			want:     []string{"tagged-public"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range filterContainers(containerFilter(tt.proxytag), containers) {
				got = append(got, c.Name)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// unsetenv unsets an environment variable for the duration of the test, restoring any previous
// value afterwards. Unlike t.Setenv it removes the variable entirely rather than setting it empty.
func unsetenv(t *testing.T, name string) {
	t.Helper()

	original, wasSet := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name))
	t.Cleanup(func() {
		if wasSet {
			require.NoError(t, os.Setenv(name, original))
		} else {
			require.NoError(t, os.Unsetenv(name))
		}
	})
}

// TestProxytagParsing verifies the parsing behaviour the proxytag semantics rely on: the default
// value is used when neither the flag nor the environment variable is given, an explicitly empty
// environment variable or flag argument yields an empty value (as opposed to the default), and an
// explicit flag argument takes precedence over the environment variable.
func TestProxytagParsing(t *testing.T) {
	tests := []struct {
		name   string
		env    string
		envSet bool
		args   []string
		want   string
	}{
		{
			name: "unset flag and environment variable default to wildcard",
			want: "*",
		},
		{
			name:   "empty environment variable yields empty value",
			envSet: true,
			want:   "",
		},
		{
			name: "empty flag argument yields empty value",
			args: []string{"--proxytag="},
			want: "",
		},
		{
			name:   "environment variable is used when flag is absent",
			envSet: true,
			env:    "public",
			want:   "public",
		},
		{
			name: "flag argument is used when environment variable is unset",
			args: []string{"--proxytag=public"},
			want: "public",
		},
		{
			name:   "flag argument takes precedence over environment variable",
			envSet: true,
			env:    "public",
			args:   []string{"--proxytag=private"},
			want:   "private",
		},
		{
			name:   "empty flag argument takes precedence over environment variable",
			envSet: true,
			env:    "public",
			args:   []string{"--proxytag="},
			want:   "",
		},
		{
			name:   "wildcard may be set explicitly",
			envSet: true,
			env:    "*",
			want:   "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetenv(t, "PROXYTAG")
			if tt.envSet {
				t.Setenv("PROXYTAG", tt.env)
			}

			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			proxytag := fs.String("proxytag", flag.Lookup("proxytag").DefValue, "")
			envflag.Parse(
				envflag.WithFlagSet(fs),
				envflag.WithArguments(tt.args),
				envflag.WithShowInUsage(false),
			)

			assert.Equal(t, tt.want, *proxytag)
		})
	}
}

// TestLateClientReceivesLastConfig verifies that a client connecting after a broadcast immediately
// receives the most recently broadcast config.
func TestLateClientReceivesLastConfig(t *testing.T) {
	server, addr := startTestServer(t)

	containers := []containuum.Container{
		{Name: "web", Labels: map[string]string{labelVhost: "example.com", labelProxy: "80"}},
	}
	configHandler(server, "")(containers)

	conn := dial(t, addr)
	assert.Equal(t, GenerateConfig(containers, ""), readFrame(t, conn))
}

// TestBroadcastReachesAllClients verifies that a broadcast is delivered to every connected client.
func TestBroadcastReachesAllClients(t *testing.T) {
	server, addr := startTestServer(t)

	conns := []net.Conn{dial(t, addr), dial(t, addr), dial(t, addr)}
	for _, conn := range conns {
		assert.Empty(t, readFrame(t, conn))
	}

	containers := []containuum.Container{
		{Name: "web", Labels: map[string]string{labelVhost: "example.com", labelProxy: "80"}},
	}
	server.Broadcast(GenerateConfig(containers, ""))

	want := GenerateConfig(containers, "")
	for _, conn := range conns {
		assert.Equal(t, want, readFrame(t, conn))
	}
}

// filterContainers applies a containuum filter to a slice of containers, mirroring the selection
// containuum performs before invoking the callback.
func filterContainers(filter containuum.Filter, containers []containuum.Container) []containuum.Container {
	var out []containuum.Container
	for _, c := range containers {
		if filter(c) {
			out = append(out, c)
		}
	}
	return out
}
