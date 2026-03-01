package watchdog

import (
	"flag"
	"log"
)

func Main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	if err := Run(*configPath); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
