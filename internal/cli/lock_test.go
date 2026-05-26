package cli

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLockAcquireAndRelease(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	out := c.run("lock", "deploy", "--ttl", "30s", "-a", "alice")
	if !strings.Contains(out, "acquired lock") || !strings.Contains(out, "deploy") {
		t.Fatalf("expected acquire notice:\n%s", out)
	}
	// Listed as live.
	out = c.run("locks")
	if !strings.Contains(out, "deploy") || !strings.Contains(out, "alice") {
		t.Fatalf("locks should show alice's deploy:\n%s", out)
	}
	// Release succeeds.
	c.run("unlock", "deploy", "-a", "alice")
	out = c.run("locks")
	if !strings.Contains(out, "(no locks)") {
		t.Fatalf("expected empty locks after release:\n%s", out)
	}
}

func TestLockContentionFailsWithoutWait(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("lock", "deploy", "--ttl", "30s", "-a", "alice")
	c.runFail("lock", "deploy", "--ttl", "30s", "-a", "bob")
}

func TestLockReacquireBySameHolderRefreshesTTL(t *testing.T) {
	// Same agent re-acquiring should succeed (refreshes TTL) — useful
	// for long-running operations that want to extend their lease.
	c := newTestCLI(t)
	c.run("init")
	c.run("lock", "deploy", "--ttl", "30s", "-a", "alice")
	c.run("lock", "deploy", "--ttl", "30s", "-a", "alice") // must not fail
}

func TestLockUnlockWrongHolderRejected(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("lock", "deploy", "--ttl", "30s", "-a", "alice")
	c.runFail("unlock", "deploy", "-a", "bob")
	// --force lets you steal it (intended for stuck-lock recovery).
	c.run("unlock", "deploy", "-a", "bob", "--force")
	out := c.run("locks")
	if !strings.Contains(out, "(no locks)") {
		t.Fatalf("--force should release stuck lock:\n%s", out)
	}
}

func TestLockExpiredCanBeStolen(t *testing.T) {
	// A 1s TTL that's elapsed → another holder can acquire.
	c := newTestCLI(t)
	c.run("init")
	c.run("lock", "deploy", "--ttl", "1s", "-a", "alice")
	time.Sleep(1100 * time.Millisecond)
	c.run("lock", "deploy", "--ttl", "30s", "-a", "bob") // expired alice's row, takes it
	out := c.run("locks")
	if !strings.Contains(out, "bob") {
		t.Fatalf("bob should now hold deploy:\n%s", out)
	}
}

func TestLockWaitBlocksUntilReleased(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("lock", "deploy", "--ttl", "10s", "-a", "alice")

	// Release alice's lock partway through bob's wait.
	go func() {
		time.Sleep(80 * time.Millisecond)
		c2 := &testCLI{t: t, dir: c.dir, ctx: context.Background()}
		c2.run("unlock", "deploy", "-a", "alice")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c.ctx = ctx
	out := c.run("lock", "deploy", "--ttl", "10s", "-a", "bob", "--wait", "--interval", "30ms")
	if !strings.Contains(out, "acquired") || !strings.Contains(out, "bob") {
		t.Fatalf("expected bob to acquire after alice released:\n%s", out)
	}
}

func TestLockWaitTimeoutFails(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("lock", "deploy", "--ttl", "30s", "-a", "alice")
	// Wait briefly, then time out.
	c.runFail("lock", "deploy", "--ttl", "30s", "-a", "bob", "--wait", "--wait-timeout", "100ms", "--interval", "30ms")
}

func TestLockWithCommandReleasesOnSuccess(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.run("lock", "deploy", "--ttl", "10s", "-a", "alice", "--", "true")
	out := c.run("locks")
	if !strings.Contains(out, "(no locks)") {
		t.Fatalf("lock should be released after cmd:\n%s", out)
	}
}

func TestLockWithCommandReleasesOnFailure(t *testing.T) {
	// Even if the command fails, the lock must still be released.
	c := newTestCLI(t)
	c.run("init")
	c.runFail("lock", "deploy", "--ttl", "10s", "-a", "alice", "--", "false")
	out := c.run("locks")
	if !strings.Contains(out, "(no locks)") {
		t.Fatalf("lock should be released even after cmd failure:\n%s", out)
	}
}

func TestLockJSON(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	out := c.run("--json", "lock", "deploy", "--ttl", "30s", "-a", "alice")
	if !strings.Contains(out, `"name":"deploy"`) || !strings.Contains(out, `"holder":"alice"`) {
		t.Fatalf("expected JSON lock object:\n%s", out)
	}
	out = c.run("--json", "locks")
	if !strings.Contains(out, `"name":"deploy"`) {
		t.Fatalf("locks --json should be an array:\n%s", out)
	}
}

func TestLockRequiresTTL(t *testing.T) {
	c := newTestCLI(t)
	c.run("init")
	c.runFail("lock", "deploy", "--ttl", "0", "-a", "alice")
}

func TestLockRaceTwoSimultaneousAcquires(t *testing.T) {
	// Two goroutines racing to acquire the same lock — exactly one wins.
	c := newTestCLI(t)
	c.run("init")

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for _, name := range []string{"alice", "bob"} {
		wg.Add(1)
		go func(holder string) {
			defer wg.Done()
			c2 := &testCLI{t: t, dir: c.dir, ctx: context.Background()}
			full := []string{"--dir", c2.dir, "lock", "race", "--ttl", "30s", "-a", holder}
			code := Run(c2.ctx, &c2.out, &c2.err, full)
			results <- (code == 0)
		}(name)
	}
	wg.Wait()
	close(results)
	wins := 0
	for ok := range results {
		if ok {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one winner, got %d", wins)
	}
}
