package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/csmith/containuum/v3"
)

// GenerateConfig generates a Centauri route configuration from containers
func GenerateConfig(containers []containuum.Container, routeExtras string) string {
	routes := groupByHostname(containers)

	primaries := make([]string, 0, len(routes))
	for primary := range routes {
		primaries = append(primaries, primary)
	}
	sort.Strings(primaries)

	var sb strings.Builder

	for _, primary := range primaries {
		route := routes[primary]

		if !route.SplitHosts {
			writeRoute(&sb, route, route.Primary, route.Alternatives, routeExtras)
			continue
		}

		seen := make(map[string]bool, 1+len(route.Alternatives))
		for _, host := range append([]string{route.Primary}, route.Alternatives...) {
			if seen[host] {
				continue
			}
			seen[host] = true
			writeRoute(&sb, route, host, nil, routeExtras)
		}
	}

	return sb.String()
}

func writeRoute(sb *strings.Builder, route *RouteInfo, host string, alternatives []string, routeExtras string) {
	sb.WriteString("route ")
	sb.WriteString(host)
	for _, alt := range alternatives {
		sb.WriteString(" ")
		sb.WriteString(alt)
	}
	sb.WriteString("\n")

	if route.Provider != "" {
		fmt.Fprintf(sb, "    provider %s\n", route.Provider)
	}

	if route.Subject != "" && !route.SplitHosts {
		fmt.Fprintf(sb, "    subject %s\n", route.Subject)
	}

	for _, upstream := range route.Upstreams {
		fmt.Fprintf(sb, "    upstream %s:%d\n", upstream.Name, upstream.Port)
	}

	statuses := make([]int, 0, len(route.Errors))
	for status := range route.Errors {
		statuses = append(statuses, status)
	}
	sort.Ints(statuses)
	for _, status := range statuses {
		fmt.Fprintf(sb, "    on_error %d %s\n", status, route.Errors[status])
	}

	for line := range strings.SplitSeq(routeExtras, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			fmt.Fprintf(sb, "    %s\n", line)
		}
	}

	headerNames := make([]string, 0, len(route.Headers))
	for name := range route.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		fmt.Fprintf(sb, "    header replace %s %s\n", name, route.Headers[name])
	}

	sb.WriteString("\n")
}
