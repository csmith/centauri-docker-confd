package main

import (
	"strconv"
	"strings"

	"github.com/csmith/containuum"
)

const (
	labelVhost   = "com.chameth.vhost"
	labelProxy   = "com.chameth.proxy"
	labelHeaders = "com.chameth.headers"
)

// RouteInfo represents the routing configuration for a hostname
type RouteInfo struct {
	Primary      string
	Alternatives []string
	Upstreams    []Upstream
	Headers      map[string]string
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
			return port
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
		return int(unboundPorts[0].ContainerPort)
	}

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
			}
		}
	}

	return headers
}

// shouldProxy checks if a container should be proxied (has vhost and valid port)
func shouldProxy(container containuum.Container) bool {
	vhost, ok := container.Labels[labelVhost]
	if !ok || vhost == "" {
		return false
	}

	port := parsePort(container)
	return port != -1
}

// groupByHostname groups containers by their primary hostname
func groupByHostname(containers []containuum.Container) map[string]*RouteInfo {
	routes := make(map[string]*RouteInfo)

	for _, container := range containers {
		if !shouldProxy(container) {
			continue
		}

		vhost := container.Labels[labelVhost]
		primary, alternatives := parseVhosts(vhost)
		port := parsePort(container)

		if primary == "" || port == -1 {
			continue
		}

		route, exists := routes[primary]
		if !exists {
			route = &RouteInfo{
				Primary:      primary,
				Alternatives: alternatives,
				Headers:      make(map[string]string),
			}
			routes[primary] = route
		}

		route.Upstreams = append(route.Upstreams, Upstream{
			Name: container.Name,
			Port: port,
		})

		for k, v := range parseHeaders(container) {
			route.Headers[k] = v
		}
	}

	return routes
}
