# Configuration

Watchdog supports multiple configuration sources with the following precedence
(highest to lowest):

1. **Command-line flags**
2. **Environment variables**
3. **Configuration file**
4. **Defaults**

## Configuration File

The primary configuration method is via YAML file. By default, Watchdog looks
for:

- `./config.yaml` (current directory)
- `/etc/watchdog/config.yaml` (system-wide)

Specify a custom location:

```bash
# Provide your configuration YAML file with --config
$ watchdog --config /path/to/config.yaml
```

See [config.example.yaml](../config.example.yaml) for all available options.

## Environment Variables

All configuration options can be set via environment variables with the
`WATCHDOG_` prefix.

Nested fields use underscore separators. For example:

```bash
# site.domains
$ export WATCHDOG_SITE_DOMAINS="example.com,blog.example.com"

# server.listen_addr
$ export WATCHDOG_SERVER_LISTEN_ADDR="127.0.0.1:8080"

# site.collect.pageviews
$ export WATCHDOG_SITE_COLLECT_PAGEVIEWS=true

# limits.max_paths
$ export WATCHDOG_LIMITS_MAX_PATHS=10000
```

### Common Environment Variables

```bash
# Server
WATCHDOG_SERVER_LISTEN_ADDR="127.0.0.1:8080"
WATCHDOG_SERVER_METRICS_PATH="/metrics"
WATCHDOG_SERVER_INGESTION_PATH="/api/event"
WATCHDOG_SERVER_STATE_PATH="/var/lib/watchdog/hll.state"

# Site
WATCHDOG_SITE_DOMAINS="example.com" # comma-separated for multiple
WATCHDOG_SITE_SALT_ROTATION="daily"
WATCHDOG_SITE_SAMPLING=1.0

# Collection
WATCHDOG_SITE_COLLECT_PAGEVIEWS=true
WATCHDOG_SITE_COLLECT_COUNTRY=true
WATCHDOG_SITE_COLLECT_DEVICE=true
WATCHDOG_SITE_COLLECT_REFERRER="domain"
WATCHDOG_SITE_COLLECT_DOMAIN=false

# Limits
WATCHDOG_LIMITS_MAX_PATHS=10000
WATCHDOG_LIMITS_MAX_SOURCES=500
WATCHDOG_LIMITS_MAX_CUSTOM_EVENTS=100
WATCHDOG_LIMITS_MAX_EVENTS_PER_MINUTE=10000

# Security
WATCHDOG_SECURITY_CORS_ENABLED=false
WATCHDOG_SECURITY_METRICS_AUTH_ENABLED=false
WATCHDOG_SECURITY_METRICS_AUTH_USERNAME="admin"
WATCHDOG_SECURITY_METRICS_AUTH_PASSWORD="changeme"
```

## Command-Line Flags

Command-line flags override both config file and environment variables:

```bash
# Override server address
watchdog --listen-addr :9090

# Override metrics path
watchdog --metrics-path /prometheus/metrics

# Override ingestion path
watchdog --ingestion-path /api/v1/event

# Combine multiple overrides
watchdog --config prod.yaml --listen-addr :9090 --metrics-path /metrics
```

Available flags:

- `--config string` - Path to config file
- `--listen-addr string` - Server listen address
- `--metrics-path string` - Metrics endpoint path
- `--ingestion-path string` - Ingestion endpoint path

## Configuration Precedence Example

Given:

**config.yaml:**

```yaml
server:
  listen_addr: ":8080"
  metrics_path: "/metrics"
```

**Environment:**

```bash
export WATCHDOG_SERVER_LISTEN_ADDR=":9090"
```

**Command:**

```bash
watchdog --metrics-path "/prometheus/metrics"
```

**Result:**

- `listen_addr`: `:9090` (from environment variable)
- `metrics_path`: `/prometheus/metrics` (from CLI flag)

## Systemd Integration

Environment variables work seamlessly with systemd:

```ini
[Service]
Environment="WATCHDOG_SERVER_LISTEN_ADDR=127.0.0.1:8080"
Environment="WATCHDOG_SITE_DOMAINS=example.com"
Environment="WATCHDOG_LIMITS_MAX_PATHS=10000"
ExecStart=/usr/local/bin/watchdog --config /etc/watchdog/config.yaml
```

Or use `EnvironmentFile`:

```ini
[Service]
EnvironmentFile=/etc/watchdog/env
ExecStart=/usr/local/bin/watchdog
```

**/etc/watchdog/env:**

```bash
WATCHDOG_SERVER_LISTEN_ADDR=127.0.0.1:8080
WATCHDOG_SITE_DOMAINS=example.com
WATCHDOG_LIMITS_MAX_PATHS=10000
```

## NixOS Integration

NixOS configuration automatically converts to the correct format:

```nix
{
  services.watchdog = {
    enable = true;
    settings = {
      site.domains = [ "example.com" ];
      server.listen_addr = "127.0.0.1:8080";
      limits.max_paths = 10000;
    };
  };
}
```

This is equivalent to setting environment variables or using a config file.

## Validation

Configuration is validated on startup. Invalid values will cause Watchdog to
exit with an error:

```bash
$ watchdog
Error: config validation failed: site.domains is required
```

Common validation errors:

- `site.domains is required` - No domains configured
- `limits.max_paths must be greater than 0` - Invalid cardinality limit
- `site.collect.referrer must be 'off', 'domain', or 'url'` - Invalid referrer
  mode
- `site.sampling must be between 0.0 and 1.0` - Invalid sampling rate

## Best Practices

1. **Use config file for base configuration** - Easier to version control and
   review
2. **Use environment variables for secrets** - Don't commit passwords to config
   files
3. **Use CLI flags for testing/overrides** - Quick temporary changes without
   editing files

Example hybrid approach:

**config.yaml:**

```yaml
site:
  domains:
    - example.com
  collect:
    pageviews: true
    device: true

limits:
  max_paths: 10000
```

**Environment (secrets):**

```bash
export WATCHDOG_SECURITY_METRICS_AUTH_PASSWORD="$SECRET_PASSWORD"
```

**CLI (testing):**

```bash
watchdog --listen-addr :9090  # Test on different port
```
