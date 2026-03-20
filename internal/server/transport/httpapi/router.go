package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/momaek/tolato/internal/server/app/usecase"
	infraauth "github.com/momaek/tolato/internal/server/infra/auth"
	"github.com/momaek/tolato/internal/server/infra/presence"
	infraredis "github.com/momaek/tolato/internal/server/infra/redis"
	"github.com/momaek/tolato/internal/server/transport/wsagent"
	"github.com/momaek/tolato/internal/server/transport/wsui"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Dependencies struct {
	Logger   *zap.Logger
	Auth     *infraauth.Service
	UseCases usecase.Services
	DB       *pgxpool.Pool
	Redis    *goredis.Client
	Presence *presence.Store
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
		presence: deps.Presence,
		uiws:     deps.UIWS,
	}

	r := chi.NewRouter()
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)

	r.Get("/ws/ui", func(w http.ResponseWriter, r *http.Request) {
		if deps.Auth == nil {
			http.Error(w, "authentication is not configured", http.StatusUnauthorized)
			return
		}
		if _, err := deps.Auth.AuthenticateRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		deps.UIWS.ServeHTTP(w, r)
	})
	r.Get("/ws/agent", deps.AgentWS.ServeHTTP)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", h.Login)
		r.Get("/me", h.Me)
		r.Get("/nodes", h.ListNodes)
		r.Get("/nodes/{id}", h.GetNode)
		r.Get("/tasks", h.ListTasks)
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
