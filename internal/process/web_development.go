//go:build webdev

package process

import (
	"context"

	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/web/server"
)

func runWebServer(ctx context.Context, request cli.WebRequest, cfg server.Config) error {
	if !request.Development {
		return server.Run(ctx, cfg)
	}
	return server.RunDevelopment(ctx, server.DevelopmentConfig{
		Config: cfg, WebDir: request.WebDir,
		Stdout: request.Notice, Stderr: request.Diagnostic,
	})
}
