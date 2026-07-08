//go:build webassets && !webdev

package process

import (
	"context"

	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/web/server"
)

func runWebServer(ctx context.Context, _ cli.WebRequest, cfg server.Config) error {
	return server.Run(ctx, cfg)
}
