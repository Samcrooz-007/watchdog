package watchdog

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"notashelf.dev/watchdog/internal/aggregate"
	"notashelf.dev/watchdog/internal/api"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/normalize"
)

func Run(configPath string) error {
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Printf("Loaded config for domain: %s", cfg.Site.Domain)

	// Initialize components
	pathNormalizer := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, *cfg)

	// Register Prometheus metrics
	promRegistry := prometheus.NewRegistry()
	metricsAgg.MustRegister(promRegistry)

	// Create HTTP handlers
	ingestionHandler := api.NewIngestionHandler(*cfg, pathNormalizer, pathRegistry, refRegistry, metricsAgg)

	// Setup routes
	mux := http.NewServeMux()

	// Metrics endpoint
	mux.Handle(cfg.Server.MetricsPath, promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Ingestion endpoint
	mux.Handle(cfg.Server.IngestionPath, ingestionHandler)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Serve static files from /web/ if the directory exists
	if info, err := os.Stat("web"); err == nil && info.IsDir() {
		log.Println("Serving static files from /web/")
		mux.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir("web"))))
	}

	// Start server
	log.Printf("Starting server on %s", cfg.Server.ListenAddr)
	log.Printf("Metrics endpoint: %s", cfg.Server.MetricsPath)
	log.Printf("Ingestion endpoint: %s", cfg.Server.IngestionPath)

	return http.ListenAndServe(cfg.Server.ListenAddr, mux)
}
