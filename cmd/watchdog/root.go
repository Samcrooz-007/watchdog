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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"notashelf.dev/watchdog/internal/aggregate"
	"notashelf.dev/watchdog/internal/api"
	"notashelf.dev/watchdog/internal/config"
	"notashelf.dev/watchdog/internal/health"
	"notashelf.dev/watchdog/internal/limits"
	"notashelf.dev/watchdog/internal/normalize"
	"notashelf.dev/watchdog/internal/ratelimit"
)

func Run(cfg *config.Config) error {
	log.Printf("Loaded config for domains: %v", cfg.Site.Domains)

	// Initialize components
	pathNormalizer := normalize.NewPathNormalizer(cfg.Site.Path)
	pathRegistry := aggregate.NewPathRegistry(cfg.Limits.MaxPaths)
	refRegistry := normalize.NewReferrerRegistry(cfg.Limits.MaxSources)
	eventRegistry := aggregate.NewCustomEventRegistry(cfg.Limits.MaxCustomEvents)
	metricsAgg := aggregate.NewMetricsAggregator(pathRegistry, eventRegistry, cfg)

	// Metric for tracking blocked file requests (scrapers/bots)
	blockedRequests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "web_blocked_requests_total",
			Help: "File server requests blocked by security filters",
		},
		[]string{"reason"},
	)

	// Load HLL state from previous run if it exists
	if cfg.Site.SaltRotation != "" {
		log.Println("HLL state persistence enabled")
		if err := metricsAgg.LoadState(); err != nil {
			log.Printf("Could not load HLL state (might be first run): %v", err)
		} else {
			log.Println("HLL state restored from previous run")
		}
	}

	// Register Prometheus metrics
	promRegistry := prometheus.NewRegistry()
	metricsAgg.MustRegister(promRegistry)
	promRegistry.MustRegister(blockedRequests)

	// Register health and runtime metrics
	healthCollector := health.NewCollector(getVersion(), getCommit(), buildDate)
	if err := healthCollector.Register(promRegistry); err != nil {
		return fmt.Errorf("failed to register health metrics: %w", err)
	}

	// Create HTTP handlers
	ingestionHandler := api.NewIngestionHandler(
		cfg,
		pathNormalizer,
		pathRegistry,
		refRegistry,
		metricsAgg,
	)

	// Setup routes
	mux := http.NewServeMux()

	// Metrics endpoint with optional basic auth and rate limiting
	metricsHandler := promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})

	if cfg.Security.MetricsAuth.Enabled {
		metricsHandler = basicAuth(
			metricsHandler,
			cfg.Security.MetricsAuth.Username,
			cfg.Security.MetricsAuth.Password,
		)
	}

	// Add rate limiting to metrics endpoint
	if cfg.Limits.MaxMetricsPerMinute > 0 {
		metricsRateLimiter := ratelimit.NewTokenBucket(
			cfg.Limits.MaxMetricsPerMinute,
			cfg.Limits.MaxMetricsPerMinute,
			time.Minute,
		)
		metricsHandler = rateLimitMiddleware(metricsHandler, metricsRateLimiter)
	}

	mux.Handle(cfg.Server.MetricsPath, metricsHandler)

	// Ingestion endpoint
	mux.Handle(cfg.Server.IngestionPath, ingestionHandler)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Serve whitelisted static files from /web/ if the directory exists
	if info, err := os.Stat("web"); err == nil && info.IsDir() {
		log.Println("Serving static files from /web/")
		mux.Handle("/web/", safeFileServer("web", blockedRequests))
	}

	// Create HTTP server with timeouts
	srv := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      mux,
		ReadTimeout:  limits.HTTPReadTimeout,
		WriteTimeout: limits.HTTPWriteTimeout,
		IdleTimeout:  limits.HTTPIdleTimeout,
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

		// Give outstanding requests time to complete
		ctx, cancel := context.WithTimeout(context.Background(), limits.ShutdownTimeout)
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

// Wraps a handler with rate limiting
func rateLimitMiddleware(next http.Handler, limiter *ratelimit.TokenBucket) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Sanitizes a path for logging to prevent log injection attacks. Uses `strconv.Quote`
// to properly escape control characters and special bytes.
func sanitizePathForLog(path string) string {
	escaped := strconv.Quote(path)
	if len(escaped) >= 2 && escaped[0] == '"' && escaped[len(escaped)-1] == '"' {
		escaped = escaped[1 : len(escaped)-1]
	}

	// Limit path length to prevent log flooding
	const maxLen = 200
	if len(escaped) > maxLen {
		return escaped[:maxLen] + "..."
	}

	return escaped
}

// Creates a file server that only serves whitelisted files. Blocks dotfiles, .git, .env, etc.
// TODO: I need to hook this up to eris somehow so I can just forward the paths that are being
// scanned despite not being on a whitelist. Would be a good way of detecting scrapers, maybe.
func safeFileServer(root string, blockedRequests *prometheus.CounterVec) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		path := filepath.Clean(r.URL.Path)

		// Block directory listings
		if strings.HasSuffix(path, "/") {
			blockedRequests.WithLabelValues("directory_listing").Inc()
			log.Printf("Blocked directory listing attempt: %s from %s", sanitizePathForLog(path), r.RemoteAddr)
			http.NotFound(w, r)
			return
		}

		// Block dotfiles and sensitive files
		for segment := range strings.SplitSeq(path, "/") {
			if strings.HasPrefix(segment, ".") {
				blockedRequests.WithLabelValues("dotfile").Inc()
				log.Printf("Blocked dotfile access: %s from %s", sanitizePathForLog(path), r.RemoteAddr)
				http.NotFound(w, r)
				return
			}
			// Block common sensitive files
			lower := strings.ToLower(segment)
			if strings.Contains(lower, ".env") ||
				strings.Contains(lower, "config") ||
				strings.HasSuffix(lower, ".bak") ||
				strings.HasSuffix(lower, "~") {
				blockedRequests.WithLabelValues("sensitive_file").Inc()
				log.Printf("Blocked sensitive file access: %s from %s", sanitizePathForLog(path), r.RemoteAddr)
				http.NotFound(w, r)
				return
			}
		}

		// Only serve .js, .html, .css files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".js" && ext != ".html" && ext != ".css" {
			blockedRequests.WithLabelValues("invalid_extension").Inc()
			log.Printf("Blocked invalid extension: %s from %s", sanitizePathForLog(path), r.RemoteAddr)
			http.NotFound(w, r)
			return
		}

		http.StripPrefix("/web/", fs).ServeHTTP(w, r)
	})
}
