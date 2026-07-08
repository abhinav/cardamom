//go:build !webdev

package cli

type webDevelopmentOptions struct {
	Development bool   `kong:"-"`
	WebDir      string `kong:"-"`
}
