package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/app/usecase"
	"github.com/momaek/tolato/internal/server/domain/audit"
	"github.com/momaek/tolato/internal/server/domain/node"
	"github.com/momaek/tolato/internal/server/domain/outbox"
	"github.com/momaek/tolato/internal/server/domain/plan"
	"github.com/momaek/tolato/internal/server/domain/policy"
	"github.com/momaek/tolato/internal/server/domain/task"
	infraauth "github.com/momaek/tolato/internal/server/infra/auth"
	"github.com/momaek/tolato/internal/server/infra/dispatch"
	"github.com/momaek/tolato/internal/server/infra/idgen"
	"github.com/momaek/tolato/internal/server/infra/llm"
	"github.com/momaek/tolato/internal/server/infra/memory"
	infpostgres "github.com/momaek/tolato/internal/server/infra/postgres"
	"github.com/momaek/tolato/internal/server/infra/presence"
	"github.com/momaek/tolato/internal/server/infra/queue"
	infraredis "github.com/momaek/tolato/internal/server/infra/redis"
	infrasummary "github.com/momaek/tolato/internal/server/infra/summary"
	"github.com/momaek/tolato/internal/server/infra/telemetry"
	"github.com/momaek/tolato/internal/server/transport/httpapi"
	"github.com/momaek/tolato/internal/server/transport/wsagent"
	"github.com/momaek/tolato/internal/server/transport/wsui"
	"github.com/momaek/tolato/internal/shared/config"
	"github.com/momaek/tolato/internal/shared/types"
	"go.uber.org/zap"
)

type ServerApp struct {
	cfg        config.ServerConfig
	logger     *zap.Logger
	httpServer *http.Server
	runners    []func(ctx context.Context)
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
	var authSessions infraauth.SessionStore
	if redisClient != nil {
		authSessions = infraauth.NewRedisSessionStore(redisClient, "tolato:auth:session:")
	} else {
		authSessions = infraauth.NewMemorySessionStore()
	}
	authService := infraauth.NewService(cfg, authSessions)
	idGenerator := idgen.NewUUIDGenerator()
	planner := llm.NewPlanner(types.LLMConfig{
		Provider: cfg.LLM.Provider,
		BaseURL:  cfg.LLM.BaseURL,
		APIKey:   cfg.LLM.APIKey,
		Model:    cfg.LLM.Model,
	})
	schemaValidator := plan.StaticSchemaValidator{}
	policyValidator := policy.NewStaticValidator()
	summaryService := infrasummary.NewService(types.LLMConfig{
		Provider: cfg.LLM.Provider,
		BaseURL:  cfg.LLM.BaseURL,
		APIKey:   cfg.LLM.APIKey,
		Model:    cfg.LLM.Model,
	})
	dispatcher := dispatch.NewManager(logger)
	queueStream := queue.NewStream(redisClient, "tolato:task-queue")

	var (
		nodeRepo     node.Repository
		sessionStore node.SessionStore
		taskRepo     task.Repository
		auditRepo    audit.Repository
		outboxRepo   outbox.Repository
	)

	if pool != nil {
		pgNodeRepo, pgSessionStore, pgTaskRepo, pgAuditRepo, pgOutboxRepo := infpostgres.NewStores(pool)
		nodeRepo = pgNodeRepo
		sessionStore = pgSessionStore
		taskRepo = pgTaskRepo
		auditRepo = pgAuditRepo
		outboxRepo = pgOutboxRepo
	} else {
		memNodeRepo, memSessionStore, memTaskRepo, memAuditRepo, memOutboxRepo := memory.NewStores()
		nodeRepo = memNodeRepo
		sessionStore = memSessionStore
		taskRepo = memTaskRepo
		auditRepo = memAuditRepo
		outboxRepo = memOutboxRepo
	}

	usecases := usecase.NewServices(planner, schemaValidator, policyValidator, summaryService, nodeRepo, sessionStore, taskRepo, auditRepo, outboxRepo, idGenerator)

	uiWS := wsui.NewHandler(logger)
	presenceStore := presence.NewStore()
	agentWS := wsagent.NewHandler(
		logger,
		usecases.AuthenticateAgent,
		usecases.HeartbeatNode,
		usecases.DisconnectNode,
		usecases.GetNode,
		usecases.RecordTaskLog,
		usecases.RecordTaskResult,
		dispatcher,
		presenceStore,
		uiWS,
	)
	router := httpapi.NewRouter(httpapi.Dependencies{
		Logger:                 logger,
		Auth:                   &authService,
		UseCases:               usecases,
		DB:                     pool,
		Redis:                  redisClient,
		Presence:               presenceStore,
		UIWS:                   uiWS,
		AgentWS:                agentWS,
		RequireSecureTransport: strings.ToLower(cfg.Server.Environment) != "dev",
		TrustProxyTLS:          cfg.Server.TrustProxyTLS,
	})

	server := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	runners := []func(ctx context.Context){
		func(ctx context.Context) {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					runOutboxRelay(ctx, logger, outboxRepo, queueStream)
				}
			}
		},
		func(ctx context.Context) {
			runDispatchWorker(ctx, logger, queueStream, dispatcher, taskRepo, auditRepo, idGenerator)
		},
		func(ctx context.Context) {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					events, err := usecases.TimeoutTasks.Execute(ctx)
					if err != nil {
						logger.Warn("task watchdog failed", zap.Error(err))
						continue
					}
					for _, event := range events {
						uiWS.BroadcastTaskStatus(event.TaskID, event.TaskStatus, time.Now().UTC())
						uiWS.BroadcastTaskResult(event.TaskID, event.Execution, time.Now().UTC())
					}
				}
			}
		},
	}

	return &ServerApp{
		cfg:        cfg,
		logger:     logger,
		httpServer: server,
		runners:    runners,
	}, nil
}

func (a *ServerApp) Run(ctx context.Context) error {
	a.logger.Info("starting tolato-server", zap.String("address", a.cfg.Server.Address))
	for _, runner := range a.runners {
		if runner != nil {
			go runner(ctx)
		}
	}

	if strings.ToLower(a.cfg.Server.Environment) != "dev" && a.cfg.Server.TLSCert == "" && a.cfg.Server.TLSKey == "" && !a.cfg.Server.TrustProxyTLS {
		return errors.New("tls is required outside dev")
	}

	errCh := make(chan error, 1)
	go func() {
		var err error
		if a.cfg.Server.TLSCert != "" && a.cfg.Server.TLSKey != "" {
			err = a.httpServer.ListenAndServeTLS(a.cfg.Server.TLSCert, a.cfg.Server.TLSKey)
		} else {
			err = a.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
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
