package main

import (
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/csmith/containuum"
)

const (
	labelErrors   = "com.chameth.errors"
	labelHeaders  = "com.chameth.headers"
	labelProvider = "com.chameth.provider"
	labelProxy    = "com.chameth.proxy"
	labelProxytag = "com.chameth.proxytag"
	labelSubject  = "com.chameth.subject"
	labelVhost    = "com.chameth.vhost"
)

// RouteInfo represents the routing configuration for a hostname
type RouteInfo struct {
	Primary      string
	Alternatives []string
	Upstreams    []Upstream
	Headers      map[string]string
	Errors       map[int]string
	Provider     string
	Subject      string
}

// Upstream represents a backend server
type Upstream struct {
	Name string
	Port int
}

// parseVhosts parses the com.chameth.vhost label into primary and alternative hostnames
func parseVhosts(vhost string) (primary string, alternatives []string) {
	parts := strings.FieldsFunc(vhost, func(r rune) bool {
		return r == ',' || r == ' '
	})

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i == 0 {
			primary = part
		} else {
			alternatives = append(alternatives, part)
		}
	}

	return primary, alternatives
}

// parsePort extracts the port from the com.chameth.proxy label or auto-detects from exposed ports
func parsePort(container containuum.Container) int {
	if portStr, ok := container.Labels[labelProxy]; ok {
		if port, err := strconv.Atoi(portStr); err == nil {
			slog.Debug("Using proxy port specified by label", "container", container.Name, "port", portStr)
			return port
		} else {
			slog.Warn("Failed to parse port label", "container", container.Name, "port", portStr, "error", err)
		}
	}

	// Auto-detect if there's exactly one port and it's not bound to host
	var unboundPorts []containuum.Port
	for _, port := range container.Ports {
		if port.HostPort == 0 {
			unboundPorts = append(unboundPorts, port)
		}
	}

	if len(unboundPorts) == 1 {
		slog.Debug("Using single declared port", "container", container.Name, "port", unboundPorts[0].ContainerPort)
		return int(unboundPorts[0].ContainerPort)
	}

	slog.Debug("Container has no valid port label, and does not declare a single unbound port", "container", container.Name)
	return -1
}

// parseHeaders collects all com.chameth.headers.* labels
func parseHeaders(container containuum.Container) map[string]string {
	headers := make(map[string]string)
	prefix := labelHeaders + "."

	for key, value := range container.Labels {
		if strings.HasPrefix(key, prefix) {
			parts := strings.SplitN(value, ":", 2)
			if len(parts) == 2 {
				headerName := strings.TrimSpace(parts[0])
				headerValue := strings.TrimSpace(parts[1])
				headers[headerName] = headerValue
			} else {
				slog.Warn("Invalid header label, missing ':'", "container", container.Name, "label", key, "value", value)
			}
		}
	}

	return headers
}

// parseErrors collects all com.chameth.errors.<status> labels, mapping an HTTP status code to the
// upstream that should generate the response for it (format: host:port or host:port/path).
func parseErrors(container containuum.Container) map[int]string {
	errors := make(map[int]string)
	prefix := labelErrors + "."

	for key, value := range container.Labels {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		statusStr := strings.TrimPrefix(key, prefix)
		status, err := strconv.Atoi(statusStr)
		if err != nil || status < 400 || status > 599 {
			slog.Warn("Invalid error status code in label", "container", container.Name, "label", key)
			continue
		}

		target := strings.TrimSpace(value)
		if target == "" {
			slog.Warn("Empty error upstream in label", "container", container.Name, "label", key)
			continue
		}

		errors[status] = target
	}

	return errors
}

// groupByHostname groups containers by their primary hostname
func groupByHostname(containers []containuum.Container) map[string]*RouteInfo {
	routes := make(map[string]*RouteInfo)

	for _, container := range containers {
		vhost, ok := container.Labels[labelVhost]
		if !ok || vhost == "" {
			slog.Debug("Not proxying container: missing or empty vhost label", "container", container.Name, "present", ok)
			continue
		}

		port := parsePort(container)
		if port == -1 {
			slog.Debug("Not proxying container: no valid port", "container", container.Name)
			continue
		}

		primary, alternatives := parseVhosts(vhost)
		if primary == "" {
			slog.Debug("Not proxying container: no valid vhost", "container", container.Name, "vhost", vhost)
			continue
		}

		route, exists := routes[primary]
		if !exists {
			route = &RouteInfo{
				Primary:      primary,
				Alternatives: alternatives,
				Headers:      make(map[string]string),
				Errors:       make(map[int]string),
				Provider:     container.Labels[labelProvider],
				Subject:      container.Labels[labelSubject],
			}
			routes[primary] = route
		} else {
			if !slices.Equal(route.Alternatives, alternatives) {
				slog.Warn(
					"Multiple containers declare the same route with different alternate names",
					"container1_name", route.Upstreams[0].Name,
					"container2_name", container.Name,
					"route", primary,
					"container1_alts", route.Alternatives,
					"container2_alts", alternatives,
				)
			}

			if route.Provider != container.Labels[labelProvider] {
				slog.Warn(
					"Multiple containers declare the same route with different providers",
					"container1_name", route.Upstreams[0].Name,
					"container2_name", container.Name,
					"route", primary,
					"container1_provider", route.Provider,
					"container2_provider", container.Labels[labelProvider],
				)
			}

			if route.Subject != container.Labels[labelSubject] {
				slog.Warn(
					"Multiple containers declare the same route with different subjects",
					"container1_name", route.Upstreams[0].Name,
					"container2_name", container.Name,
					"route", primary,
					"container1_subject", route.Subject,
					"container2_subject", container.Labels[labelSubject],
				)
			}
		}

		route.Upstreams = append(route.Upstreams, Upstream{
			Name: container.Name,
			Port: port,
		})

		maps.Copy(route.Headers, parseHeaders(container))
		maps.Copy(route.Errors, parseErrors(container))
	}

	return routes
}
