# centauri-docker-confd

A config server that monitors Docker containers and generates [Centauri](https://github.com/csmith/centauri)
route configuration, serving it via the network config protocol.

- Monitors Docker containers using [containuum](https://github.com/csmith/containuum)
- Generates Centauri route configuration from container labels (compatible with [Dotege](https://github.com/csmith/Dotege) labels)
- Serves configuration over TCP using the Centauri network config protocol
- Automatically pushes updates when containers start/stop or labels change

## Docker Labels

centauri-docker-confd uses the following labels:

- `com.chameth.vhost` - Comma/space-delimited list of hostnames (required)
- `com.chameth.proxy` - Port number (auto-detected from single exposed port if omitted)
- `com.chameth.headers.*` - Response headers (format: `Header-Name: value`)
- `com.chameth.proxytag` - Optional tag for filtering containers

## Configuration

Configuration is done via command-line flags or environment variables:

| Flag             | Environment Variable | Default | Description                                                        |
|------------------|----------------------|---------|--------------------------------------------------------------------|
| `--listen`       | `LISTEN`             | `:8080` | TCP address to listen on                                           |
| `--route-extras` | `ROUTE_EXTRAS`       | (empty) | Lines to include in every route block (can contain newlines)       |
| `--proxytag`     | `PROXYTAG`           | (empty) | Only process containers with matching `com.chameth.proxytag` label |

## Example Usage

### Setting default headers

```bash
ROUTE_EXTRAS="header default Strict-Transport-Security max-age=15768000
header delete Server" ./centauri-docker-confd
```

### Configuring Centauri

Configure Centauri to connect to centauri-docker-confd:

```bash
CONFIG_SOURCE=network CONFIG_NETWORK_ADDRESS=localhost:8080 centauri
```

### Container labels

```yaml
services:
  web:
    image: nginx
    labels:
      com.chameth.vhost: "example.com www.example.com"
      com.chameth.proxy: "80"
      com.chameth.headers.csp: "Content-Security-Policy: default-src 'self'"
```

This generates a Centauri config like:

```
route example.com www.example.com
    upstream web:80
    header replace Content-Security-Policy default-src 'self'
```


## Provenance

This project was primarily created with Claude Code, but with a strong guiding
hand. It's not "vibe coded", but an LLM was still the primary author of most
lines of code. I believe it meets the same sort of standards I'd aim for with
hand-crafted code, but some slop may slip through. I understand if you
prefer not to use LLM-created software, and welcome human-authored alternatives
(I just don't personally have the time/motivation to do so).
