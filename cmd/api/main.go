package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appdoc "multi-tenant-hybrid-search/internal/application/document"
	appt "multi-tenant-hybrid-search/internal/application/tenant"
	"multi-tenant-hybrid-search/internal/auth"
	"multi-tenant-hybrid-search/internal/config"
	httpapi "multi-tenant-hybrid-search/internal/delivery/http"
	"multi-tenant-hybrid-search/internal/delivery/middleware"
	"multi-tenant-hybrid-search/internal/infrastructure/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return fmt.Errorf("invalid LOG_LEVEL %q: %w", cfg.LogLevel, err)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pool, err := database.NewPool(startCtx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.CheckExtensions(startCtx, pool); err != nil {
		return err
	}

	ready := func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return err
		}
		return database.CheckExtensions(ctx, pool)
	}
	authn, err := auth.New(cfg.JWTSecret)
	if err != nil {
		return err
	}
	handler := httpapi.NewRouter(httpapi.Deps{
		Ready:     ready,
		Log:       log,
		Auth:      middleware.Auth(authn),
		Me:        appt.NewService(database.NewTenantRepository(pool)),
		Documents: appdoc.NewService(database.NewDocumentRepository(pool)),
	})
	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	return srv.Shutdown(shutCtx)
}
