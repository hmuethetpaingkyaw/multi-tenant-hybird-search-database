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

	"multi-tenant-hybrid-search/api/ollama"
	"multi-tenant-hybrid-search/config"
	"multi-tenant-hybrid-search/driver/auth"
	"multi-tenant-hybrid-search/driver/db"
	"multi-tenant-hybrid-search/http/controller"
	"multi-tenant-hybrid-search/http/middleware"
	"multi-tenant-hybrid-search/http/router"
	"multi-tenant-hybrid-search/repository"
	"multi-tenant-hybrid-search/usecase"
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
	pool, err := db.NewPool(startCtx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.CheckExtensions(startCtx, pool); err != nil {
		return err
	}

	ready := func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return err
		}
		return db.CheckExtensions(ctx, pool)
	}
	authn, err := auth.New(cfg.JWTSecret)
	if err != nil {
		return err
	}

	// Explicit dependency injection: adapters -> repositories -> usecases -> controllers -> router.
	embedder := ollama.NewOllama(cfg.OllamaURL, cfg.EmbeddingModel, cfg.EmbeddingTimeout)
	tenantRepository := repository.NewTenantRepository(pool)
	documentRepository := repository.NewDocumentRepository(pool)
	searchRepository := repository.NewSearchRepository(pool)

	handler := router.New(log, middleware.Auth(authn), router.Controllers{
		Health:   controller.NewHealthController(ready, log),
		Tenant:   controller.NewTenantController(usecase.NewTenantUsecase(tenantRepository)),
		Document: controller.NewDocumentController(usecase.NewDocumentUsecase(documentRepository, embedder)),
		Search:   controller.NewSearchController(usecase.NewSearchUsecase(searchRepository, embedder)),
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
