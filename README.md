# Watchdog

Privacy-preserving web analytics with Prometheus-native metrics.

## Design

- No raw event storage
- No persistent identifiers
- Bounded cardinality by design
- Aggregate at ingestion
- Prometheus-native export

## Quick Start

```bash
go build -o watchdog ./cmd/watchdog
./watchdog --config config.yaml
```
