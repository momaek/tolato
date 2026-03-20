package bootstrap

import (
	"context"
	"net/http"
	"time"

	"github.com/momaek/tolato/internal/server/app/usecase"
	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/node"
	"github.com/momaek/tolato/internal/server/domain/plan"
	"github.com/momaek/tolato/internal/server/domain/policy"
	"github.com/momaek/tolato/internal/server/domain/task"
	infraauth "github.com/momaek/tolato/internal/server/infra/auth"
	"github.com/momaek/tolato/internal/server/infra/idgen"
	"github.com/momaek/tolato/internal/server/infra/llm"
	"github.com/momaek/tolato/internal/server/infra/memory"
	infpostgres "github.com/momaek/tolato/internal/server/infra/postgres"
	"github.com/momaek/tolato/internal/server/infra/presence"
	infraredis "github.com/momaek/tolato/internal/server/infra/redis"
	"github.com/momaek/tolato/internal/server/infra/telemetry"
	"github.com/momaek/tolato/internal/server/transport/httpapi"
	"github.com/momaek/tolato/internal/server/transport/wsagent"
	"github.com/momaek/tolato/internal/server/transport/wsui"
	"github.com/momaek/tolato/internal/shared/config"
	"go.uber.org/zap"
)

type ServerApp struct {
	cfg        config.ServerConfig
	logger     *zap.Logger
	httpServer *http.Server
}

func NewServerApp(ctx context.Context, configPath string) (*ServerApp, error) {
	cfg, err := config.LoadServerConfig(configPath)
	if err != nil {
		return nil, err
	}

	logger, err := telemetry.NewLogger()
	if err != nil {
		return nil, err
	}

	pool, err := infpostgres.NewPool(ctx, cfg.Postgres.DSN)
	if err != nil {
		return nil, err
	}

	redisClient := infraredis.NewClient(cfg.Redis.Addr, cfg.Redis.DB)
	authService := infraauth.NewService(cfg.Auth.AdminUsername, cfg.Auth.AdminPassword)
	idGenerator := idgen.NewUUIDGenerator()
	planner := llm.NewStubPlanner()
	schemaValidator := plan.StaticSchemaValidator{}
	policyValidator := policy.NewStaticValidator()

	var (
		nodeRepo     node.Repository
		sessionStore node.SessionStore
		taskRepo     task.Repository
		auditRepo    audit.Repository
	)

	if pool != nil {
		pgNodeRepo, pgSessionStore, pgTaskRepo, pgAuditRepo := infpostgres.NewStores(pool)
		nodeRepo = pgNodeRepo
		sessionStore = pgSessionStore
		taskRepo = pgTaskRepo
		auditRepo = pgAuditRepo
	} else {
		memNodeRepo, memSessionStore, memTaskRepo, memAuditRepo := memory.NewStores()
		nodeRepo = memNodeRepo
		sessionStore = memSessionStore
		taskRepo = memTaskRepo
		auditRepo = memAuditRepo
	}

	usecases := usecase.NewServices(planner, schemaValidator, policyValidator, nodeRepo, sessionStore, taskRepo, auditRepo, idGenerator)

	uiWS := wsui.NewHandler(logger)
	presenceStore := presence.NewStore()
	agentWS := wsagent.NewHandler(
		logger,
		usecases.AuthenticateAgent,
		usecases.HeartbeatNode,
		usecases.GetNode,
		usecases.RecordTaskLog,
		usecases.RecordTaskResult,
		presenceStore,
		uiWS,
	)
	router := httpapi.NewRouter(httpapi.Dependencies{
		Logger:   logger,
		Auth:     authService,
		UseCases: usecases,
		DB:       pool,
		Redis:    redisClient,
		Presence: presenceStore,
		UIWS:     uiWS,
		AgentWS:  agentWS,
	})

	server := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &ServerApp{
		cfg:        cfg,
		logger:     logger,
		httpServer: server,
	}, nil
}

func (a *ServerApp) Run(ctx context.Context) error {
	a.logger.Info("starting tolato-server", zap.String("address", a.cfg.Server.Address))

	errCh := make(chan error, 1)
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		a.logger.Info("shutting down tolato-server")
		return a.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
