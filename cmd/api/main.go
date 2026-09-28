package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
	"xprem/config"
	"xprem/internal/bucketmigration"
	"xprem/internal/metrics"
	infrastructure "xprem/internal/router"

	_ "xprem/internal/bucketmigrations"
)

// bootHandler answers while the bucket migrations run, and splits the two probes
// on purpose. /hc (liveness) is registered below and answers 200 throughout, so
// the orchestrator does not kill a pod in the middle of a long migration.
// /ready (readiness) is deliberately NOT registered: it falls into the catch-all
// and answers 503 + Retry-After, which drops the pod from the Service endpoints
// without killing it, so nothing is served from a half-migrated bucket. Every
// other request gets that same 503. Once main swaps in the real router, /ready
// answers 200 like /hc (see internal/router/router.go).
func bootHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/hc", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "15")
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "storage migration in progress, updates are on hold", http.StatusServiceUnavailable)
	})
	return mux
}

func main() {
	config.LoadConfig()
	metrics.InitMetrics()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	var handler atomic.Pointer[http.Handler]
	boot := bootHandler()
	handler.Store(&boot)

	server := &http.Server{
		Addr: net.JoinHostPort(config.GetBindAddress(), config.GetPort()),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			(*handler.Load()).ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	// Bind synchronously: port conflicts must fail before touching storage.
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	defer server.Close()
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	log.Println("Server is running on " + server.Addr)

	// The lock is released on failure, so a crash-looping pod retries the
	// migration on every boot instead of skipping it while the lock expires.
	if err := bucketmigration.EnsureMigrations(); err != nil {
		return err
	}

	// Keep dependency workers alive until active HTTP requests have drained.
	dependencyCtx, cancelDependencies := context.WithCancel(context.Background())
	defer cancelDependencies()
	container, cleanup := infrastructure.InitDependencies(dependencyCtx)
	defer cleanup()
	router := infrastructure.NewRouter(container)
	// No global CORS: each surface declares its own where its routes are
	// registered (dashboard subrouters, OAuth endpoints, /mcp).
	ready := http.Handler(router)
	handler.Store(&ready)
	log.Println("✅ Server is ready to serve traffic.")
	select {
	case <-ctx.Done():
		log.Println("Shutdown requested; draining active requests.")
		// New requests arriving on an existing connection receive 503 while
		// in-flight publish/download requests retain their dependencies.
		draining := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "15")
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
		}))
		handler.Store(&draining)
		return shutdownServer(server, 30*time.Second)
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func shutdownServer(server *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		// Bounded termination also covers stalled uploads and persistent SSE.
		_ = server.Close()
		return err
	}
	return nil
}
