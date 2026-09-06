package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/csmith/containuum"
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

		writeRoute := func(host string, alternatives []string) {
			sb.WriteString("route ")
			sb.WriteString(host)
			for _, alt := range alternatives {
				sb.WriteString(" ")
				sb.WriteString(alt)
			}
			sb.WriteString("\n")

			if route.Provider != "" {
				sb.WriteString("    provider ")
				sb.WriteString(route.Provider)
				sb.WriteString("\n")
			}

			if route.Subject != "" && !route.SplitHosts {
				sb.WriteString("    subject ")
				sb.WriteString(route.Subject)
				sb.WriteString("\n")
			}

			for _, upstream := range route.Upstreams {
				sb.WriteString("    upstream ")
				sb.WriteString(upstream.Name)
				sb.WriteString(":")
				fmt.Fprintf(&sb, "%d", upstream.Port)
				sb.WriteString("\n")
			}

			if len(route.Errors) > 0 {
				statuses := make([]int, 0, len(route.Errors))
				for status := range route.Errors {
					statuses = append(statuses, status)
				}
				sort.Ints(statuses)

				for _, status := range statuses {
					sb.WriteString("    on_error ")
					fmt.Fprintf(&sb, "%d", status)
					sb.WriteString(" ")
					sb.WriteString(route.Errors[status])
					sb.WriteString("\n")
				}
			}

			if routeExtras != "" {
				for line := range strings.SplitSeq(routeExtras, "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						sb.WriteString("    ")
						sb.WriteString(line)
						sb.WriteString("\n")
					}
				}
			}

			if len(route.Headers) > 0 {
				headerNames := make([]string, 0, len(route.Headers))
				for name := range route.Headers {
					headerNames = append(headerNames, name)
				}
				sort.Strings(headerNames)

				for _, name := range headerNames {
					sb.WriteString("    header replace ")
					sb.WriteString(name)
					sb.WriteString(" ")
					sb.WriteString(route.Headers[name])
					sb.WriteString("\n")
				}
			}

			sb.WriteString("\n")
		}

		if route.SplitHosts {
			seen := make(map[string]bool, 1+len(route.Alternatives))
			for _, host := range append([]string{route.Primary}, route.Alternatives...) {
				if seen[host] {
					continue
				}
				seen[host] = true
				writeRoute(host, nil)
			}
		} else {
			writeRoute(route.Primary, route.Alternatives)
		}
	}

	return sb.String()
}
