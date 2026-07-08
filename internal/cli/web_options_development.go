//go:build webdev

package cli

type webDevelopmentOptions struct {
	Development bool   `name:"dev" help:"Run the live frontend development server."`
	WebDir      string `name:"web-dir" placeholder:"PATH" help:"Frontend source directory for --dev."`
}
