package bootstrap

import (
	"context"

	"github.com/momaek/tolato/internal/agent/executor/runner"
	validatorpkg "github.com/momaek/tolato/internal/agent/executor/validator"
	"github.com/momaek/tolato/internal/agent/infra/agentstate"
	"github.com/momaek/tolato/internal/agent/infra/cancellation"
	"github.com/momaek/tolato/internal/agent/infra/persistence"
	"github.com/momaek/tolato/internal/agent/infra/sysinfo"
	"github.com/momaek/tolato/internal/agent/infra/telemetry"
	"github.com/momaek/tolato/internal/agent/loop/connection"
	"github.com/momaek/tolato/internal/agent/loop/dispatch"
	"github.com/momaek/tolato/internal/agent/loop/execution"
	"github.com/momaek/tolato/internal/agent/loop/supervisor"
	"github.com/momaek/tolato/internal/agent/transport/enroll"
	"github.com/momaek/tolato/internal/agent/transport/wsclient"
	"github.com/momaek/tolato/internal/shared/config"
	"github.com/momaek/tolato/internal/shared/protocol"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type AgentApp struct {
	cfg    config.AgentConfig
	logger *zap.Logger
	run    func(ctx context.Context) error
}

func NewAgentApp(configPath string) (*AgentApp, error) {
	cfg, err := config.LoadAgentConfig(configPath)
	if err != nil {
		return nil, err
	}

	logger, err := telemetry.NewLogger()
	if err != nil {
		return nil, err
	}

	identityStore := persistence.NewFileStore(cfg.Agent.IdentityFile)
	enrollClient := enroll.NewClient(cfg.Agent.ServerBaseURL)
	client := wsclient.NewClient(logger)

	incoming := make(chan protocol.Envelope, 32)
	queue := make(chan runner.Job, 32)
	cancelQueue := make(chan runner.CancelRequest, 32)
	runnerImpl := runner.NewActionRunner()
	validatorImpl := validatorpkg.NewRegistryValidator()
	busyTracker := agentstate.NewBusyTracker()
	cancelStore := cancellation.NewStore()
	sysInfoCollector := sysinfo.NewCollector(busyTracker)

	connectionLoop := connection.Loop{
		Logger:       logger,
		Config:       cfg,
		Store:        identityStore,
		EnrollClient: enrollClient,
		WSClient:     client,
		Incoming:     incoming,
		SysInfo:      sysInfoCollector,
	}
	dispatchLoop := dispatch.Loop{
		Logger:   logger,
		Incoming: incoming,
		Queue:    queue,
		Cancel:   cancelQueue,
		WSClient: client,
	}
	executionLoop := execution.Loop{
		Logger:    logger,
		NodeID:    cfg.Agent.Hostname,
		Queue:     queue,
		Cancel:    cancelQueue,
		Runner:    runnerImpl,
		Validator: validatorImpl,
		WSClient:  client,
		Busy:      busyTracker,
		Cancels:   cancelStore,
	}
	supervisorLoop := supervisor.Loop{
		Logger: logger,
		Queue:  queue,
	}

	return &AgentApp{
		cfg:    cfg,
		logger: logger,
		run: func(ctx context.Context) error {
			group, groupCtx := errgroup.WithContext(ctx)
			group.Go(func() error { return connectionLoop.Run(groupCtx) })
			group.Go(func() error { return dispatchLoop.Run(groupCtx) })
			group.Go(func() error { return executionLoop.Run(groupCtx) })
			group.Go(func() error { return supervisorLoop.Run(groupCtx) })
			return group.Wait()
		},
	}, nil
}

func (a *AgentApp) Run(ctx context.Context) error {
	a.logger.Info("starting tolato-agent", zap.String("hostname", a.cfg.Agent.Hostname))
	return a.run(ctx)
}
