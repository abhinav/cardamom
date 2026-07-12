package cli

import (
	"errors"
	"fmt"

	"github.com/arjia-labs/clu/internal/store"
)

type DoctorCmd struct {
	StuckHours int `name:"stuck-hours" default:"24" help:"Flag in-progress issues not updated in this many hours (0 disables)."`
}

func (c *DoctorCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		rep, err := s.Doctor(r.ctx, c.StuckHours)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(rep)
		}
		check := func(label, value string, ok bool) {
			mark := "✓"
			if !ok {
				mark = "✗"
			}
			fmt.Fprintf(r.stdout, "  %-22s %-20s %s\n", label+":", value, mark)
		}
		ver := fmt.Sprintf("%d (code expects %d)", rep.DBSchemaVersion, rep.CodeSchemaVersion)
		check("Schema version", ver, rep.DBSchemaVersion == rep.CodeSchemaVersion)
		fkValue := "OK"
		if !rep.ForeignKeyOK {
			fkValue = fmt.Sprintf("%d errors", len(rep.ForeignKeyErrors))
		}
		check("Foreign keys", fkValue, rep.ForeignKeyOK)
		check("Orphaned labels", fmt.Sprintf("%d", rep.OrphanedLabels), rep.OrphanedLabels == 0)
		check("Orphaned deps", fmt.Sprintf("%d", rep.OrphanedDeps), rep.OrphanedDeps == 0)
		if rep.StuckThresholdH > 0 {
			check("Stuck in_progress", fmt.Sprintf("%d (>%dh)", rep.StuckInProgress, rep.StuckThresholdH), rep.StuckInProgress == 0)
		}
		check("Closed+deferred", fmt.Sprintf("%d", rep.ClosedButDeferred), rep.ClosedButDeferred == 0)
		check("Invalid status", fmt.Sprintf("%d", rep.InvalidStatus), rep.InvalidStatus == 0)
		check("Invalid type", fmt.Sprintf("%d", rep.InvalidType), rep.InvalidType == 0)
		check("Invalid priority", fmt.Sprintf("%d", rep.InvalidPriority), rep.InvalidPriority == 0)
		check("Failing cron jobs", fmt.Sprintf("%d", rep.CronJobsFailing), rep.CronJobsFailing == 0)
		for _, e := range rep.ForeignKeyErrors {
			fmt.Fprintf(r.stdout, "  fk error: %s\n", e)
		}
		if !rep.OK() {
			return errors.New("doctor found issues")
		}
		return nil
	})
}
