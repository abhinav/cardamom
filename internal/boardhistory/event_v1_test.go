package boardhistory

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestEncodeEvent_normalizesTimestampsToUTC(t *testing.T) {
	zone := time.FixedZone("TestOffset", 5*60*60+30*60)
	at := time.Date(2026, time.September, 16, 15, 30, 0, 0, zone)
	reason := "complete"
	encoded, err := encodeEvent(Event{
		OccurredAt: at,
		Payload: &BoardOrigin{
			CreatedAt: at,
			Name:      "Board",
			Archive:   &board.Archive{Actor: "agent", At: at, Reason: &reason},
		},
	})
	require.NoError(t, err)

	const want = `{"kind":"board.origin","occurred_at":"2026-09-16T10:00:00Z","value":{"archive":{"actor":"agent","at":"2026-09-16T10:00:00Z","reason":"complete"},"attachment_max_bytes":null,"created_at":"2026-09-16T10:00:00Z","description":null,"issue_id_prefix":null,"issue_id_strategy":null,"name":"Board","pins_max_count":null,"summary_max_bytes":null},"version":1}`
	assert.Equal(t, want, string(encoded))
	_, err = decodeEvent(encoded)
	require.NoError(t, err)
}

func TestEncodeEvent_streamsCanonicalStrings(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	event := Event{
		OccurredAt: at,
		Payload: &IssueTitleSet{
			IssueUID: testIssueUID(t),
			Title:    "quotes \" slashes \\ controls \b\f\n\r\t\u000b Unicode \u2028",
		},
	}
	payload, err := canonicalJSONForTest(event.Payload.valueV1())
	require.NoError(t, err)
	want, err := canonicalJSONForTest(eventV1{
		Version: 1, Kind: event.Payload.kind(),
		OccurredAt: event.OccurredAt.UTC(), Value: payload,
	})
	require.NoError(t, err)

	encoded, err := encodeEvent(event)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(encoded))
}

func TestEncodeEvent_matchesRetainedV1ForEveryPayloadKind(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	uid := testIssueUID(t)
	otherUID, err := issue.ParseUID(bytes.Repeat([]byte{3}, 16))
	require.NoError(t, err)
	text := "text \" \\ \n Unicode \u2028"
	actor := "agent"
	prefix := "card-"
	strategy := "random"
	limit := int64(4096)
	pinLimit := int64(3)
	logID := "log_0123456789abcdef0123456789abcdef"
	attachmentID := "att_aeaqcaibaeaqcaibaeaqcaibae"
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := []struct {
		name    string
		payload Payload
	}{
		{name: "BoardOrigin", payload: &BoardOrigin{
			CreatedAt: at, Name: "Board", Description: nil,
			Archive:       &board.Archive{Actor: actor, At: at, Reason: &text},
			IssueIDPrefix: &prefix, IssueIDStrategy: &strategy,
			SummaryMaxBytes: &limit, AttachmentMaxBytes: &limit,
			PinsMaxCount: &pinLimit,
		}},
		{name: "BoardNameSet", payload: &BoardNameSet{Name: "Board"}},
		{name: "BoardDescriptionSet", payload: &BoardDescriptionSet{Description: &text}},
		{name: "BoardArchiveSet", payload: &BoardArchiveSet{}},
		{name: "BoardIssueIDPrefixSet", payload: &BoardIssueIDPrefixSet{Prefix: &prefix}},
		{name: "BoardIssueIDStrategySet", payload: &BoardIssueIDStrategySet{Strategy: &strategy}},
		{name: "BoardSummaryLimitSet", payload: &BoardSummaryLimitSet{Limit: &limit}},
		{name: "BoardAttachmentLimitSet", payload: &BoardAttachmentLimitSet{Limit: &limit}},
		{name: "BoardPinLimitSet", payload: &BoardPinLimitSet{Limit: &pinLimit}},
		{name: "IssueOrigin", payload: &IssueOrigin{
			UID: uid, PublicID: "card-1", Title: "Title", Kind: "task",
			Lifecycle: "closed", Priority: 2, CreatedAt: at, UpdatedAt: at,
			ClosedAt: &at, Waiting: &issue.WaitingState{Reason: text, Since: at},
			Summary: &text, Details: &text,
		}},
		{name: "IssuePublicIDSet", payload: &IssuePublicIDSet{IssueUID: uid, PublicID: "card-2"}},
		{name: "IssueTitleSet", payload: &IssueTitleSet{IssueUID: uid, Title: text}},
		{name: "IssueKindSet", payload: &IssueKindSet{IssueUID: uid, Kind: "workstream"}},
		{name: "IssueLifecycleSet", payload: &IssueLifecycleSet{
			IssueUID: uid, Lifecycle: "closed",
			ClosedAt: &at,
		}},
		{name: "IssuePrioritySet", payload: &IssuePrioritySet{IssueUID: uid, Priority: 3}},
		{name: "IssueWaitingSet", payload: &IssueWaitingSet{
			IssueUID: uid,
			Waiting:  &issue.WaitingState{Reason: text, Since: at},
		}},
		{name: "IssueSummarySet", payload: &IssueSummarySet{IssueUID: uid}},
		{name: "IssueDetailsSet", payload: &IssueDetailsSet{
			IssueUID: uid,
			Details:  &text,
		}},
		{name: "IssueLabel", payload: &IssueLabel{IssueUID: uid, Label: "label", Present: true}},
		{name: "IssueDependency", payload: &IssueDependency{IssueUID: uid, PrerequisiteUID: otherUID, Present: true}},
		{name: "IssueContainment", payload: &IssueContainment{ChildUID: uid, ParentUID: otherUID}},
		{name: "ExternalKey", payload: &ExternalKey{Key: text, IssueUID: uid, Present: true}},
		{name: "BoardPin", payload: &BoardPin{IssueUID: uid, Present: true, Position: 2}},
		{name: "IssueLog", payload: &IssueLog{
			IssueUID: uid, ID: logID, Kind: "state_snapshot",
			Author: &actor, Committer: &actor, Body: text,
			CreatedAt: &at, NextAction: &text,
		}},
		{name: "IssueState", payload: &IssueState{
			IssueUID: uid,
			State: &StateRecord{
				Body: text, Author: &actor, UpdatedAt: &at,
				SnapshotLogEntryID: &logID, NextAction: &text,
			},
		}},
		{name: "IssueResult", payload: &IssueResult{IssueUID: uid, Body: &text}},
		{name: "IssueCheckpoint", payload: &IssueCheckpoint{IssueUID: uid, Outcome: "denied", Reason: text, DecidedAt: at}},
		{name: "AttachmentAdd", payload: &AttachmentAdd{
			ID: attachmentID, OriginIssueUID: &uid, Digest: digest,
			SizeBytes: 23, Filename: "result.txt", MediaType: "text/plain",
			Actor: actor, At: at,
		}},
		{name: "AttachmentRemove", payload: &AttachmentRemove{ID: attachmentID, Actor: actor, At: at}},
	}
	require.Len(t, tests, 29)
	var fixture strings.Builder
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := tt.payload
			event := Event{OccurredAt: at, Payload: payload}
			value, err := canonicalJSONForTest(event.Payload.valueV1())
			require.NoError(t, err)
			want, err := canonicalJSONForTest(eventV1{
				Version: 1, Kind: event.Payload.kind(),
				OccurredAt: event.OccurredAt.UTC(), Value: value,
			})
			require.NoError(t, err)

			got, err := encodeEvent(Event{OccurredAt: at, Payload: payload})
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
			fixture.Write(got)
			fixture.WriteByte('\n')
		})
	}
	assert.Equal(t, retainedV1Events, fixture.String())
}

func TestEncodeEvent_normalizesEveryV1Timestamp(t *testing.T) {
	zone := time.FixedZone("TestOffset", -7*60*60)
	at := time.Date(2026, time.September, 16, 3, 0, 0, 0, zone)
	uid := testIssueUID(t)
	logID := "log_0123456789abcdef0123456789abcdef"
	author := "agent"
	tests := []struct {
		name    string
		payload Payload
	}{
		{name: "IssueOrigin", payload: &IssueOrigin{
			UID: uid, PublicID: "card-1", Title: "Title", Kind: "task",
			Lifecycle: "closed", Priority: 2, CreatedAt: at, UpdatedAt: at,
			ClosedAt: &at,
		}},
		{name: "IssueLifecycle", payload: &IssueLifecycleSet{
			IssueUID: uid, Lifecycle: "closed",
			ClosedAt: &at,
		}},
		{name: "IssueWaiting", payload: &IssueWaitingSet{
			IssueUID: uid,
			Waiting:  &issue.WaitingState{Reason: "external", Since: at},
		}},
		{name: "IssueLog", payload: &IssueLog{
			IssueUID: uid, ID: logID, Kind: "post", Body: "Body", CreatedAt: &at,
		}},
		{name: "IssueState", payload: &IssueState{
			IssueUID: uid,
			State:    &StateRecord{Body: "State", Author: &author, UpdatedAt: &at},
		}},
		{name: "IssueCheckpoint", payload: &IssueCheckpoint{
			IssueUID: uid, Outcome: "approved", DecidedAt: at,
		}},
		{name: "AttachmentAdd", payload: &AttachmentAdd{
			ID:        "att_aeaqcaibaeaqcaibaeaqcaibae",
			Digest:    "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			SizeBytes: 0, Filename: "file.txt", MediaType: "text/plain",
			Actor: "agent", At: at,
		}},
		{name: "AttachmentRemove", payload: &AttachmentRemove{
			ID: "att_aeaqcaibaeaqcaibaeaqcaibae", Actor: "agent", At: at,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeEvent(Event{OccurredAt: at, Payload: tt.payload})
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "-07:00")
			_, err = decodeEvent(encoded)
			require.NoError(t, err)
		})
	}
}

func TestEncodeEvent_representsClearAndRemoval(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	uid := testIssueUID(t)
	for _, payload := range []Payload{
		&IssueState{IssueUID: uid},
		&IssueResult{IssueUID: uid},
		&BoardPin{IssueUID: uid},
		&IssueSummarySet{IssueUID: uid},
		&IssueLifecycleSet{IssueUID: uid, Lifecycle: "open"},
	} {
		_, err := encodeEvent(Event{OccurredAt: at, Payload: payload})
		require.NoError(t, err, "encode %s", payload.kind())
	}

	_, err := encodeEvent(Event{
		OccurredAt: at,
		Payload:    &BoardPin{IssueUID: uid, Position: 1},
	})
	assert.ErrorContains(t, err, "removed board pin")

	_, err = encodeEvent(Event{
		OccurredAt: at,
		Payload: &IssueLifecycleSet{
			IssueUID:  uid,
			Lifecycle: "closed",
		},
	})
	assert.ErrorContains(t, err, "lifecycle and closure time")
}

func TestEncodeEvent_preservesExternalKeyWhitespace(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	uid := testIssueUID(t)
	whitespace := " \t\n"
	payload := &ExternalKey{Key: whitespace, IssueUID: uid, Present: true}
	_, err := encodeEvent(Event{OccurredAt: at, Payload: payload})
	require.NoError(t, err)
}

func TestEncodeEvent_rejectsTypedNilPayload(t *testing.T) {
	var payload *BoardNameSet
	_, err := encodeEvent(Event{
		OccurredAt: time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC),
		Payload:    payload,
	})
	assert.ErrorContains(t, err, "mutation event value required")
}

func TestDecodeEvent_dispatchesUnknownVersionBeforeStrictShape(t *testing.T) {
	_, err := decodeEvent([]byte(`{"future_member":true,"version":2}`))
	require.Error(t, err)
	assert.ErrorContains(t, err, "unsupported event version 2")
	assert.NotContains(t, err.Error(), "unknown")
}

func TestDecodeEvent_rejectsAbsentAssignmentWithValue(t *testing.T) {
	tests := []struct {
		name  string
		kind  EventKind
		value string
	}{
		{name: "BoardDescription", kind: EventBoardDescriptionSet, value: `{"description":{"present":false,"value":"hidden"}}`},
		{name: "BoardPrefix", kind: EventBoardIssueIDPrefixSet, value: `{"prefix":{"present":false,"value":"card-"}}`},
		{name: "BoardStrategy", kind: EventBoardIssueIDStrategySet, value: `{"strategy":{"present":false,"value":"random"}}`},
		{name: "SummaryLimit", kind: EventBoardSummaryLimitSet, value: `{"limit":{"present":false,"value":1}}`},
		{name: "AttachmentLimit", kind: EventBoardAttachmentLimitSet, value: `{"limit":{"present":false,"value":1}}`},
		{name: "PinLimit", kind: EventBoardPinLimitSet, value: `{"limit":{"present":false,"value":1}}`},
		{name: "IssueSummary", kind: EventIssueSummarySet, value: `{"issue_uid":"AgICAgICAgICAgICAgICAg==","summary":{"present":false,"value":"hidden"}}`},
		{name: "IssueDetails", kind: EventIssueDetailsSet, value: `{"details":{"present":false,"value":"hidden"},"issue_uid":"AgICAgICAgICAgICAgICAg=="}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := `{"kind":"` + string(tt.kind) + `","occurred_at":"2026-09-16T10:00:00Z","value":` + tt.value + `,"version":1}`
			_, err := decodeEvent([]byte(encoded))
			assert.Error(t, err)
		})
	}
}
