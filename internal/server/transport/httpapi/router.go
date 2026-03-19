package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/momaek/tolato/internal/server/app/usecase"
	infraauth "github.com/momaek/tolato/internal/server/infra/auth"
	infraredis "github.com/momaek/tolato/internal/server/infra/redis"
	"github.com/momaek/tolato/internal/server/transport/wsagent"
	"github.com/momaek/tolato/internal/server/transport/wsui"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Dependencies struct {
	Logger   *zap.Logger
	Auth     infraauth.Service
	UseCases usecase.Services
	DB       *pgxpool.Pool
	Redis    *goredis.Client
	UIWS     *wsui.Handler
	AgentWS  *wsagent.Handler
}

func NewRouter(deps Dependencies) http.Handler {
	h := Handler{
		logger:   deps.Logger,
		auth:     deps.Auth,
		usecases: deps.UseCases,
		db:       deps.DB,
		redis:    deps.Redis,
	}

	r := chi.NewRouter()
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)

	r.Get("/ws/ui", deps.UIWS.ServeHTTP)
	r.Get("/ws/agent", deps.AgentWS.ServeHTTP)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", h.Login)
		r.Get("/me", h.Me)
		r.Get("/nodes", h.ListNodes)
		r.Get("/nodes/{id}", h.GetNode)
		r.Post("/tasks/plan", h.GenerateTaskPlan)
		r.Post("/tasks/{id}/approve", h.ApproveTask)
		r.Post("/tasks/{id}/reject", h.RejectTask)
		r.Post("/tasks/{id}/cancel", h.CancelTask)
		r.Get("/tasks/{id}", h.GetTask)
		r.Get("/tasks/{id}/executions", h.ListTaskExecutions)
		r.Get("/audits", h.ListAudits)
		r.Post("/agent/enroll", h.EnrollAgent)
	})

	return r
}

func dependenciesReady(ctx context.Context, db *pgxpool.Pool, redis *goredis.Client) error {
	if db != nil {
		if err := db.Ping(ctx); err != nil {
			return err
		}
	}
	return infraredis.Ping(ctx, redis)
}
