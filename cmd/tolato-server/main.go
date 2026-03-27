package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"time"

	appauth "github.com/momaek/tolato/internal/server/app/auth"
	appexecution "github.com/momaek/tolato/internal/server/app/execution"
	"github.com/momaek/tolato/internal/server/app/history"
	"github.com/momaek/tolato/internal/server/app/nodeview"
	"github.com/momaek/tolato/internal/server/app/policy"
	"github.com/momaek/tolato/internal/server/app/recovery"
	appruntime "github.com/momaek/tolato/internal/server/app/runtime"
	appsession "github.com/momaek/tolato/internal/server/app/session"
	"github.com/momaek/tolato/internal/server/app/settings"
	"github.com/momaek/tolato/internal/server/domain"
	"github.com/momaek/tolato/internal/server/infra"
	"github.com/momaek/tolato/internal/server/infra/config"
	"github.com/momaek/tolato/internal/server/infra/devseed"
	settingsllm "github.com/momaek/tolato/internal/server/infra/llm/settings"
	"github.com/momaek/tolato/internal/server/infra/lock"
	devnodes "github.com/momaek/tolato/internal/server/infra/nodes"
	"github.com/momaek/tolato/internal/server/infra/store/memory"
	storepostgres "github.com/momaek/tolato/internal/server/infra/store/postgres"
	infraws "github.com/momaek/tolato/internal/server/infra/ws"
	"github.com/momaek/tolato/internal/server/transport/ginhttp"
	"github.com/momaek/tolato/internal/server/transport/ginws"
	"github.com/momaek/tolato/internal/server/transport/wsagent"
	"github.com/momaek/tolato/internal/server/transport/wsui"
)

func main() {
	configPath := flag.String("config", "configs/server.local.yaml", "path to server config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	repos, cleanup, err := openRepositories(context.Background(), cfg)
	if err != nil {
		log.Fatalf("open repositories: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	clock := infra.SystemClock{}
	ids := infra.RandomIDGenerator{}
	locks := lock.NewMemoryLockManager()
	logger := infra.NewLogger(nil, slog.LevelInfo)
	if err := devseed.EnsureConsoleSession(context.Background(), repos.Sessions, time.Now().UTC()); err != nil {
		log.Fatalf("bootstrap console session: %v", err)
	}
	authService := appauth.NewService(appauth.Repositories{
		Settings:     repos.Settings,
		AuthSessions: repos.AuthSessions,
	}, appauth.Config{
		AdminUsername: cfg.Auth.AdminUsername,
		AdminPassword: cfg.Auth.AdminPassword,
		AgentToken:    cfg.Auth.AgentToken,
	})
	if err := authService.BootstrapAdmin(context.Background()); err != nil {
		log.Fatalf("bootstrap auth: %v", err)
	}

	hub := infraws.NewMemoryHub()
	sessionRegistry := infraws.NewMemorySessionRegistry(hub)
	agentRegistry := infraws.NewMemoryAgentRegistry(hub)
	nodeSource := devnodes.NewObservedSource(nil, agentRegistry)
	nodeService := nodeview.NewService(nodeSource, nodeview.Repositories{
		Sessions:   repos.Sessions,
		Tasks:      repos.Tasks,
		Executions: repos.Executions,
	})
	historyService := history.NewService(history.Repositories{
		Sessions:    repos.Sessions,
		Tasks:       repos.Tasks,
		Timelines:   repos.Timelines,
		ToolCalls:   repos.ToolCalls,
		Executions:  repos.Executions,
		Audits:      repos.Audits,
		ToolResults: repos.ToolResults,
	})
	settingsService := settings.NewService(settings.Repositories{
		Settings: repos.Settings,
		Security: authService,
		Models:   settingsllm.Catalog{},
	})
	eventPublisher := wsui.NewPublisher(sessionRegistry)
	sessionService := appsession.NewService(appsession.Repositories{
		Sessions:      repos.Sessions,
		Timelines:     repos.Timelines,
		Tasks:         repos.Tasks,
		Executions:    repos.Executions,
		Subscriptions: sessionRegistry,
	}, appsession.WithClock(clock), appsession.WithIDGenerator(ids))

	execRef := &executionStarterRef{}
	policyRegistry := policy.NewRegistry(nodeSource, policy.WithExecutionStarter(execRef))
	runtimeService := appruntime.NewService(appruntime.Repositories{
		Sessions:    repos.Sessions,
		Messages:    repos.ThreadMessages,
		Timelines:   repos.Timelines,
		ToolCalls:   repos.ToolCalls,
		ToolResults: repos.ToolResults,
		Tasks:       repos.Tasks,
		Executions:  repos.Executions,
		Audits:      repos.Audits,
	}, &settingsllm.Provider{
		Settings:      repos.Settings,
		DefaultUserID: cfg.Auth.AdminUsername,
		Logger:        logger,
		Events:        eventPublisher,
		IDGenerator:   ids,
	}, appruntime.NewPolicyToolRegistry(policyRegistry), clock, ids, appruntime.WithEventPublisher(eventPublisher), appruntime.WithLockManager(locks), appruntime.WithLogger(logger))

	dispatchPublisher := wsagent.NewDispatchPublisher(agentRegistry)
	executionService := appexecution.NewService(appexecution.Repositories{
		Sessions:    repos.Sessions,
		Tasks:       repos.Tasks,
		Executions:  repos.Executions,
		Timelines:   repos.Timelines,
		ToolResults: repos.ToolResults,
		Audits:      repos.Audits,
	}, clock, ids, appexecution.WithEventPublisher(eventPublisher), appexecution.WithDispatchPublisher(dispatchPublisher), appexecution.WithCompletionHandler(runtimeService), appexecution.WithLockManager(locks), appexecution.WithLogger(logger))
	execRef.service = executionService

	uiHandler := wsui.Handler{
		Auth:          wsui.TokenAuthenticator{Auth: authService},
		Hub:           hub,
		Subscriptions: sessionRegistry,
		Dispatcher: wsui.Dispatcher{
			Sessions:  sessionService,
			Runtime:   runtimeService,
			Execution: executionService,
		},
	}
	agentHandler := wsagent.Handler{
		Auth: wsagent.TokenAuthenticator{Auth: authService},
		Hub:  hub,
		Dispatcher: wsagent.Dispatcher{
			Agents:     agentRegistry,
			Executions: executionService,
		},
	}

	recoveryService := recovery.NewService(recovery.Repositories{
		Sessions:   repos.Sessions,
		Executions: repos.Executions,
		Audits:     repos.Audits,
	}, clock, ids, recovery.WithRuntimeResumer(runtimeService))
	report, err := recoveryService.Scan(context.Background())
	if err != nil {
		log.Fatalf("startup recovery scan: %v", err)
	}
	logRecoveryReport(report)

	router := ginhttp.NewRouter(ginhttp.Handler{
		Nodes:      nodeService,
		History:    historyService,
		Settings:   settingsService,
		Auth:       authService,
		Execution:  executionService,
		AgentToken: cfg.Auth.AgentToken,
	})
	ginws.RegisterUIRoute(router, cfg.Server.UIWSPath, uiHandler)
	ginws.RegisterAgentRoute(router, cfg.Server.AgentWSPath, agentHandler)

	server := &http.Server{
		Addr:    cfg.Server.HTTPAddress,
		Handler: router,
	}

	log.Printf("tolato-server listening on %s", cfg.Server.HTTPAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen and serve: %v", err)
	}
}

type repositoryBundle struct {
	Sessions       domain.SessionRepository
	ThreadMessages domain.ThreadMessageRepository
	Timelines      domain.TimelineRepository
	ToolCalls      domain.ToolCallRepository
	ToolResults    domain.ToolResultRepository
	Tasks          domain.TaskRepository
	Executions     domain.ExecutionRepository
	Audits         domain.AuditRepository
	AuthSessions   domain.AuthSessionRepository
	Settings       domain.SettingsRepository
}

func openRepositories(ctx context.Context, cfg config.Config) (repositoryBundle, func(), error) {
	switch cfg.Store.Driver {
	case "memory":
		store := memory.NewStore()
		now := time.Now().UTC()
		if err := devseed.SeedConsoleStore(ctx, store, now); err != nil {
			return repositoryBundle{}, nil, err
		}
		if err := devseed.SeedHistoryStore(ctx, store, now.Add(-30*time.Minute)); err != nil {
			return repositoryBundle{}, nil, err
		}
		if err := devseed.SeedSettingsStore(ctx, store, cfg.Auth.AdminUsername, now); err != nil {
			return repositoryBundle{}, nil, err
		}
		return repositoryBundle{
			Sessions:       store.Sessions,
			ThreadMessages: store.ThreadMessages,
			Timelines:      store.Timelines,
			ToolCalls:      store.ToolCalls,
			ToolResults:    store.ToolResults,
			Tasks:          store.Tasks,
			Executions:     store.Executions,
			Audits:         store.Audits,
			AuthSessions:   store.AuthSessions,
			Settings:       store.Settings,
		}, func() {}, nil
	case "postgres":
		db, err := storepostgres.Open(cfg.Store.DSN)
		if err != nil {
			return repositoryBundle{}, nil, err
		}
		pgStore := storepostgres.NewStore(storepostgres.SQLDB{DB: db})
		return repositoryBundle{
				Sessions:       pgStore.Sessions,
				ThreadMessages: pgStore.ThreadMessages,
				Timelines:      pgStore.Timelines,
				ToolCalls:      pgStore.ToolCalls,
				ToolResults:    pgStore.ToolResults,
				Tasks:          pgStore.Tasks,
				Executions:     pgStore.Executions,
				Audits:         pgStore.Audits,
				AuthSessions:   pgStore.AuthSessions,
				Settings:       pgStore.Settings,
			}, func() {
				_ = db.Close()
			}, nil
	default:
		return repositoryBundle{}, nil, errors.New("unsupported store driver")
	}
}

type executionStarterRef struct {
	service appexecution.Service
}

func (r *executionStarterRef) StartUpgrade(ctx context.Context, input appexecution.StartUpgradeInput) (appexecution.StartDispatchResult, error) {
	if r.service == nil {
		return appexecution.StartDispatchResult{}, errors.New("execution service is not configured")
	}
	return r.service.StartUpgrade(ctx, input)
}

func (r *executionStarterRef) StartDispatch(ctx context.Context, input appexecution.StartDispatchInput) (appexecution.StartDispatchResult, error) {
	if r.service == nil {
		return appexecution.StartDispatchResult{}, errors.New("execution service is not configured")
	}
	return r.service.StartDispatch(ctx, input)
}

func (r *executionStarterRef) CancelTask(ctx context.Context, sessionID string, taskID string, idempotencyKey string) error {
	if r.service == nil {
		return errors.New("execution service is not configured")
	}
	return r.service.CancelTask(ctx, sessionID, taskID, idempotencyKey)
}

func (r *executionStarterRef) RecordChunk(ctx context.Context, input appexecution.RecordChunkInput) error {
	if r.service == nil {
		return errors.New("execution service is not configured")
	}
	return r.service.RecordChunk(ctx, input)
}

func (r *executionStarterRef) FinishExecution(ctx context.Context, input appexecution.FinishExecutionInput) error {
	if r.service == nil {
		return errors.New("execution service is not configured")
	}
	return r.service.FinishExecution(ctx, input)
}

func (r *executionStarterRef) SendShellInput(ctx context.Context, input appexecution.ShellInputInput) error {
	if r.service == nil {
		return errors.New("execution service is not configured")
	}
	return r.service.SendShellInput(ctx, input)
}

func (r *executionStarterRef) ResizeShell(ctx context.Context, input appexecution.ShellResizeInput) error {
	if r.service == nil {
		return errors.New("execution service is not configured")
	}
	return r.service.ResizeShell(ctx, input)
}

func logRecoveryReport(report recovery.ScanReport) {
	if len(report.FailedRunning) == 0 && len(report.PausedWaiting) == 0 && len(report.WaitingAsync) == 0 {
		return
	}

	log.Printf(
		"startup recovery: failed_running=%d paused_waiting=%d waiting_async=%d",
		len(report.FailedRunning),
		len(report.PausedWaiting),
		len(report.WaitingAsync),
	)
}
