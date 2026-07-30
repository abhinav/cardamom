package cli

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/skill"
)

type skillCommand struct {
	Install skillInstallCommand `cmd:"" help:"Install the embedded Cardamom skill."`
}

type skillInstallCommand struct {
	SkillsDirectory string `arg:"" name:"skills-directory" type:"path" help:"Parent skills directory that will contain cardamom."`
	Force           bool   `name:"force" help:"Replace a different existing cardamom skill."`
}

// Help describes destination ownership and replacement behavior.
func (*skillInstallCommand) Help() string {
	return `Install the runtime Cardamom skill beneath the supplied skills
directory. An identical cardamom directory is left unchanged. A different
cardamom directory is preserved unless --force is supplied.`
}

// SkillInstallOperation installs the embedded runtime skill without selecting
// a Cardamom namespace.
type SkillInstallOperation interface {
	Install(context.Context, skill.InstallRequest) (skill.InstallResult, error)
}

// Run translates command syntax into one skill installation.
func (c *skillInstallCommand) Run(
	invocation *Invocation,
	operation SkillInstallOperation,
) error {
	existing := skill.PreserveExisting
	if c.Force {
		existing = skill.ReplaceExisting
	}
	result, err := operation.Install(invocation.Context, skill.InstallRequest{
		SkillsDirectory: c.SkillsDirectory,
		Existing:        existing,
	})
	if err != nil {
		return err
	}
	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(result)
	}
	switch result.Status {
	case skill.StatusInstalled:
		return invocation.Output.Noticef(
			"installed Cardamom skill at %s",
			result.Destination,
		)
	case skill.StatusUnchanged:
		return invocation.Output.Noticef(
			"Cardamom skill unchanged at %s",
			result.Destination,
		)
	case skill.StatusReplaced:
		return invocation.Output.Noticef(
			"replaced Cardamom skill at %s",
			result.Destination,
		)
	default:
		return errors.New("skill installation returned an unknown status")
	}
}
