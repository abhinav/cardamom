package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/alecthomas/kong"
)

// WebSource identifies one configured aggregate web source.
type WebSource struct {
	// Alias is the source name used by aggregate routing.
	Alias string // required

	// URL is the absolute HTTP(S) endpoint of the source.
	URL *url.URL // required
}

// Decode implements kong.MapperValue for repeated --source values.
func (s *WebSource) Decode(ctx *kong.DecodeContext) error {
	token := ctx.Scan.Pop()
	if token.IsEOL() {
		return errors.New(`missing value, expecting "<alias=url>"`)
	}
	if token.InferredType() == kong.FlagToken {
		return fmt.Errorf(
			"expected source value but got %q (%s)",
			token.String(), token.InferredType(),
		)
	}
	value, ok := token.Value.(string)
	if !ok {
		return fmt.Errorf("expected source value but got %q", token.Value)
	}

	source, err := parseWebSource(value)
	if err != nil {
		return err
	}
	*s = source
	return nil
}

func parseWebSource(value string) (WebSource, error) {
	alias, rawURL, ok := strings.Cut(value, "=")
	if !ok {
		return WebSource{}, fmt.Errorf("source %q must use alias=url", value)
	}
	if alias == "" || strings.IndexFunc(alias, unicode.IsSpace) >= 0 {
		return WebSource{}, fmt.Errorf("source alias %q must be non-empty and contain no whitespace", alias)
	}

	endpoint, err := url.Parse(rawURL)
	if err != nil {
		return WebSource{}, fmt.Errorf("source URL %q is invalid: %w", rawURL, err)
	}
	if !strings.EqualFold(endpoint.Scheme, "http") &&
		!strings.EqualFold(endpoint.Scheme, "https") {
		return WebSource{}, fmt.Errorf("source URL %q must use http or https", rawURL)
	}
	if endpoint.Host == "" {
		return WebSource{}, fmt.Errorf("source URL %q must be absolute", rawURL)
	}

	return WebSource{Alias: alias, URL: endpoint}, nil
}
