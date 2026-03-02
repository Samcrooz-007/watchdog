package main

import "notashelf.dev/watchdog/cmd/watchdog"

// Injected at build time via ldflags:
//
// -X main.Version=v1.0.0
// -X main.Commit=abc1234
// -X main.BuildDate=2026-03-02
//
// I hate this pattern btw.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	watchdog.Main(Version, Commit, BuildDate)
}
