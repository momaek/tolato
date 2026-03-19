package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/momaek/tolato/internal/agent/app/bootstrap"
	"github.com/spf13/cobra"
)

func main() {
	var configPath string

	cmd := &cobra.Command{
		Use:   "tolato-agent",
		Short: "ToLaTo node agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			app, err := bootstrap.NewAgentApp(configPath)
			if err != nil {
				return err
			}

			return app.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "configs/agent.example.yaml", "path to agent config")

	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
