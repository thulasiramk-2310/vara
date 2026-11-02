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
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/repomanager"
	"github.com/thulasiramk-2310/vara/internal/server"
)

// ServeConfig configures identity, authorization, and the repository control
// plane for `vara serve`. Empty fields leave the server anonymous / allow-all
// (RFC-0016 behavior).
type ServeConfig struct {
	PolicyDir   string // enables authorization (RFC-0018) when set
	MetaDir     string // enables the RFC-0019 control plane when set (requires PolicyDir)
	AccountsDir string // enables the RFC-0020 account/session/token control plane when set
	HubDir      string // serves the RFC-0021 same-origin static Hub UI when set
	// AllowAnonymousAll lets a server with no --policy listen on a non-local
	// address. Without a policy every caller, signed in or not, may read AND write
	// every repository, so this must be asked for explicitly.
	AllowAnonymousAll bool
	Basic             map[string]string // user -> secret (RFC-0017 static Basic)
	Bearer            map[string]string // token -> subject (RFC-0017 static Bearer)
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

	if err := checkServeSafety(addr, cfg); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	opts, err := buildServerOptions(cfg, absRoot)
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

// buildServerOptions assembles identity, authorization, and the control plane
// from the config. With no credentials the server is anonymous; with a policy
// directory it enforces authorization (RFC-0018); with a metadata directory it
// also serves the repository control plane (RFC-0019).
func buildServerOptions(cfg ServeConfig, absRoot string) (server.Options, error) {
	var opts server.Options

	var sources []identity.IdentitySource
	hasBasic, hasBearer := false, false
	if len(cfg.Basic) > 0 {
		sources = append(sources, identity.NewBasicSource(cfg.Basic))
		hasBasic = true
	}
	if len(cfg.Bearer) > 0 {
		sources = append(sources, identity.NewBearerSource(cfg.Bearer))
		hasBearer = true
	}

	// RFC-0020: durable accounts contribute persistent Basic + Bearer sources
	// backed by the same manager that also serves the account control plane.
	if cfg.AccountsDir != "" {
		absAcct, err := filepath.Abs(cfg.AccountsDir)
		if err != nil {
			return opts, fmt.Errorf("resolve accounts dir: %w", err)
		}
		mgr, err := identity.NewAccountManager(absAcct)
		if err != nil {
			return opts, fmt.Errorf("accounts: %w", err)
		}
		opts.Accounts = mgr
		sources = append(sources, mgr.BasicSource(), mgr.BearerSource())
		hasBasic, hasBearer = true, true
	}

	if hasBasic {
		opts.Methods = append(opts.Methods, "auth-basic")
	}
	if hasBearer {
		opts.Methods = append(opts.Methods, "auth-bearer")
	}
	if len(sources) > 0 {
		// Credentials required for writes, but allow anonymous so a policy can
		// still grant anonymous reads (RFC-0017: anonymous is an identity).
		opts.Identity = &identity.Multi{Sources: sources, AllowAnonymous: true}
		opts.Methods = append(opts.Methods, "auth-anonymous")
	}

	var absPolicy string
	if cfg.PolicyDir != "" {
		p, err := filepath.Abs(cfg.PolicyDir)
		if err != nil {
			return opts, fmt.Errorf("resolve policy dir: %w", err)
		}
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			return opts, fmt.Errorf("policy dir %q is not a directory", cfg.PolicyDir)
		}
		absPolicy = p
		store := authz.NewStore(absPolicy)
		opts.Authz = authz.NewEnforcer(store, log.Default())
	}

	if cfg.MetaDir != "" {
		// The control plane seeds ownership as policy (RFC-0019 §7.3), so it
		// cannot run without a policy store to write to.
		if absPolicy == "" {
			return opts, fmt.Errorf("the repository control plane (--meta) requires --policy")
		}
		absMeta, err := filepath.Abs(cfg.MetaDir)
		if err != nil {
			return opts, fmt.Errorf("resolve metadata dir: %w", err)
		}
		mgr, err := repomanager.New(absRoot, absPolicy, absMeta)
		if err != nil {
			return opts, err
		}
		opts.Manager = mgr
	}

	if cfg.HubDir != "" {
		absHub, err := filepath.Abs(cfg.HubDir)
		if err != nil {
			return opts, fmt.Errorf("resolve hub dir: %w", err)
		}
		if fi, err := os.Stat(absHub); err != nil || !fi.IsDir() {
			return opts, fmt.Errorf("hub dir %q is not a directory", cfg.HubDir)
		}
		opts.HubDir = absHub
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

// checkServeSafety refuses the one dangerous combination: no per-repository
// policy (so every caller may read and write everything) on an address other
// machines can reach. A policy, or a loopback address, or an explicit
// --insecure-allow-all, makes it acceptable.
func checkServeSafety(addr string, cfg ServeConfig) error {
	if cfg.PolicyDir != "" || cfg.AllowAnonymousAll || isLoopbackAddr(addr) {
		return nil
	}
	return fmt.Errorf("refusing to listen on %q without --policy: every caller could read and write every repository.\n"+
		"  Use --policy <dir> to enforce permissions (see docs/DEPLOYMENT.md),\n"+
		"  or bind to the local machine only with --addr 127.0.0.1:8080,\n"+
		"  or pass --insecure-allow-all if an anonymous, allow-all server is really what you want", addr)
}

// isLoopbackAddr reports whether addr (host:port, or just :port) listens only on
// the local machine. An empty host means every interface, which is not loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
