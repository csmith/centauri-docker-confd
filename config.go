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

		sb.WriteString("route ")
		sb.WriteString(route.Primary)
		for _, alt := range route.Alternatives {
			sb.WriteString(" ")
			sb.WriteString(alt)
		}
		sb.WriteString("\n")

		for _, upstream := range route.Upstreams {
			sb.WriteString("    upstream ")
			sb.WriteString(upstream.Name)
			sb.WriteString(":")
			sb.WriteString(fmt.Sprintf("%d", upstream.Port))
			sb.WriteString("\n")
		}

		if routeExtras != "" {
			lines := strings.Split(routeExtras, "\n")
			for _, line := range lines {
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

	return sb.String()
}
