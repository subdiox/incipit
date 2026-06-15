// Command incipit is a lightweight, modern, single-binary server for Calibre
// libraries: browse, read CBZ comics in the browser, manage metadata, OPDS, and
// multi-user access with local/LDAP/reverse-proxy auth.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"incipit/internal/appdb"
	"incipit/internal/auth"
	"incipit/internal/calibre"
	"incipit/internal/config"
	"incipit/internal/httpapi"
	"incipit/internal/reader"
)

func main() {
	// `incipit healthcheck` is used by the container HEALTHCHECK; distroless has
	// no shell or curl, so the binary checks itself.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// healthcheck performs a GET /healthz against the local server and returns a
// process exit code (0 = healthy).
func healthcheck() int {
	addr := os.Getenv("INCIPIT_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	lib, err := calibre.Open(cfg.LibraryPath, cfg.ReadOnly)
	if err != nil {
		return err
	}
	defer lib.Close()

	store, err := appdb.Open(cfg.AppDBPath())
	if err != nil {
		return err
	}
	defer store.Close()

	var external auth.ExternalAuthenticator
	if ldap := auth.NewLDAPAuthenticator(cfg.LDAP); ldap != nil {
		external = ldap
	}
	authSvc := auth.NewService(store, external)
	rd := reader.NewService(cfg.CacheDir)

	srv := httpapi.New(cfg, lib, store, authSvc, rd)

	// Periodically purge expired sessions.
	stopJanitor := startJanitor(store)
	defer stopJanitor()

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		slog.Info("incipit listening", "addr", cfg.Addr, "library", cfg.LibraryPath, "readonly", cfg.ReadOnly)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}

// startJanitor runs a background session cleanup until the returned stop func is
// called.
func startJanitor(store *appdb.Store) func() {
	ticker := time.NewTicker(time.Hour)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				if err := store.DeleteExpiredSessions(context.Background()); err != nil {
					slog.Warn("session cleanup", "error", err)
				}
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	return func() { close(done) }
}
