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

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/server"
)

// ServeConfig configures identity and authorization for `vara serve`. Empty
// fields leave the server anonymous / allow-all (RFC-0016 behavior).
type ServeConfig struct {
	PolicyDir string            // enables authorization (RFC-0018) when set
	Basic     map[string]string // user -> secret (RFC-0017 Basic)
	Bearer    map[string]string // token -> subject (RFC-0017 Bearer)
}

// RunServe serves the repositories under root on addr until interrupted.
func RunServe(addr, root string, cfg ServeConfig) error {
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

	opts, err := buildServerOptions(cfg)
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: requestLogger(server.HandlerWithOptions(absRoot, opts)),
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

// buildServerOptions assembles identity and authorization from the config. With
// no credentials configured the server is anonymous; with a policy directory it
// enforces authorization (RFC-0018).
func buildServerOptions(cfg ServeConfig) (server.Options, error) {
	var opts server.Options

	var sources []identity.IdentitySource
	if len(cfg.Basic) > 0 {
		sources = append(sources, identity.NewBasicSource(cfg.Basic))
		opts.Methods = append(opts.Methods, "auth-basic")
	}
	if len(cfg.Bearer) > 0 {
		sources = append(sources, identity.NewBearerSource(cfg.Bearer))
		opts.Methods = append(opts.Methods, "auth-bearer")
	}
	if len(sources) > 0 {
		// Credentials required for writes, but allow anonymous so a policy can
		// still grant anonymous reads (RFC-0017: anonymous is an identity).
		opts.Identity = &identity.Multi{Sources: sources, AllowAnonymous: true}
		opts.Methods = append(opts.Methods, "auth-anonymous")
	}

	if cfg.PolicyDir != "" {
		absPolicy, err := filepath.Abs(cfg.PolicyDir)
		if err != nil {
			return opts, fmt.Errorf("resolve policy dir: %w", err)
		}
		if fi, err := os.Stat(absPolicy); err != nil || !fi.IsDir() {
			return opts, fmt.Errorf("policy dir %q is not a directory", cfg.PolicyDir)
		}
		store := authz.NewStore(absPolicy)
		opts.Authz = authz.NewEnforcer(store, log.Default())
	}
	return opts, nil
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
