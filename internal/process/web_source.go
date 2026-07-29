//go:build !webassets && !webdev

package process

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/web/server"
)

func runWebServer(context.Context, cli.WebRequest, server.Config) error {
	return errors.New("embedded web assets: web assets not found; run an asset-bearing build")
}
