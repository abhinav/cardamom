package execution

import (
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

func issueProjection(state issue.State, revision board.Revision) issue.Issue {
	if state.ID() == "" {
		return issue.Issue{}
	}
	var nextAction *string
	if recovery := state.RecoveryStateRecord(); recovery != nil {
		nextAction = optionalString(recovery.NextAction)
	}
	return issue.Issue{
		ID: state.ID().String(), Title: state.Title(), Type: state.Kind().String(),
		Lifecycle: state.Lifecycle().String(), Status: state.Status().String(),
		Priority: state.Priority().Int(), ActiveClaim: activeClaimProjection(state.ActiveClaim()),
		Assignee: optionalString(state.Assignee().String()),
		Created:  state.Created().Unix(), Updated: state.Updated().Unix(),
		StartedAt: unixPointer(state.StartedAt()), Closed: unixPointer(state.ClosedAt()),
		Waiting: waitingProjection(state.Waiting()),
		Summary: optionalString(state.Summary()), Details: optionalString(state.Details()),
		State: optionalString(state.RecoveryState()), NextAction: nextAction,
		Revision: int64(revision),
	}
}

func waitingProjection(waiting *issue.WaitingState) *issue.Waiting {
	if waiting == nil {
		return nil
	}
	return &issue.Waiting{Reason: waiting.Reason, Since: waiting.Since.Unix()}
}

func activeClaimProjection(claim *issue.ClaimState) *issue.ActiveClaim {
	if claim == nil {
		return nil
	}
	return &issue.ActiveClaim{Actor: claim.Actor.String(), StartedAt: claim.StartedAt.Unix()}
}

func issueProjections(states []issue.State, revision board.Revision) []issue.Issue {
	result := make([]issue.Issue, len(states))
	for index, state := range states {
		result[index] = issueProjection(state, revision)
	}
	return result
}

func parentIDStrings(groups ...[]issue.ID) []string {
	seen := make(map[issue.ID]struct{})
	parents := make([]string, 0)
	for _, group := range groups {
		for _, parent := range group {
			if _, ok := seen[parent]; ok {
				continue
			}
			seen[parent] = struct{}{}
			parents = append(parents, parent.String())
		}
	}
	return parents
}

func issueSummary(detail issue.Detail) issue.Summary {
	return issue.Summary{
		Issue:   detail.Issue,
		Labels:  detail.Labels,
		Blocked: detail.Blocked,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func unixPointer(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	unix := value.Unix()
	return &unix
}
