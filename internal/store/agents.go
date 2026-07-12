package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// AgentStaleThresholdSec is the default age (seconds) past which an
// active_agents row is considered dead. Loops should heartbeat well
// under this — typical poll interval is 250ms-1s.
const AgentStaleThresholdSec = 30

// AgentTouch upserts an active_agents row with the current timestamp.
// capabilities is the agent's declared capability list (JSON-encoded
// before storage). Called from every poll-loop tick in claim/ready/list.
func (s *Store) AgentTouch(ctx context.Context, name string, pid int, host string, capabilities []string) error {
	if name == "" {
		return errors.New("name required")
	}
	if capabilities == nil {
		capabilities = []string{} // marshal as `[]` rather than `null`
	}
	caps, err := json.Marshal(capabilities)
	if err != nil {
		return err
	}
	t := now()
	a := ActiveAgent{
		Name:         name,
		PID:          pid,
		Host:         host,
		Capabilities: string(caps),
		StartedAt:    t,
		LastSeen:     t,
	}
	_, err = s.db.NewInsert().Model(&a).
		On("CONFLICT (name) DO UPDATE").
		Set("pid = EXCLUDED.pid").
		Set("host = EXCLUDED.host").
		Set("capabilities = EXCLUDED.capabilities").
		// Keep the original started_at across heartbeats.
		Set("last_seen = EXCLUDED.last_seen").
		Exec(ctx)
	return err
}

// AgentRemove drops an active_agents row. Called from a defer in the
// poll-loop so a graceful exit clears the row immediately rather than
// waiting for the freshness threshold.
func (s *Store) AgentRemove(ctx context.Context, name string) error {
	_, err := s.db.NewDelete().
		Model((*ActiveAgent)(nil)).
		Where("name = ?", name).
		Exec(ctx)
	return err
}

// AgentList returns active agents whose last_seen is within
// `freshness` seconds of now. Pass 0 to skip the filter and return all
// rows including stale.
func (s *Store) AgentList(ctx context.Context, freshnessSec int64) ([]ActiveAgent, error) {
	var agents []ActiveAgent
	q := s.db.NewSelect().Model(&agents).OrderExpr("name ASC")
	if freshnessSec > 0 {
		q = q.Where("last_seen >= ?", now()-freshnessSec)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}

// AgentGet looks up a single active agent by name. Returns ErrNotFound
// (overloaded — fine here, callers can distinguish via context) if absent.
func (s *Store) AgentGet(ctx context.Context, name string) (ActiveAgent, error) {
	var a ActiveAgent
	err := s.db.NewSelect().Model(&a).Where("name = ?", name).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ActiveAgent{}, ErrNotFound
	}
	return a, err
}

// DecodeCapabilities is a convenience for callers reading rows back.
func (a ActiveAgent) DecodeCapabilities() []string {
	if a.Capabilities == "" {
		return nil
	}
	var caps []string
	_ = json.Unmarshal([]byte(a.Capabilities), &caps)
	return caps
}
