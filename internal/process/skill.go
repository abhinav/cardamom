package process

import (
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/skill"
)

func provideSkillInstallOperation() cli.SkillInstallOperation {
	return skill.NewInstaller()
}
