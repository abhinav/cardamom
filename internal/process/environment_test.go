package process

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBuildVCS(t *testing.T) {
	const revision = "946440c7b34322afb39e1693431aaf1d41f3f5af"
	tests := []struct {
		name         string
		giveSettings []debug.BuildSetting
		wantRevision string
		wantModified bool
	}{
		{
			name: "Clean",
			giveSettings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: "false"},
			},
			wantRevision: revision,
		},
		{
			name: "Dirty",
			giveSettings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: "true"},
			},
			wantRevision: revision,
			wantModified: true,
		},
		{
			name: "WithoutRevision",
			giveSettings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "true"},
			},
		},
		{name: "WithoutVCSSettings"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			revision, modified := parseBuildVCS(&debug.BuildInfo{
				Settings: tt.giveSettings,
			})

			assert.Equal(t, tt.wantRevision, revision)
			assert.Equal(t, tt.wantModified, modified)
		})
	}
}
