package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/momaek/tolato/internal/server/app/bootstrap"
	"github.com/spf13/cobra"
)

func main() {
	var configPath string

	cmd := &cobra.Command{
		Use:   "tolato-server",
		Short: "ToLaTo control server",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			app, err := bootstrap.NewServerApp(ctx, configPath)
			if err != nil {
				return err
			}

			return app.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "configs/server.example.yaml", "path to server config")

	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
