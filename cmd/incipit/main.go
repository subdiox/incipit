// Command incipit is a lightweight, modern, single-binary server for Calibre
// libraries: browse, read CBZ comics in the browser, manage metadata, OPDS, and
// multi-user access with local/LDAP/reverse-proxy auth.
package main

import (
	"context"
	"encoding/json"
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

	store, err := appdb.Open(cfg.AppDBPath())
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()

	// The library path may come from app.db (chosen during first-run setup) or
	// INCIPIT_LIBRARY; when neither is set the server starts unconfigured and
	// setup collects it.
	lib, err := openConfiguredLibrary(ctx, store, cfg)
	if err != nil {
		return err
	}
	if lib != nil {
		defer lib.Close()
	}

	ldapMgr := auth.NewLDAPManager(loadLDAPSettings(ctx, store, cfg))
	authSvc := auth.NewService(store, ldapMgr)
	rd := reader.NewService(cfg.CacheDir)

	srv := httpapi.New(cfg, lib, store, authSvc, rd, ldapMgr)

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

// openConfiguredLibrary resolves the Calibre library path (app.db setting from
// first-run setup, else INCIPIT_LIBRARY) and opens it. It returns (nil, nil)
// when no path is configured yet, so the server can start and let setup choose
// one.
func openConfiguredLibrary(ctx context.Context, store *appdb.Store, cfg *config.Config) (*calibre.Adapter, error) {
	path, _ := store.GetSetting(ctx, httpapi.LibraryPathKey)
	fromSetting := path != ""
	if path == "" {
		path = cfg.LibraryPath // INCIPIT_LIBRARY
	}
	if path == "" {
		slog.Info("no library configured; first-run setup will prompt for the path")
		return nil, nil
	}
	lib, err := calibre.Open(path, cfg.ReadOnly)
	if err != nil {
		return nil, err
	}
	// Persist an env-provided path so the library stays configured even if
	// INCIPIT_LIBRARY is later removed (avoids a locked-out, setup-complete-but-
	// library-missing state).
	if !fromSetting {
		_ = store.SetSetting(ctx, httpapi.LibraryPathKey, path)
	}
	return lib, nil
}

// loadLDAPSettings reads the admin-editable LDAP settings from app.db, falling
// back to the connection-related environment variables on first run so existing
// env-based deployments keep working (and become editable in the admin UI).
func loadLDAPSettings(ctx context.Context, store *appdb.Store, cfg *config.Config) auth.LDAPSettings {
	if raw, _ := store.GetSetting(ctx, httpapi.LDAPSettingKey); raw != "" {
		var s auth.LDAPSettings
		if json.Unmarshal([]byte(raw), &s) == nil {
			return s
		}
	}
	return auth.LDAPSettings{
		Enabled:      cfg.LDAP.Enabled,
		URL:          cfg.LDAP.URL,
		StartTLS:     cfg.LDAP.StartTLS,
		BaseDN:       cfg.LDAP.BaseDN,
		UserFilter:   cfg.LDAP.UserFilter,
		AdminGroupDN: cfg.LDAP.AdminGroupDN,
		LoginGroupDN: cfg.LDAP.LoginGroupDN,
	}
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
