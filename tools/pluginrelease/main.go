// Command pluginrelease synchronizes the Cardamom plugin version metadata.
//
// Run pluginrelease from the Cardamom repository root:
//
//	go run ./tools/pluginrelease check [<version>]
//	go run ./tools/pluginrelease materialize <version>
//
// A version may be prefixed with "v", as in "v1.2.3", or supplied without the
// prefix. The check operation validates plugins/cardamom/VERSION and the Claude
// and Codex manifests under plugins/cardamom. When an expected version is
// supplied, check also verifies that the repository metadata matches it. The
// materialize operation first validates all existing metadata, then writes the
// requested version to VERSION and both manifests. Manifest output uses
// deterministic JSON formatting and preserves unknown top-level members.
//
// Successful operations write one status line to standard output. A malformed
// command, invalid version, unreadable or inconsistent metadata file, failed
// write, or standard-output failure returns a non-zero status after writing a
// diagnostic to standard error. Materialize performs no writes until every
// existing metadata file has passed validation.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	commandUsage     = "usage: pluginrelease (check [<version>] | materialize <version>)"
	checkUsage       = "usage: pluginrelease check [<version>]"
	materializeUsage = "usage: pluginrelease materialize <version>"
)

func main() {
	if err := run(".", os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pluginrelease:", err)
		os.Exit(1)
	}
}

func run(root string, stdout io.Writer, args []string) error {
	if len(args) == 0 {
		return errors.New(commandUsage)
	}

	metadata := pluginMetadata{root: root}
	switch args[0] {
	case "check":
		if len(args) > 2 {
			return errors.New(checkUsage)
		}

		var expected *pluginVersion
		if len(args) == 2 {
			version, err := parseReleaseVersion(args[1])
			if err != nil {
				return err
			}
			expected = &version
		}

		version, err := metadata.check(expected)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			stdout,
			"plugin version %s is consistent\n",
			version,
		); err != nil {
			return fmt.Errorf("write check result: %w", err)
		}
		return nil

	case "materialize":
		if len(args) != 2 {
			return errors.New(materializeUsage)
		}

		version, err := parseReleaseVersion(args[1])
		if err != nil {
			return err
		}
		if err := metadata.materialize(version); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			stdout,
			"materialized plugin version %s\n",
			version,
		); err != nil {
			return fmt.Errorf("write materialize result: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unknown operation %q: %s", args[0], commandUsage)
	}
}
