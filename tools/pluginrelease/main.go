// Command pluginrelease synchronizes release versions in the Cardamom plugin.
//
// Run it from the repository root:
//
//	pluginrelease <version>
//	pluginrelease -check <version>
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/mod/semver"
)

func main() {
	os.Exit(run(".", os.Stderr, os.Args[1:]))
}

func run(root string, stderr io.Writer, args []string) int {
	log := log.New(stderr, "pluginrelease: ", 0)
	flags := flag.NewFlagSet("pluginrelease", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	check := flags.Bool("check", false, "check without writing")
	if err := flags.Parse(args); err != nil {
		log.Print(err)
		return 2
	}
	if flags.NArg() != 1 {
		log.Print("usage: pluginrelease [-check] <version>")
		return 2
	}

	version := flags.Arg(0)
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if !semver.IsValid(version) {
		log.Printf("release version %q is not valid SemVer", flags.Arg(0))
		return 2
	}
	version = strings.TrimPrefix(version, "v")

	if err := synchronize(root, log, version, *check); err != nil {
		log.Print(err)
		return 1
	}
	return 0
}

func synchronize(
	root string,
	logger *log.Logger,
	version string,
	check bool,
) error {
	files := []struct {
		path   string
		render func([]byte, string) ([]byte, error)
	}{
		{"plugins/cardamom/.claude-plugin/plugin.json", renderJSON},
		{"plugins/cardamom/.codex-plugin/plugin.json", renderJSON},
		{"plugins/cardamom/skills/cardamom/scripts/cardamom", renderBash},
		{
			"plugins/cardamom/skills/cardamom/scripts/cardamom.ps1",
			renderPowerShell,
		},
	}

	changed := 0
	for _, file := range files {
		path := filepath.Join(root, file.path)
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", file.path, err)
		}
		rendered, err := file.render(body, version)
		if err != nil {
			return fmt.Errorf("update %q: %w", file.path, err)
		}
		if bytes.Equal(body, rendered) {
			continue
		}
		if check {
			logger.Printf("would update: %s", file.path)
			changed++
			continue
		}
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			return fmt.Errorf("write %q: %w", file.path, err)
		}
	}

	if check && changed > 0 {
		return fmt.Errorf("%d files would be changed", changed)
	}
	return nil
}

func renderJSON(body []byte, version string) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, errors.New("invalid JSON")
	}
	current := gjson.GetBytes(body, "version")
	if !current.Exists() {
		return nil, errors.New("version field is missing")
	}
	if current.Type != gjson.String {
		return nil, errors.New("version field must be a string")
	}

	rendered, err := sjson.SetBytes(body, "version", version)
	if err != nil {
		return nil, fmt.Errorf("set version field: %w", err)
	}
	return rendered, nil
}

var bashVersionPattern = regexp.MustCompile(
	`(?m)^readonly VERSION='[^']*'$`,
)

func renderBash(body []byte, version string) ([]byte, error) {
	if len(bashVersionPattern.FindAllIndex(body, 2)) != 1 {
		return nil, errors.New("expected one version constant")
	}
	return bashVersionPattern.ReplaceAllLiteral(
		body,
		[]byte("readonly VERSION='"+version+"'"),
	), nil
}

var powerShellVersionPattern = regexp.MustCompile(
	`(?m)^\$Version = "[^"]*"$`,
)

func renderPowerShell(body []byte, version string) ([]byte, error) {
	if len(powerShellVersionPattern.FindAllIndex(body, 2)) != 1 {
		return nil, errors.New("expected one version constant")
	}
	return powerShellVersionPattern.ReplaceAllLiteral(
		body,
		[]byte(`$Version = "`+version+`"`),
	), nil
}
