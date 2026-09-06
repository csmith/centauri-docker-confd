package main

import (
	"testing"

	"github.com/csmith/containuum"
	"github.com/stretchr/testify/assert"
)

func TestGenerateConfig(t *testing.T) {
	tests := []struct {
		name        string
		containers  []containuum.Container
		routeExtras string
		want        string
	}{
		{
			name:       "no containers produces empty config",
			containers: nil,
			want:       "",
		},
		{
			name: "container without vhost label is ignored",
			containers: []containuum.Container{
				{Name: "web", Labels: map[string]string{labelProxy: "80"}},
			},
			want: "",
		},
		{
			name: "container without a resolvable port is ignored",
			containers: []containuum.Container{
				{Name: "web", Labels: map[string]string{labelVhost: "example.com"}},
			},
			want: "",
		},
		{
			name: "minimal container with explicit port",
			containers: []containuum.Container{
				{
					Name:   "web",
					Labels: map[string]string{labelVhost: "example.com", labelProxy: "80"},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "port auto-detected from single unbound port",
			containers: []containuum.Container{
				{
					Name:   "web",
					Labels: map[string]string{labelVhost: "example.com"},
					Ports: []containuum.Port{
						{ContainerPort: 8080, HostPort: 0},
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:8080\n\n",
		},
		{
			name: "port is not auto-detected when multiple unbound ports exist",
			containers: []containuum.Container{
				{
					Name:   "web",
					Labels: map[string]string{labelVhost: "example.com"},
					Ports: []containuum.Port{
						{ContainerPort: 8080, HostPort: 0},
						{ContainerPort: 9090, HostPort: 0},
					},
				},
			},
			want: "",
		},
		{
			name: "host-bound ports are ignored for auto-detection",
			containers: []containuum.Container{
				{
					Name:   "web",
					Labels: map[string]string{labelVhost: "example.com"},
					Ports: []containuum.Port{
						{ContainerPort: 8080, HostPort: 32768},
						{ContainerPort: 9090, HostPort: 0},
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:9090\n\n",
		},
		{
			name: "explicit port label takes precedence over exposed ports",
			containers: []containuum.Container{
				{
					Name:   "web",
					Labels: map[string]string{labelVhost: "example.com", labelProxy: "80"},
					Ports: []containuum.Port{
						{ContainerPort: 8080, HostPort: 0},
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "invalid port label falls back to auto-detection",
			containers: []containuum.Container{
				{
					Name:   "web",
					Labels: map[string]string{labelVhost: "example.com", labelProxy: "notaport"},
					Ports: []containuum.Port{
						{ContainerPort: 8080, HostPort: 0},
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:8080\n\n",
		},
		{
			name: "vhost with alternatives, provider and subject",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:    "example.com, www.example.com foo.example.com",
						labelProxy:    "80",
						labelProvider: "letsencrypt",
						labelSubject:  "example.com *.example.com",
					},
				},
			},
			want: "route example.com www.example.com foo.example.com\n" +
				"    provider letsencrypt\n" +
				"    subject example.com *.example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "headers and errors are emitted sorted",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:            "example.com",
						labelProxy:            "80",
						labelHeaders + ".csp": "Content-Security-Policy: default-src 'self'",
						labelHeaders + ".sts": "Strict-Transport-Security: max-age=15768000",
						labelErrors + ".502":  "error-pages:8080/502.html",
						labelErrors + ".404":  "error-pages:8080/404.html",
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n" +
				"    on_error 404 error-pages:8080/404.html\n" +
				"    on_error 502 error-pages:8080/502.html\n" +
				"    header replace Content-Security-Policy default-src 'self'\n" +
				"    header replace Strict-Transport-Security max-age=15768000\n\n",
		},
		{
			name: "invalid header without colon is skipped",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:            "example.com",
						labelProxy:            "80",
						labelHeaders + ".bad": "no-colon-here",
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "out-of-range error status is skipped",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:           "example.com",
						labelProxy:           "80",
						labelErrors + ".200": "error-pages:8080/200.html",
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "route extras are indented and trimmed",
			containers: []containuum.Container{
				{
					Name:   "web",
					Labels: map[string]string{labelVhost: "example.com", labelProxy: "80"},
				},
			},
			routeExtras: "header default Strict-Transport-Security max-age=15768000\n\nheader delete Server\n",
			want: "route example.com\n" +
				"    upstream web:80\n" +
				"    header default Strict-Transport-Security max-age=15768000\n" +
				"    header delete Server\n\n",
		},
		{
			name: "splithosts emits one full route per vhost",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:            "example.com, www.example.com",
						labelProxy:            "80",
						labelSplitHosts:       "true",
						labelHeaders + ".sts": "Strict-Transport-Security: max-age=15768000",
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n" +
				"    header replace Strict-Transport-Security max-age=15768000\n\n" +
				"route www.example.com\n" +
				"    upstream web:80\n" +
				"    header replace Strict-Transport-Security max-age=15768000\n\n",
		},
		{
			name: "splithosts set to a false value does not split",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:      "example.com, www.example.com",
						labelProxy:      "80",
						labelSplitHosts: "false",
					},
				},
			},
			want: "route example.com www.example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "splithosts accepts shorthand values like 1",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:      "example.com, www.example.com",
						labelProxy:      "80",
						labelSplitHosts: "1",
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n\n" +
				"route www.example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "splithosts with a duplicate hostname emits the route once",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:      "example.com, example.com",
						labelProxy:      "80",
						labelSplitHosts: "true",
					},
				},
			},
			want: "route example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "unparsable splithosts value is ignored and does not split",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:      "example.com, www.example.com",
						labelProxy:      "80",
						labelSplitHosts: "yes",
					},
				},
			},
			want: "route example.com www.example.com\n" +
				"    upstream web:80\n\n",
		},
		{
			name: "splithosts mismatch between containers warns and keeps the first",
			containers: []containuum.Container{
				{Name: "web1", Labels: map[string]string{labelVhost: "example.com, www.example.com", labelProxy: "80"}},
				{
					Name: "web2",
					Labels: map[string]string{
						labelVhost:      "example.com, www.example.com",
						labelProxy:      "80",
						labelSplitHosts: "true",
					},
				},
			},
			want: "route example.com www.example.com\n" +
				"    upstream web1:80\n" +
				"    upstream web2:80\n\n",
		},
		{
			name: "splithosts combined with subject is an error and produces no routes",
			containers: []containuum.Container{
				{
					Name: "web",
					Labels: map[string]string{
						labelVhost:      "example.com, www.example.com",
						labelProxy:      "80",
						labelSplitHosts: "true",
						labelSubject:    "example.com",
					},
				},
			},
			want: "",
		},
		{
			name: "multiple containers on same vhost become multiple upstreams",
			containers: []containuum.Container{
				{Name: "web1", Labels: map[string]string{labelVhost: "example.com", labelProxy: "80"}},
				{Name: "web2", Labels: map[string]string{labelVhost: "example.com", labelProxy: "80"}},
			},
			want: "route example.com\n" +
				"    upstream web1:80\n" +
				"    upstream web2:80\n\n",
		},
		{
			name: "distinct vhosts are emitted sorted by primary hostname",
			containers: []containuum.Container{
				{Name: "zeta", Labels: map[string]string{labelVhost: "zeta.example.com", labelProxy: "80"}},
				{Name: "alpha", Labels: map[string]string{labelVhost: "alpha.example.com", labelProxy: "80"}},
			},
			want: "route alpha.example.com\n" +
				"    upstream alpha:80\n\n" +
				"route zeta.example.com\n" +
				"    upstream zeta:80\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateConfig(tt.containers, tt.routeExtras)
			assert.Equal(t, tt.want, got)
		})
	}
}
