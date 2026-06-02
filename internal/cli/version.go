package cli

import (
	"fmt"
	"runtime/debug"
)

// Version is set via -ldflags "-X github.com/Rovak/agents-clu/internal/cli.Version=…"
// at release time. Falls back to module/VCS info from runtime/debug when empty.
var Version = ""

type VersionCmd struct{}

type versionInfo struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Dirty    bool   `json:"dirty,omitempty"`
	Time     string `json:"time,omitempty"`
	GoVer    string `json:"go,omitempty"`
}

func (c *VersionCmd) Run(r *runCtx) error {
	v := versionInfo{Version: Version}
	if v.Version == "" {
		v.Version = "dev"
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		v.GoVer = info.GoVersion
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				v.Revision = s.Value
				if len(v.Revision) > 12 {
					v.Revision = v.Revision[:12]
				}
			case "vcs.modified":
				v.Dirty = s.Value == "true"
			case "vcs.time":
				v.Time = s.Value
			}
		}
	}
	if r.json {
		return r.emitJSON(v)
	}
	fmt.Fprintf(r.stdout, "clu %s", v.Version)
	if v.Revision != "" {
		fmt.Fprintf(r.stdout, " (%s", v.Revision)
		if v.Dirty {
			fmt.Fprint(r.stdout, "-dirty")
		}
		fmt.Fprint(r.stdout, ")")
	}
	if v.GoVer != "" {
		fmt.Fprintf(r.stdout, " %s", v.GoVer)
	}
	fmt.Fprintln(r.stdout)
	return nil
}
