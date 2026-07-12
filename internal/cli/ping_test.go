package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPingAndInboxBasic(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("ping", "eng2", "hey hold off on session.go", "-a", "alice")
	// alice's own inbox: empty.
	out := c.run("inbox", "-a", "alice")
	if !strings.Contains(out, "(no pings)") {
		t.Fatalf("alice's inbox should be empty:\n%s", out)
	}
	// eng2 sees it.
	out = c.run("inbox", "-a", "eng2")
	if !strings.Contains(out, "from alice") || !strings.Contains(out, "session.go") {
		t.Fatalf("eng2 should see the ping:\n%s", out)
	}
	// Second read marks-as-default; the ping is now consumed.
	out = c.run("inbox", "-a", "eng2")
	if !strings.Contains(out, "(no pings)") {
		t.Fatalf("ping should be marked read after first list:\n%s", out)
	}
	// --all surfaces the read row.
	out = c.run("inbox", "-a", "eng2", "--all")
	if !strings.Contains(out, "session.go") || !strings.Contains(out, "(read)") {
		t.Fatalf("--all should show read pings with marker:\n%s", out)
	}
}

func TestInboxPeekDoesNotMarkRead(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("ping", "eng2", "msg", "-a", "alice")
	// Peek shouldn't consume.
	c.run("inbox", "-a", "eng2", "--peek")
	// Default read should still see it.
	out := c.run("inbox", "-a", "eng2")
	if !strings.Contains(out, "msg") {
		t.Fatalf("peek should not mark read; ping missing on next read:\n%s", out)
	}
}

func TestInboxClearMarksAllRead(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	for i := 0; i < 3; i++ {
		c.run("ping", "eng2", "msg", "-a", "alice")
	}
	out := c.run("inbox", "--clear", "-a", "eng2")
	if !strings.Contains(out, "cleared 3") {
		t.Fatalf("expected 'cleared 3':\n%s", out)
	}
	out = c.run("inbox", "-a", "eng2")
	if !strings.Contains(out, "(no pings)") {
		t.Fatalf("inbox should be empty after clear:\n%s", out)
	}
}

func TestPingStdin(t *testing.T) {
	// `ping <recip> -` should read body from stdin. Tested by piping
	// is awkward without rewiring; instead verify the explicit '-'
	// arg behaviour by stuffing stdin via the test harness — but the
	// CLI harness routes os.Stdin, not a buffer. Here just exercise
	// the explicit "body via positional" path and let the unit test
	// for readBody cover the rest.
	c := newTestCLI(t)
	c.run("init")
	c.run("ping", "eng2", "from", "args")
	out := c.run("inbox", "-a", "eng2", "--peek")
	if !strings.Contains(out, "from args") {
		t.Fatalf("positional body should be joined:\n%s", out)
	}
}

func TestPingRejectsEmpty(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("ping", "eng2") // no body, no stdin pipe (test harness has TTY-ish stdin? need explicit fail)
	c.runFail("ping", "eng2", "")
}

func TestInboxJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("ping", "eng2", "structured", "-a", "alice")
	out := c.run("--json", "inbox", "-a", "eng2", "--peek")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0]["sender"] != "alice" || rows[0]["recipient"] != "eng2" || rows[0]["body"] != "structured" {
		t.Fatalf("wrong shape: %+v", rows[0])
	}
}

func TestBriefShowsUnreadCount(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	// Send some pings to $USER (the default --agent on inbox/brief).
	for i := 0; i < 2; i++ {
		c.run("ping", currentUserName(t), "x", "-a", "sender")
	}
	out := c.run("brief")
	if !strings.Contains(out, "2 unread ping(s)") {
		t.Fatalf("brief should surface unread count:\n%s", out)
	}
}
