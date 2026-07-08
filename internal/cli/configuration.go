package cli

import (
	"fmt"
	"strconv"
	"text/tabwriter"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
)

type configurationCommand struct {
	Show  configurationShowCommand  `cmd:"" help:"Show effective or scoped configuration."`
	Set   configurationSetCommand   `cmd:"" help:"Set one typed configuration value."`
	Unset configurationUnsetCommand `cmd:"" help:"Restore inheritance for one configuration value."`
}

// Help explains configuration selection and mutation scope.
func (*configurationCommand) Help() string {
	return `Inspect and update typed configuration for the selected board.

Root --store and --board flags select the physical store and board context.
The selected board determines project scope. Mutations require --scope.`
}

type configurationShowCommand struct {
	Scope string `name:"scope" default:"" enum:"store,project,board," placeholder:"SCOPE" help:"Show the raw store, project, or board layer instead of effective values."`
}

// Help distinguishes the effective configuration from one raw layer.
func (*configurationShowCommand) Help() string {
	return `Show the selected board's effective configuration by default.

Use --scope store, project, or board to show only values explicitly stored at
that layer. Fields without a layer value remain unset.`
}

// Run resolves the selected board configuration and renders one view.
func (c *configurationShowCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *configuration.Service,
) error {
	view, err := service.Resolve(invocation.Context, selected.ID())
	if err != nil {
		return err
	}
	if c.Scope == "" {
		return renderConfiguration(invocation.Output, newEffectiveConfigurationOut(view))
	}
	scope, err := parseConfigurationScope(c.Scope)
	if err != nil {
		return UsageErrorf("config show: %v", err)
	}
	return renderConfiguration(invocation.Output, newLayerConfigurationOut(view, scope))
}

type configurationSetCommand struct {
	Scope string `name:"scope" required:"" enum:"store,project,board" placeholder:"SCOPE" help:"Layer to update: store, project, or board."`
	Key   string `arg:"" name:"key" enum:"issue.id.prefix,issue.id.strategy,issue.summary.max_bytes,attachment.max_bytes" help:"Typed configuration key."`
	Value string `arg:"" name:"value" help:"Replacement value."`
}

// Help lists the finite value syntax accepted by config set.
func (*configurationSetCommand) Help() string {
	return `Set one value at the required store, project, or board scope.

Keys:
  issue.id.prefix
  issue.id.strategy
  issue.summary.max_bytes
  attachment.max_bytes

Issue ID prefixes start with a lowercase letter or digit, contain lowercase
letters, digits, or dashes, end in a dash, and are at most 16 characters.

Byte limits accept a positive decimal byte count.`
}

// Run parses one typed replacement and renders the resulting effective view.
func (c *configurationSetCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *configuration.Service,
) error {
	scope, err := parseConfigurationScope(c.Scope)
	if err != nil {
		return UsageErrorf("config set: %v", err)
	}
	patch, err := newConfigurationSetPatch(c.Key, c.Value)
	if err != nil {
		return UsageErrorf("config set: %v", err)
	}
	view, err := service.Update(
		invocation.Context,
		configuration.NewInvocation(invocation.Actor),
		configuration.UpdateRequest{
			BoardID: selected.ID(), Scope: scope, Patch: patch,
		},
	)
	if err != nil {
		return err
	}
	return renderConfiguration(invocation.Output, newEffectiveConfigurationOut(view))
}

type configurationUnsetCommand struct {
	Scope string `name:"scope" required:"" enum:"store,project,board" placeholder:"SCOPE" help:"Layer to update: store, project, or board."`
	Key   string `arg:"" name:"key" enum:"issue.id.prefix,issue.id.strategy,issue.summary.max_bytes,attachment.max_bytes" help:"Typed configuration key."`
}

// Help explains inheritance restoration and lists the finite key vocabulary.
func (*configurationUnsetCommand) Help() string {
	return `Clear one value at the required store, project, or board scope.
The field then inherits from less specific layers.

Keys:
  issue.id.prefix
  issue.id.strategy
  issue.summary.max_bytes
  attachment.max_bytes`
}

// Run clears one typed override and renders the resulting effective view.
func (c *configurationUnsetCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *configuration.Service,
) error {
	scope, err := parseConfigurationScope(c.Scope)
	if err != nil {
		return UsageErrorf("config unset: %v", err)
	}
	field, err := parseConfigurationField(c.Key)
	if err != nil {
		return UsageErrorf("config unset: %v", err)
	}
	view, err := service.Update(
		invocation.Context,
		configuration.NewInvocation(invocation.Actor),
		configuration.UpdateRequest{
			BoardID: selected.ID(),
			Scope:   scope,
			Patch:   configuration.Patch{Fields: []configuration.Field{field}},
		},
	)
	if err != nil {
		return err
	}
	return renderConfiguration(invocation.Output, newEffectiveConfigurationOut(view))
}

func parseConfigurationScope(value string) (configuration.Scope, error) {
	switch value {
	case "store":
		return configuration.ScopeStore, nil
	case "project":
		return configuration.ScopeProject, nil
	case "board":
		return configuration.ScopeBoard, nil
	default:
		return 0, fmt.Errorf("unsupported configuration scope %q", value)
	}
}

func parseConfigurationField(value string) (configuration.Field, error) {
	switch value {
	case "issue.id.prefix":
		return configuration.FieldIssueIDPrefix, nil
	case "issue.id.strategy":
		return configuration.FieldIssueIDStrategy, nil
	case "issue.summary.max_bytes":
		return configuration.FieldIssueSummaryMaxBytes, nil
	case "attachment.max_bytes":
		return configuration.FieldAttachmentMaxBytes, nil
	default:
		return 0, fmt.Errorf("unsupported configuration key %q", value)
	}
}

func newConfigurationSetPatch(key, value string) (configuration.Patch, error) {
	field, err := parseConfigurationField(key)
	if err != nil {
		return configuration.Patch{}, err
	}
	patch := configuration.Patch{Fields: []configuration.Field{field}}
	switch field {
	case configuration.FieldIssueIDPrefix:
		parsed, err := configuration.NewPrefix(value)
		if err != nil {
			return configuration.Patch{}, err
		}
		patch.Overrides.Issue.ID.Prefix = &parsed
	case configuration.FieldIssueIDStrategy:
		parsed, err := configuration.NewIDStrategy(value)
		if err != nil {
			return configuration.Patch{}, err
		}
		patch.Overrides.Issue.ID.Strategy = &parsed
	case configuration.FieldIssueSummaryMaxBytes:
		parsed, err := parseConfigurationByteLimit(value)
		if err != nil {
			return configuration.Patch{}, err
		}
		patch.Overrides.Issue.Summary.MaxBytes = &parsed
	case configuration.FieldAttachmentMaxBytes:
		parsed, err := parseConfigurationByteLimit(value)
		if err != nil {
			return configuration.Patch{}, err
		}
		patch.Overrides.Attachment.MaxBytes = &parsed
	}
	return patch, nil
}

func parseConfigurationByteLimit(value string) (configuration.ByteLimit, error) {
	bytes, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("byte limit %q: positive decimal byte count required", value)
	}
	return configuration.NewByteLimit(bytes)
}

// configurationOut is the CLI's typed JSON and human configuration view.
// Raw-layer values and origins are nil when that layer inherits the field.
type configurationOut struct {
	Scope   string                  `json:"scope"`
	Store   string                  `json:"store"`
	Project string                  `json:"project"`
	Board   string                  `json:"board"`
	Values  configurationValuesOut  `json:"values"`
	Origins configurationOriginsOut `json:"origins"`
}

// configurationValuesOut mirrors the nested typed configuration shape with
// optional fields so the same representation can render effective and raw views.
type configurationValuesOut struct {
	Issue      configurationIssueValuesOut      `json:"issue"`
	Attachment configurationAttachmentValuesOut `json:"attachment"`
}

type configurationIssueValuesOut struct {
	ID      configurationIssueIDValuesOut `json:"id"`
	Summary configurationSummaryValuesOut `json:"summary"`
}

type configurationIssueIDValuesOut struct {
	Prefix   *string `json:"prefix"`
	Strategy *string `json:"strategy"`
}

type configurationSummaryValuesOut struct {
	MaxBytes *uint64 `json:"max_bytes"`
}

type configurationAttachmentValuesOut struct {
	MaxBytes *uint64 `json:"max_bytes"`
}

// configurationOriginsOut mirrors configurationValuesOut with one source for
// each value that the rendered view supplies explicitly.
type configurationOriginsOut struct {
	Issue      configurationIssueOriginsOut      `json:"issue"`
	Attachment configurationAttachmentOriginsOut `json:"attachment"`
}

type configurationIssueOriginsOut struct {
	ID      configurationIssueIDOriginsOut `json:"id"`
	Summary configurationSummaryOriginsOut `json:"summary"`
}

type configurationIssueIDOriginsOut struct {
	Prefix   *configurationSourceOut `json:"prefix"`
	Strategy *configurationSourceOut `json:"strategy"`
}

type configurationSummaryOriginsOut struct {
	MaxBytes *configurationSourceOut `json:"max_bytes"`
}

type configurationAttachmentOriginsOut struct {
	MaxBytes *configurationSourceOut `json:"max_bytes"`
}

// configurationSourceOut identifies the layer and concrete namespace that
// supplies one rendered value.
type configurationSourceOut struct {
	Scope    string `json:"scope"`
	Identity string `json:"identity"`
}

func newEffectiveConfigurationOut(view configuration.View) configurationOut {
	return configurationOut{
		Scope:   "effective",
		Store:   view.Store.Source.Identity,
		Project: view.Project.Source.Identity,
		Board:   view.Board.Source.Identity,
		Values: configurationValuesOut{
			Issue: configurationIssueValuesOut{
				ID: configurationIssueIDValuesOut{
					Prefix:   new(view.Effective.Issue.ID.Prefix.String()),
					Strategy: new(view.Effective.Issue.ID.Strategy.String()),
				},
				Summary: configurationSummaryValuesOut{
					MaxBytes: new(view.Effective.Issue.Summary.MaxBytes.Uint64()),
				},
			},
			Attachment: configurationAttachmentValuesOut{
				MaxBytes: new(view.Effective.Attachment.MaxBytes.Uint64()),
			},
		},
		Origins: newEffectiveConfigurationOriginsOut(view.Origins),
	}
}

func newEffectiveConfigurationOriginsOut(
	origins configuration.Origins,
) configurationOriginsOut {
	return configurationOriginsOut{
		Issue: configurationIssueOriginsOut{
			ID: configurationIssueIDOriginsOut{
				Prefix:   newConfigurationSourceOut(origins.Issue.ID.Prefix),
				Strategy: newConfigurationSourceOut(origins.Issue.ID.Strategy),
			},
			Summary: configurationSummaryOriginsOut{
				MaxBytes: newConfigurationSourceOut(origins.Issue.Summary.MaxBytes),
			},
		},
		Attachment: configurationAttachmentOriginsOut{
			MaxBytes: newConfigurationSourceOut(origins.Attachment.MaxBytes),
		},
	}
}

func newLayerConfigurationOut(
	view configuration.View,
	scope configuration.Scope,
) configurationOut {
	var layer configuration.Layer
	switch scope {
	case configuration.ScopeStore:
		layer = view.Store
	case configuration.ScopeProject:
		layer = view.Project
	case configuration.ScopeBoard:
		layer = view.Board
	}
	return configurationOut{
		Scope:   scope.String(),
		Store:   view.Store.Source.Identity,
		Project: view.Project.Source.Identity,
		Board:   view.Board.Source.Identity,
		Values:  newLayerConfigurationValuesOut(layer.Overrides),
		Origins: newLayerConfigurationOriginsOut(layer),
	}
}

func newLayerConfigurationValuesOut(
	overrides configuration.Overrides,
) configurationValuesOut {
	var values configurationValuesOut
	if value := overrides.Issue.ID.Prefix; value != nil {
		values.Issue.ID.Prefix = new(value.String())
	}
	if value := overrides.Issue.ID.Strategy; value != nil {
		values.Issue.ID.Strategy = new(value.String())
	}
	if value := overrides.Issue.Summary.MaxBytes; value != nil {
		values.Issue.Summary.MaxBytes = new(value.Uint64())
	}
	if value := overrides.Attachment.MaxBytes; value != nil {
		values.Attachment.MaxBytes = new(value.Uint64())
	}
	return values
}

func newLayerConfigurationOriginsOut(
	layer configuration.Layer,
) configurationOriginsOut {
	var origins configurationOriginsOut
	if layer.Overrides.Issue.ID.Prefix != nil {
		origins.Issue.ID.Prefix = newConfigurationSourceOut(layer.Source)
	}
	if layer.Overrides.Issue.ID.Strategy != nil {
		origins.Issue.ID.Strategy = newConfigurationSourceOut(layer.Source)
	}
	if layer.Overrides.Issue.Summary.MaxBytes != nil {
		origins.Issue.Summary.MaxBytes = newConfigurationSourceOut(layer.Source)
	}
	if layer.Overrides.Attachment.MaxBytes != nil {
		origins.Attachment.MaxBytes = newConfigurationSourceOut(layer.Source)
	}
	return origins
}

func newConfigurationSourceOut(source configuration.Source) *configurationSourceOut {
	return &configurationSourceOut{
		Scope: source.Scope.String(), Identity: source.Identity,
	}
}

func renderConfiguration(output *Output, value configurationOut) error {
	if output.JSON() {
		return output.WriteJSON(value)
	}
	writer := tabwriter.NewWriter(output.Stdout(), 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(
		writer,
		"Scope:\t%s\nStore:\t%s\nProject:\t%s\nBoard:\t%s\n\n",
		value.Scope, value.Store, value.Project, value.Board,
	)
	_, _ = fmt.Fprintln(writer, "KEY\tVALUE\tORIGIN")
	writeConfigurationRow(
		writer, "issue.id.prefix",
		formatConfigurationValue(value.Values.Issue.ID.Prefix),
		value.Origins.Issue.ID.Prefix,
	)
	writeConfigurationRow(
		writer, "issue.id.strategy",
		formatConfigurationValue(value.Values.Issue.ID.Strategy),
		value.Origins.Issue.ID.Strategy,
	)
	writeConfigurationRow(
		writer, "issue.summary.max_bytes",
		formatConfigurationValue(value.Values.Issue.Summary.MaxBytes),
		value.Origins.Issue.Summary.MaxBytes,
	)
	writeConfigurationRow(
		writer, "attachment.max_bytes",
		formatConfigurationValue(value.Values.Attachment.MaxBytes),
		value.Origins.Attachment.MaxBytes,
	)
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("write configuration output: %w", err)
	}
	return nil
}

func writeConfigurationRow(
	writer *tabwriter.Writer,
	key string,
	value string,
	origin *configurationSourceOut,
) {
	_, _ = fmt.Fprintf(
		writer,
		"%s\t%s\t%s\n",
		key, value, formatConfigurationOrigin(origin),
	)
}

func formatConfigurationValue[T any](value *T) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprint(*value)
}

func formatConfigurationOrigin(origin *configurationSourceOut) string {
	if origin == nil {
		return "-"
	}
	return fmt.Sprintf("%s (%s)", origin.Scope, origin.Identity)
}
