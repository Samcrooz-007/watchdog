# Watchdog

Watchdog is a privacy-preserving web analytics platform similar to Plausible,
where data visualisation is deferred to a Prometheus-compatible dashboard such
as Grafana. It is designed to be as simple and lightweight as possible, without
any additional web components to worry about. You, in turn, get to use your
existing monitoring stack to also collect information about your web
applications tracked by Watchdog.

## Design

- No raw event storage
- No persistent identifiers
- Bounded cardinality by design
- Aggregate at ingestion
- Prometheus-native export

## Quick Start

```bash
# Build the prıject
$ go build -o watchdog ./cmd/watchdog

# Start the Watchdog daemon with your own config
$ ./watchdog --config config.yaml
```
