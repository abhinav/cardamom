package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// WebSource identifies one configured aggregate web source.
type WebSource struct {
	// Alias is the source name used by aggregate routing.
	Alias string // required

	// URL is the absolute HTTP(S) endpoint of the source.
	URL *url.URL // required
}

// UnmarshalText parses the command-line alias=url representation.
func (s *WebSource) UnmarshalText(text []byte) error {
	alias, rawURL, ok := strings.Cut(string(text), "=")
	if !ok {
		return fmt.Errorf("source %q must use alias=url", string(text))
	}
	if alias == "" {
		return errors.New("source alias is required")
	}

	endpoint, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return fmt.Errorf("source URL %q must use http or https", rawURL)
	}
	if endpoint.Host == "" {
		return fmt.Errorf("source URL %q must be absolute", rawURL)
	}
	*s = WebSource{Alias: alias, URL: endpoint}
	return nil
}
