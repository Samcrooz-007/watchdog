package watchdog

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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

	log.Printf("Loaded config for domains: %v", cfg.Site.Domains)

	// Initialize components
	pathNormalizer := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, *cfg)

	// HLL state persistence is handled automatically if salt_rotation is configured
	if cfg.Site.SaltRotation != "" {
		log.Println("HLL state persistence enabled")
	}

	// Register Prometheus metrics
	promRegistry := prometheus.NewRegistry()
	metricsAgg.MustRegister(promRegistry)

	// Create HTTP handlers
	ingestionHandler := api.NewIngestionHandler(*cfg, pathNormalizer, pathRegistry, refRegistry, metricsAgg)

	// Setup routes
	mux := http.NewServeMux()

	// Metrics endpoint with optional basic auth
	metricsHandler := promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})

	if cfg.Security.MetricsAuth.Enabled {
		metricsHandler = basicAuth(metricsHandler, cfg.Security.MetricsAuth.Username, cfg.Security.MetricsAuth.Password)
	}

	mux.Handle(cfg.Server.MetricsPath, metricsHandler)

	// Ingestion endpoint
	mux.Handle(cfg.Server.IngestionPath, ingestionHandler)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Serve whitelisted static files from /web/ if the directory exists
	if info, err := os.Stat("web"); err == nil && info.IsDir() {
		log.Println("Serving static files from /web/")
		mux.Handle("/web/", safeFileServer("web"))
	}

	// Create HTTP server with timeouts
	srv := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Starting server on %s", cfg.Server.ListenAddr)
		log.Printf("Metrics endpoint: %s", cfg.Server.MetricsPath)
		log.Printf("Ingestion endpoint: %s", cfg.Server.IngestionPath)
		serverErrors <- srv.ListenAndServe()
	}()

	// Listen for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)
	case sig := <-shutdown:
		log.Printf("Received signal: %v, starting graceful shutdown", sig)

		// Give outstanding requests 30 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Shutdown metrics aggregator.
		// This stops background goroutines, and saves HLL state
		if err := metricsAgg.Shutdown(ctx); err != nil {
			log.Printf("Error during metrics shutdown: %v", err)
		}

		// Shutdown HTTP server
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Error during HTTP shutdown: %v", err)
			return fmt.Errorf("shutdown error: %w", err)
		}

		log.Println("Graceful shutdown complete")
		return nil
	}
}

// Wraps a handler with HTTP Basic Authentication
func basicAuth(next http.Handler, username, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Metrics"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Creates a file server that only serves whitelisted files. Blocks dotfiles, .git, .env, etc.
// TODO: I need to hook this up to eris somehow so I can just forward the paths that are being
// scanned despite not being on a whitelist. Would be a good way of detecting scrapers, maybe.
func safeFileServer(root string) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		path := filepath.Clean(r.URL.Path)

		// Block directory listings
		if strings.HasSuffix(path, "/") {
			http.NotFound(w, r)
			return
		}

		// Block dotfiles and sensitive files
		for segment := range strings.SplitSeq(path, "/") {
			if strings.HasPrefix(segment, ".") {
				http.NotFound(w, r)
				return
			}
			// Block common sensitive files
			lower := strings.ToLower(segment)
			if strings.Contains(lower, ".env") ||
				strings.Contains(lower, "config") ||
				strings.HasSuffix(lower, ".bak") ||
				strings.HasSuffix(lower, "~") {
				http.NotFound(w, r)
				return
			}
		}

		// Only serve .js, .html, .css files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".js" && ext != ".html" && ext != ".css" {
			http.NotFound(w, r)
			return
		}

		http.StripPrefix("/web/", fs).ServeHTTP(w, r)
	})
}
