package process

import (
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/storelocation"
)

type hookContextOperation struct {
	cwd string
}

func provideHookContextOperation(config *Config) cli.HookContextOperation {
	return &hookContextOperation{cwd: config.CWD}
}

func (o *hookContextOperation) Associated() bool {
	associated, err := storelocation.HasCheckoutAssociation(o.cwd)
	return err == nil && associated
}
