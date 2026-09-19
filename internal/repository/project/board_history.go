package project

import (
	"context"
	"reflect"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func appendBoardHistory(
	ctx context.Context,
	change *store.Change,
	boardID board.ID,
	events ...boardhistory.Event,
) error {
	return repositoryhistory.AppendV1(
		ctx,
		change,
		boardID,
		func(yield func(boardhistory.Event, error) bool) {
			for _, event := range events {
				if !yield(event, nil) {
					return
				}
			}
		},
	)
}

func boardSettingsEvents(
	at time.Time,
	current *board.State,
	next *board.State,
) []boardhistory.Event {
	events := make([]boardhistory.Event, 0, 2)
	if current.Name() != next.Name() {
		events = append(events, boardhistory.Event{
			OccurredAt: at,
			Payload:    &boardhistory.BoardNameSet{Name: next.Name()},
		})
	}
	if !reflect.DeepEqual(current.Description(), next.Description()) {
		events = append(events, boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.BoardDescriptionSet{
				Description: next.Description(),
			},
		})
	}
	return events
}

func boardConfigurationEvents(
	at time.Time,
	current configuration.Overrides,
	next configuration.Overrides,
) []boardhistory.Event {
	before := nullableConfigurationFromOverrides(current)
	after := nullableConfigurationFromOverrides(next)
	events := make([]boardhistory.Event, 0, 5)
	if !reflect.DeepEqual(before.prefix, after.prefix) {
		events = append(events, boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.BoardIssueIDPrefixSet{
				Prefix: after.prefix,
			},
		})
	}
	if !reflect.DeepEqual(before.strategy, after.strategy) {
		events = append(events, boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.BoardIssueIDStrategySet{
				Strategy: after.strategy,
			},
		})
	}
	if !reflect.DeepEqual(before.summaryMaxBytes, after.summaryMaxBytes) {
		events = append(events, boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.BoardSummaryLimitSet{
				Limit: after.summaryMaxBytes,
			},
		})
	}
	if !reflect.DeepEqual(before.attachmentMaxBytes, after.attachmentMaxBytes) {
		events = append(events, boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.BoardAttachmentLimitSet{
				Limit: after.attachmentMaxBytes,
			},
		})
	}
	if !reflect.DeepEqual(before.pinMaxCount, after.pinMaxCount) {
		events = append(events, boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.BoardPinLimitSet{
				Limit: after.pinMaxCount,
			},
		})
	}
	return events
}
