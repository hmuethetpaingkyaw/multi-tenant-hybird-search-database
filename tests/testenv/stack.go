package testenv

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"multi-tenant-hybrid-search/api/ollama/mock"
	"multi-tenant-hybrid-search/driver/auth"
	"multi-tenant-hybrid-search/http/controller"
	"multi-tenant-hybrid-search/http/middleware"
	"multi-tenant-hybrid-search/http/router"
	"multi-tenant-hybrid-search/repository"
	"multi-tenant-hybrid-search/usecase"
)

// NewRouter wires the production stack (repositories, usecases, controllers, router) over pool with the
// given fake embedder. This is the only place the integration tests depend on how the layers are assembled.
func NewRouter(log *slog.Logger, pool *pgxpool.Pool, authn *auth.Authenticator, emb *mock_ollama.Fake, ready controller.ReadyFunc) http.Handler {
	return router.New(log, middleware.Auth(authn), router.Controllers{
		Health:   controller.NewHealthController(ready, log),
		Tenant:   controller.NewTenantController(usecase.NewTenantUsecase(repository.NewTenantRepository(pool))),
		Document: controller.NewDocumentController(usecase.NewDocumentUsecase(repository.NewDocumentRepository(pool), emb)),
		Search:   controller.NewSearchController(usecase.NewSearchUsecase(repository.NewSearchRepository(pool), emb)),
	})
}

func NewDocumentUsecase(e *Env, emb *mock_ollama.Fake) *usecase.DocumentUsecase {
	return usecase.NewDocumentUsecase(repository.NewDocumentRepository(e.App), emb)
}
