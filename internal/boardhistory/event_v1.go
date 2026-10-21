package boardhistory

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"
)

// eventV1 is the strict retained representation decoded after V1 dispatch.
type eventV1 struct {
	Version    uint8          `json:"version"`
	Kind       EventKind      `json:"kind"`
	OccurredAt time.Time      `json:"occurred_at"`
	Value      jsontext.Value `json:"value"`
}

// eventWriteV1 holds the semantic payload's private V1 DTO while streaming.
type eventWriteV1 struct {
	Kind       EventKind `json:"kind"`
	OccurredAt time.Time `json:"occurred_at"`
	Value      any       `json:"value"`
	Version    uint8     `json:"version"`
}

func encodeEvent(event Event) ([]byte, error) {
	var encoded bytes.Buffer
	if err := writeEvent(&encoded, event); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func writeEvent(out io.Writer, event Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}
	// The private wire value may borrow payload fields; it lives only for this
	// synchronous write, during which Event requires the caller to keep them stable.
	return writeEventV1(out, eventWriteV1{
		Kind: event.Payload.kind(), OccurredAt: event.OccurredAt.UTC(),
		Value: event.Payload.valueV1(), Version: 1,
	})
}

func decodeEvent(encoded []byte) (Event, error) {
	version, err := decodeVersion(encoded, "mutation event")
	if err != nil {
		return Event{}, err
	}
	if version != 1 {
		return Event{}, fmt.Errorf("unsupported event version %d", version)
	}
	var wire eventV1
	if err := json.Unmarshal(encoded, &wire, json.RejectUnknownMembers(true)); err != nil {
		return Event{}, err
	}
	payload, err := decodePayloadV1(wire.Kind, wire.Value)
	if err != nil {
		return Event{}, err
	}
	event := Event{OccurredAt: wire.OccurredAt, Payload: payload}
	canonical, err := encodeEvent(event)
	if err != nil {
		return Event{}, err
	}
	if !bytes.Equal(canonical, encoded) {
		return Event{}, errors.New("mutation event is not canonical")
	}
	return event, nil
}

// decodePayloadV1 is the sole dispatch for the closed V1 event vocabulary.
// Each event owns its retained record and conversion beside its payload type.
func decodePayloadV1(kind EventKind, encoded []byte) (Payload, error) {
	switch kind {
	case EventBoardOrigin:
		return decodeBoardOriginV1(encoded)
	case EventBoardNameSet:
		return decodeBoardNameSetV1(encoded)
	case EventBoardDescriptionSet:
		return decodeBoardDescriptionSetV1(encoded)
	case EventBoardArchiveSet:
		return decodeBoardArchiveSetV1(encoded)
	case EventBoardIssueIDPrefixSet:
		return decodeBoardIssueIDPrefixSetV1(encoded)
	case EventBoardIssueIDStrategySet:
		return decodeBoardIssueIDStrategySetV1(encoded)
	case EventBoardSummaryLimitSet:
		return decodeBoardSummaryLimitSetV1(encoded)
	case EventBoardAttachmentLimitSet:
		return decodeBoardAttachmentLimitSetV1(encoded)
	case EventBoardPinLimitSet:
		return decodeBoardPinLimitSetV1(encoded)
	case EventIssueOrigin:
		return decodeIssueOriginV1(encoded)
	case EventIssuePublicIDSet:
		return decodeIssuePublicIDSetV1(encoded)
	case EventIssueTitleSet:
		return decodeIssueTitleSetV1(encoded)
	case EventIssueKindSet:
		return decodeIssueKindSetV1(encoded)
	case EventIssueLifecycleSet:
		return decodeIssueLifecycleSetV1(encoded)
	case EventIssuePrioritySet:
		return decodeIssuePrioritySetV1(encoded)
	case EventIssueWaitingSet:
		return decodeIssueWaitingSetV1(encoded)
	case EventIssueSummarySet:
		return decodeIssueSummarySetV1(encoded)
	case EventIssueDetailsSet:
		return decodeIssueDetailsSetV1(encoded)
	case EventIssueLabel:
		return decodeIssueLabelV1(encoded)
	case EventIssueDependency:
		return decodeIssueDependencyV1(encoded)
	case EventIssueContainment:
		return decodeIssueContainmentV1(encoded)
	case EventExternalKey:
		return decodeExternalKeyV1(encoded)
	case EventBoardPin:
		return decodeBoardPinV1(encoded)
	case EventIssueLog:
		return decodeIssueLogV1(encoded)
	case EventIssueState:
		return decodeIssueStateV1(encoded)
	case EventIssueResult:
		return decodeIssueResultV1(encoded)
	case EventIssueCheckpoint:
		return decodeIssueCheckpointV1(encoded)
	case EventAttachmentAdd:
		return decodeAttachmentAddV1(encoded)
	case EventAttachmentRemove:
		return decodeAttachmentRemoveV1(encoded)
	default:
		return nil, fmt.Errorf("unsupported event kind %q", kind)
	}
}

func validateEvent(event Event) error {
	if event.OccurredAt.IsZero() || event.Payload == nil {
		return errors.New("mutation event time and value required")
	}
	if reflect.ValueOf(event.Payload).IsNil() {
		return errors.New("mutation event value required")
	}
	return event.Payload.validateV1()
}

func decodeValueV1[T any](encoded []byte) (*T, error) {
	var value T
	if err := json.Unmarshal(encoded, &value, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &value, nil
}
