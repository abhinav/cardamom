// Package configurationconnect exposes configuration operations through
// Connect.
package configurationconnect

import (
	"context"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	privatev1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
)

//go:generate go tool mockgen -destination mocks_test.go -package configurationconnect -typed -write_package_comment=false . Configurations

// Configurations supplies configuration reads and mutations.
type Configurations interface {
	// Resolve rereads every layer and resolves one board's configuration.
	Resolve(context.Context, board.ID) (configuration.View, error)

	// Update applies one typed layer patch and returns the resulting view.
	Update(
		context.Context,
		configuration.Invocation,
		configuration.UpdateRequest,
	) (configuration.View, error)
}

// Service adapts configuration operations to generated ConfigurationService
// RPCs.
type Service struct {
	privatev1connect.UnimplementedConfigurationServiceHandler
	configurations Configurations
}

var _ privatev1connect.ConfigurationServiceHandler = (*Service)(nil)

// New constructs a ConfigurationService handler.
func New(configurations Configurations) *Service {
	must.NotBeNilf(
		configurations,
		"configurationconnect: configuration operations are required",
	)
	return &Service{configurations: configurations}
}

// GetConfiguration returns ordered layers, effective values, and origins.
func (s *Service) GetConfiguration(
	ctx context.Context,
	request *connect.Request[privatev1.GetConfigurationRequest],
) (*connect.Response[privatev1.GetConfigurationResponse], error) {
	boardID, err := board.NewID(request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	view, err := s.configurations.Resolve(ctx, boardID)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetConfigurationResponse{
		View: configurationView(view),
	}), nil
}

// UpdateConfiguration applies one finite typed patch and returns its result.
func (s *Service) UpdateConfiguration(
	ctx context.Context,
	request *connect.Request[privatev1.UpdateConfigurationRequest],
) (*connect.Response[privatev1.UpdateConfigurationResponse], error) {
	boardID, err := board.NewID(request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	scope, err := updateScope(request.Msg.GetScope())
	if err != nil {
		return nil, web.FromError(err)
	}
	patch, err := configurationPatch(request.Msg)
	if err != nil {
		return nil, web.FromError(err)
	}
	view, err := s.configurations.Update(
		ctx,
		configuration.NewInvocation(request.Msg.GetContext().GetActor()),
		configuration.UpdateRequest{
			BoardID: boardID,
			Scope:   scope,
			Patch:   patch,
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.UpdateConfigurationResponse{
		View: configurationView(view),
	}), nil
}

func configurationPatch(
	request *privatev1.UpdateConfigurationRequest,
) (configuration.Patch, error) {
	patch := configuration.Patch{
		Fields: make([]configuration.Field, 0, len(request.GetUpdateMask().GetPaths())),
	}
	for _, path := range request.GetUpdateMask().GetPaths() {
		field, err := configurationField(path)
		if err != nil {
			return configuration.Patch{}, err
		}
		patch.Fields = append(patch.Fields, field)
	}

	overrides := request.GetOverrides()
	issueID := overrides.GetIssue().GetId()
	if issueID != nil && issueID.Prefix != nil {
		value, err := configuration.NewPrefix(issueID.GetPrefix())
		if err != nil {
			return configuration.Patch{}, invalidInput(err)
		}
		patch.Overrides.Issue.ID.Prefix = &value
	}
	if issueID != nil && issueID.Strategy != nil {
		value, err := issueIDStrategy(issueID.GetStrategy())
		if err != nil {
			return configuration.Patch{}, err
		}
		patch.Overrides.Issue.ID.Strategy = &value
	}
	summary := overrides.GetIssue().GetSummary()
	if summary != nil && summary.MaxBytes != nil {
		value, err := configuration.NewByteLimit(summary.GetMaxBytes())
		if err != nil {
			return configuration.Patch{}, invalidInput(err)
		}
		patch.Overrides.Issue.Summary.MaxBytes = &value
	}
	attachment := overrides.GetAttachment()
	if attachment != nil && attachment.MaxBytes != nil {
		value, err := configuration.NewByteLimit(attachment.GetMaxBytes())
		if err != nil {
			return configuration.Patch{}, invalidInput(err)
		}
		patch.Overrides.Attachment.MaxBytes = &value
	}
	pins := overrides.GetBoard().GetPins()
	if pins != nil && pins.MaxCount != nil {
		value, err := board.NewPinLimit(pins.GetMaxCount())
		if err != nil {
			return configuration.Patch{}, invalidInput(err)
		}
		patch.Overrides.Board.Pins.MaxCount = &value
	}
	if err := patch.Validate(); err != nil {
		return configuration.Patch{}, invalidInput(err)
	}
	return patch, nil
}

func configurationField(path string) (configuration.Field, error) {
	switch path {
	case "issue.id.prefix":
		return configuration.FieldIssueIDPrefix, nil
	case "issue.id.strategy":
		return configuration.FieldIssueIDStrategy, nil
	case "issue.summary.max_bytes":
		return configuration.FieldIssueSummaryMaxBytes, nil
	case "attachment.max_bytes":
		return configuration.FieldAttachmentMaxBytes, nil
	case "board.pins.max_count":
		return configuration.FieldBoardPinsMaxCount, nil
	default:
		return 0, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: unknown configuration field %q",
			path,
		)
	}
}

func updateScope(value privatev1.ConfigurationScope) (configuration.Scope, error) {
	switch value {
	case privatev1.ConfigurationScope_CONFIGURATION_SCOPE_STORE:
		return configuration.ScopeStore, nil
	case privatev1.ConfigurationScope_CONFIGURATION_SCOPE_PROJECT:
		return configuration.ScopeProject, nil
	case privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BOARD:
		return configuration.ScopeBoard, nil
	default:
		return 0, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: configuration scope %d is not mutable",
			value,
		)
	}
}

func issueIDStrategy(
	value privatev1.ConfigurationIssueIDStrategy,
) (configuration.IDStrategy, error) {
	switch value {
	case privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_RANDOM:
		return configuration.IDStrategyRandom, nil
	case privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL:
		return configuration.IDStrategySequential, nil
	default:
		return 0, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: unknown issue ID strategy %d",
			value,
		)
	}
}

func configurationView(view configuration.View) *privatev1.ConfigurationView {
	return &privatev1.ConfigurationView{
		Layers: []*privatev1.ConfigurationLayer{
			{
				Source: configurationSource(configuration.Source{
					Scope: configuration.ScopeBuiltIn, Identity: "built-in",
				}),
				Overrides: configurationOverridesFromValues(view.BuiltIn),
			},
			configurationLayer(view.Store),
			configurationLayer(view.Project),
			configurationLayer(view.Board),
		},
		Effective: configurationValues(view.Effective),
		Origins:   configurationOrigins(view.Origins),
	}
}

func configurationLayer(layer configuration.Layer) *privatev1.ConfigurationLayer {
	return &privatev1.ConfigurationLayer{
		Source:    configurationSource(layer.Source),
		Overrides: configurationOverrides(layer.Overrides),
	}
}

func configurationValues(
	value configuration.Configuration,
) *privatev1.Configuration {
	return &privatev1.Configuration{
		Issue: &privatev1.ConfigurationIssue{
			Id: &privatev1.ConfigurationIssueID{
				Prefix:   value.Issue.ID.Prefix.String(),
				Strategy: configurationIssueIDStrategy(value.Issue.ID.Strategy),
			},
			Summary: &privatev1.ConfigurationSummary{
				MaxBytes: value.Issue.Summary.MaxBytes.Uint64(),
			},
		},
		Attachment: &privatev1.ConfigurationAttachment{
			MaxBytes: value.Attachment.MaxBytes.Uint64(),
		},
		Board: &privatev1.ConfigurationBoard{
			Pins: &privatev1.ConfigurationPins{
				MaxCount: value.Board.Pins.MaxCount.Uint64(),
			},
		},
	}
}

func configurationOverridesFromValues(
	value configuration.Configuration,
) *privatev1.ConfigurationOverrides {
	prefix := value.Issue.ID.Prefix.String()
	strategy := configurationIssueIDStrategy(value.Issue.ID.Strategy)
	summaryMaxBytes := value.Issue.Summary.MaxBytes.Uint64()
	attachmentMaxBytes := value.Attachment.MaxBytes.Uint64()
	pinMaxCount := value.Board.Pins.MaxCount.Uint64()
	return &privatev1.ConfigurationOverrides{
		Issue: &privatev1.ConfigurationIssueOverrides{
			Id: &privatev1.ConfigurationIssueIDOverrides{
				Prefix: &prefix, Strategy: &strategy,
			},
			Summary: &privatev1.ConfigurationSummaryOverrides{
				MaxBytes: &summaryMaxBytes,
			},
		},
		Attachment: &privatev1.ConfigurationAttachmentOverrides{
			MaxBytes: &attachmentMaxBytes,
		},
		Board: &privatev1.ConfigurationBoardOverrides{
			Pins: &privatev1.ConfigurationPinsOverrides{MaxCount: &pinMaxCount},
		},
	}
}

func configurationOverrides(
	value configuration.Overrides,
) *privatev1.ConfigurationOverrides {
	result := &privatev1.ConfigurationOverrides{
		Issue: &privatev1.ConfigurationIssueOverrides{
			Id:      &privatev1.ConfigurationIssueIDOverrides{},
			Summary: &privatev1.ConfigurationSummaryOverrides{},
		},
		Attachment: &privatev1.ConfigurationAttachmentOverrides{},
		Board: &privatev1.ConfigurationBoardOverrides{
			Pins: &privatev1.ConfigurationPinsOverrides{},
		},
	}
	if value.Issue.ID.Prefix != nil {
		result.Issue.Id.Prefix = new(value.Issue.ID.Prefix.String())
	}
	if value.Issue.ID.Strategy != nil {
		result.Issue.Id.Strategy = new(
			configurationIssueIDStrategy(*value.Issue.ID.Strategy),
		)
	}
	if value.Issue.Summary.MaxBytes != nil {
		result.Issue.Summary.MaxBytes = new(
			value.Issue.Summary.MaxBytes.Uint64(),
		)
	}
	if value.Attachment.MaxBytes != nil {
		result.Attachment.MaxBytes = new(value.Attachment.MaxBytes.Uint64())
	}
	if value.Board.Pins.MaxCount != nil {
		result.Board.Pins.MaxCount = new(value.Board.Pins.MaxCount.Uint64())
	}
	return result
}

func configurationOrigins(
	value configuration.Origins,
) *privatev1.ConfigurationOrigins {
	return &privatev1.ConfigurationOrigins{
		Issue: &privatev1.ConfigurationIssueOrigins{
			Id: &privatev1.ConfigurationIssueIDOrigins{
				Prefix:   configurationSource(value.Issue.ID.Prefix),
				Strategy: configurationSource(value.Issue.ID.Strategy),
			},
			Summary: &privatev1.ConfigurationSummaryOrigins{
				MaxBytes: configurationSource(value.Issue.Summary.MaxBytes),
			},
		},
		Attachment: &privatev1.ConfigurationAttachmentOrigins{
			MaxBytes: configurationSource(value.Attachment.MaxBytes),
		},
		Board: &privatev1.ConfigurationBoardOrigins{
			Pins: &privatev1.ConfigurationPinsOrigins{
				MaxCount: configurationSource(value.Board.Pins.MaxCount),
			},
		},
	}
}

func configurationSource(
	value configuration.Source,
) *privatev1.ConfigurationSource {
	return &privatev1.ConfigurationSource{
		Scope: configurationScope(value.Scope), Identity: value.Identity,
	}
}

func configurationScope(value configuration.Scope) privatev1.ConfigurationScope {
	switch value {
	case configuration.ScopeBuiltIn:
		return privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BUILT_IN
	case configuration.ScopeStore:
		return privatev1.ConfigurationScope_CONFIGURATION_SCOPE_STORE
	case configuration.ScopeProject:
		return privatev1.ConfigurationScope_CONFIGURATION_SCOPE_PROJECT
	case configuration.ScopeBoard:
		return privatev1.ConfigurationScope_CONFIGURATION_SCOPE_BOARD
	default:
		return privatev1.ConfigurationScope_CONFIGURATION_SCOPE_UNSPECIFIED
	}
}

func configurationIssueIDStrategy(
	value configuration.IDStrategy,
) privatev1.ConfigurationIssueIDStrategy {
	switch value {
	case configuration.IDStrategyRandom:
		return privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_RANDOM
	case configuration.IDStrategySequential:
		return privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_SEQUENTIAL
	default:
		return privatev1.ConfigurationIssueIDStrategy_CONFIGURATION_ISSUE_ID_STRATEGY_UNSPECIFIED
	}
}

func invalidInput(err error) error {
	return errkind.Errorf(errkind.InvalidInput, "invalid input: %w", err)
}
