package boardhistory

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeEvent_retainsEveryV1Payload(t *testing.T) {
	var count int
	for line := range strings.Lines(retainedV1Events) {
		encoded := []byte(strings.TrimSuffix(line, "\n"))
		event, err := decodeEvent(encoded)
		require.NoError(t, err)
		roundTrip, err := encodeEvent(event)
		require.NoError(t, err)
		assert.Equal(t, encoded, roundTrip, "event %d", count)
		count++
	}
	assert.Equal(t, 29, count)
}

const retainedV1Events = `{"kind":"board.origin","occurred_at":"2026-09-16T10:00:00Z","value":{"archive":{"actor":"agent","at":"2026-09-16T10:00:00Z","reason":"text \" \\ \n Unicode  "},"attachment_max_bytes":4096,"created_at":"2026-09-16T10:00:00Z","description":null,"issue_id_prefix":"card-","issue_id_strategy":"random","name":"Board","pins_max_count":3,"summary_max_bytes":4096},"version":1}
{"kind":"board.name.set","occurred_at":"2026-09-16T10:00:00Z","value":{"name":"Board"},"version":1}
{"kind":"board.description.set","occurred_at":"2026-09-16T10:00:00Z","value":{"description":{"present":true,"value":"text \" \\ \n Unicode  "}},"version":1}
{"kind":"board.archive.set","occurred_at":"2026-09-16T10:00:00Z","value":{"archive":null,"archived":false},"version":1}
{"kind":"board.issue_id_prefix.set","occurred_at":"2026-09-16T10:00:00Z","value":{"prefix":{"present":true,"value":"card-"}},"version":1}
{"kind":"board.issue_id_strategy.set","occurred_at":"2026-09-16T10:00:00Z","value":{"strategy":{"present":true,"value":"random"}},"version":1}
{"kind":"board.summary_limit.set","occurred_at":"2026-09-16T10:00:00Z","value":{"limit":{"present":true,"value":4096}},"version":1}
{"kind":"board.attachment_limit.set","occurred_at":"2026-09-16T10:00:00Z","value":{"limit":{"present":true,"value":4096}},"version":1}
{"kind":"board.pin_limit.set","occurred_at":"2026-09-16T10:00:00Z","value":{"limit":{"present":true,"value":3}},"version":1}
{"kind":"issue.origin","occurred_at":"2026-09-16T10:00:00Z","value":{"closed_at":"2026-09-16T10:00:00Z","created_at":"2026-09-16T10:00:00Z","details":"text \" \\ \n Unicode  ","kind":"task","lifecycle":"closed","priority":2,"public_id":"card-1","summary":"text \" \\ \n Unicode  ","title":"Title","uid":"AgICAgICAgICAgICAgICAg==","updated_at":"2026-09-16T10:00:00Z","waiting_reason":"text \" \\ \n Unicode  ","waiting_since":"2026-09-16T10:00:00Z"},"version":1}
{"kind":"issue.public_id.set","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","public_id":"card-2"},"version":1}
{"kind":"issue.title.set","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","title":"text \" \\ \n Unicode  "},"version":1}
{"kind":"issue.kind.set","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","kind":"workstream"},"version":1}
{"kind":"issue.lifecycle.set","occurred_at":"2026-09-16T10:00:00Z","value":{"closed_at":{"present":true,"value":"2026-09-16T10:00:00Z"},"issue_uid":"AgICAgICAgICAgICAgICAg==","lifecycle":"closed"},"version":1}
{"kind":"issue.priority.set","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","priority":3},"version":1}
{"kind":"issue.waiting.set","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","waiting":{"reason":"text \" \\ \n Unicode  ","since":"2026-09-16T10:00:00Z","waiting":true}},"version":1}
{"kind":"issue.summary.set","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","summary":{"present":false,"value":""}},"version":1}
{"kind":"issue.details.set","occurred_at":"2026-09-16T10:00:00Z","value":{"details":{"present":true,"value":"text \" \\ \n Unicode  "},"issue_uid":"AgICAgICAgICAgICAgICAg=="},"version":1}
{"kind":"issue.label","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","label":"label","present":true},"version":1}
{"kind":"issue.dependency","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","prerequisite_uid":"AwMDAwMDAwMDAwMDAwMDAw==","present":true},"version":1}
{"kind":"issue.containment","occurred_at":"2026-09-16T10:00:00Z","value":{"child_uid":"AgICAgICAgICAgICAgICAg==","parent_uid":"AwMDAwMDAwMDAwMDAwMDAw==","present":false},"version":1}
{"kind":"board.external_key","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","key":"text \" \\ \n Unicode  ","present":true},"version":1}
{"kind":"board.pin","occurred_at":"2026-09-16T10:00:00Z","value":{"issue_uid":"AgICAgICAgICAgICAgICAg==","position":2,"present":true},"version":1}
{"kind":"issue.log","occurred_at":"2026-09-16T10:00:00Z","value":{"author":"agent","body":"text \" \\ \n Unicode  ","committer":"agent","created_at":"2026-09-16T10:00:00Z","id":"log_0123456789abcdef0123456789abcdef","issue_uid":"AgICAgICAgICAgICAgICAg==","kind":"state_snapshot","next_action":"text \" \\ \n Unicode  "},"version":1}
{"kind":"issue.state","occurred_at":"2026-09-16T10:00:00Z","value":{"author":"agent","body":"text \" \\ \n Unicode  ","issue_uid":"AgICAgICAgICAgICAgICAg==","next_action":"text \" \\ \n Unicode  ","present":true,"snapshot_log_entry_id":"log_0123456789abcdef0123456789abcdef","updated_at":"2026-09-16T10:00:00Z"},"version":1}
{"kind":"issue.result","occurred_at":"2026-09-16T10:00:00Z","value":{"body":"text \" \\ \n Unicode  ","issue_uid":"AgICAgICAgICAgICAgICAg==","present":true},"version":1}
{"kind":"issue.checkpoint","occurred_at":"2026-09-16T10:00:00Z","value":{"decided_at":"2026-09-16T10:00:00Z","issue_uid":"AgICAgICAgICAgICAgICAg==","outcome":"denied","reason":"text \" \\ \n Unicode  "},"version":1}
{"kind":"attachment.add","occurred_at":"2026-09-16T10:00:00Z","value":{"actor":"agent","at":"2026-09-16T10:00:00Z","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","filename":"result.txt","id":"att_aeaqcaibaeaqcaibaeaqcaibae","media_type":"text/plain","origin_issue_uid":"AgICAgICAgICAgICAgICAg==","size_bytes":23},"version":1}
{"kind":"attachment.remove","occurred_at":"2026-09-16T10:00:00Z","value":{"actor":"agent","at":"2026-09-16T10:00:00Z","id":"att_aeaqcaibaeaqcaibaeaqcaibae"},"version":1}
`
