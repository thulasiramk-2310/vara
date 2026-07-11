// Package commands — vara serve (RFC-0016 §9).
//
// RunServe starts the HTTP binding of the remote transport protocol over a
// directory of repositories. It is intentionally minimal for v1: discover
// repositories by path, route requests, log them, shut down gracefully. No
// authentication, users, database, or web UI (those are RFC-0017/0018 and the
// Hub).
package commands

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/thulasiramk-2310/vara/internal/server"
)

// RunServe serves the repositories under root on addr until interrupted.
func RunServe(addr, root string) error {
	if addr == "" {
		addr = ":8080"
	}
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("serve: resolve root: %w", err)
	}
	if fi, err := os.Stat(absRoot); err != nil || !fi.IsDir() {
		return fmt.Errorf("serve: root %q is not a directory", root)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: requestLogger(server.Handler(absRoot)),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("vara serve: listening on %s (root %s)\n", addr, absRoot)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		fmt.Println("\nvara serve: shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// requestLogger logs each request's method, path, status, and duration.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
