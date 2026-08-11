package board

import (
	"context"
	"errors"
	"fmt"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ApplyDocument validates or atomically persists one canonical apply document.
func (r *Repository) ApplyDocument(
	ctx context.Context,
	issueIDConfiguration configuration.IssueIDConfiguration,
	document planning.ApplyDocument,
	mode planning.ApplyMode,
) (out planning.DocumentApplied, err error) {
	if mode == planning.ApplyModeDryRun {
		return r.dryRunDocument(ctx, document)
	}
	if mode != planning.ApplyModeCommit {
		return out, planning.ErrIncompleteSnapshot
	}

	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()

	snapshot, err := r.applySnapshot(
		ctx,
		mutation.change,
		mutation.current,
		document.ReferencedIssueIDs(),
	)
	if err != nil {
		return out, err
	}
	snapshot.Mode = planning.ApplyModeDryRun
	policy, err := planning.LoadApply(snapshot)
	if err != nil {
		return out, err
	}
	validated, err := policy.ApplyDocument(document)
	if err != nil {
		return out, err
	}
	if validated.Receipt.Counts.Create+validated.Receipt.Counts.Update == 0 {
		validated.Receipt.DryRun = false
		return validated, nil
	}

	snapshot.Mode = planning.ApplyModeCommit
	snapshot.OccurredAt = mutation.occurredAt
	snapshot.AllocatedIDs = make([]issue.ID, len(validated.Receipt.Entries))
	for index, entry := range validated.Receipt.Entries {
		if entry.Action != planning.ApplyActionCreate {
			continue
		}
		snapshot.AllocatedIDs[index], err = r.allocateIssueID(
			ctx,
			mutation,
			issueIDConfiguration,
		)
		if err != nil {
			return out, err
		}
	}
	policy, err = planning.LoadApply(snapshot)
	if err != nil {
		return out, err
	}
	out, err = policy.ApplyDocument(document)
	if err != nil {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	// Materialize every new identity before relationships so forward aliases and
	// keys obey the same document-order-independent contract as policy.
	for _, applied := range out.Applied {
		if applied.Issue.ID() == "" || applied.Existing {
			continue
		}
		if err := r.insertIssue(ctx, mutation, applied.Issue); err != nil {
			return out, err
		}
	}
	for _, applied := range out.Applied {
		if applied.Issue.ID() == "" {
			continue
		}
		if applied.Existing && applied.WriteIssue {
			if err := r.updateIssue(ctx, mutation, applied.Issue); err != nil {
				return out, err
			}
		}
		if applied.WriteLabels {
			if err := r.replaceLabels(ctx, mutation, applied.Issue.ID(), applied.Labels); err != nil {
				return out, err
			}
		}
		if applied.WriteDependencies {
			if err := r.replaceDependencies(
				ctx,
				mutation,
				applied.Issue.ID(),
				applied.Dependencies,
			); err != nil {
				return out, err
			}
		}
		if applied.WriteParent {
			if err := r.replaceParent(ctx, mutation, applied.Issue.ID(), applied.Parent); err != nil {
				return out, err
			}
		}
		if applied.ExternalKey != nil {
			if err := r.insertExternalKey(
				ctx,
				mutation,
				applied.Issue.ID(),
				*applied.ExternalKey,
			); err != nil {
				return out, err
			}
		}
	}
	issueIDs := make([]issue.ID, 0, len(out.Applied))
	for _, applied := range out.Applied {
		if applied.Issue.ID() != "" {
			issueIDs = append(issueIDs, applied.Issue.ID())
		}
	}
	if len(issueIDs) == 0 {
		return out, planning.ErrIncompleteSnapshot
	}
	if err := mutation.commit(ctx, issueIDs...); err != nil {
		return out, err
	}
	out.CommittedRevision = planningCommittedRevision(mutation)
	return out, nil
}

// dryRunDocument reads archived boards because a dry run produces no mutation;
// beginMutation owns the corresponding lifecycle guard for committed applies.
func (r *Repository) dryRunDocument(
	ctx context.Context,
	document planning.ApplyDocument,
) (out planning.DocumentApplied, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	current, err := query.New(view).BoardGetRevision(ctx, r.boardID.String())
	if err != nil {
		return out, err
	}
	snapshot, err := r.applySnapshot(
		ctx,
		view,
		domainboard.Revision(current.Revision),
		document.ReferencedIssueIDs(),
	)
	if err != nil {
		return out, err
	}
	snapshot.Mode = planning.ApplyModeDryRun
	policy, err := planning.LoadApply(snapshot)
	if err != nil {
		return out, err
	}
	return policy.ApplyDocument(document)
}

func (r *Repository) applySnapshot(
	ctx context.Context,
	scope queryScope,
	revision domainboard.Revision,
	referencedIssueIDs []issue.ID,
) (planning.ApplySnapshot, error) {
	ids, err := r.listBoardIssueIDs(ctx, scope)
	if err != nil {
		return planning.ApplySnapshot{}, err
	}
	labels, err := r.readBoardIssueLabels(ctx, scope)
	if err != nil {
		return planning.ApplySnapshot{}, err
	}
	dependencies, err := r.readSnapshotDependencies(ctx, scope)
	if err != nil {
		return planning.ApplySnapshot{}, err
	}
	containment, err := r.readSnapshotContainment(ctx, scope)
	if err != nil {
		return planning.ApplySnapshot{}, err
	}
	dependencyIDs := make(map[issue.ID][]issue.ID, len(ids))
	for _, dependency := range dependencies {
		issueID, err := issue.NewID(dependency.childID)
		if err != nil {
			return planning.ApplySnapshot{}, err
		}
		prerequisiteID, err := issue.NewID(dependency.parentID)
		if err != nil {
			return planning.ApplySnapshot{}, err
		}
		dependencyIDs[issueID] = append(dependencyIDs[issueID], prerequisiteID)
	}
	parents := make(map[issue.ID]issue.ID, len(containment))
	for _, relation := range containment {
		childID, err := issue.NewID(relation.childID)
		if err != nil {
			return planning.ApplySnapshot{}, err
		}
		parentID, err := issue.NewID(relation.parentID)
		if err != nil {
			return planning.ApplySnapshot{}, err
		}
		parents[childID] = parentID
	}
	issues := make(map[issue.ID]planning.ApplyIssueSnapshot, len(ids))
	for _, id := range ids {
		state, _, err := r.readIssueState(ctx, scope, id)
		if err != nil {
			return planning.ApplySnapshot{}, err
		}
		issueLabels, err := labelsFromStrings(labels[id])
		if err != nil {
			return planning.ApplySnapshot{}, err
		}
		value := planning.ApplyIssueSnapshot{
			State: state, Labels: issueLabels,
			Dependencies: dependencyIDs[id],
		}
		if parent, ok := parents[id]; ok {
			value.Parent = &parent
		}
		issues[id] = value
	}
	externalKeys, err := r.readExternalKeys(ctx, scope)
	if err != nil {
		return planning.ApplySnapshot{}, err
	}
	foreignBoards, err := r.readForeignIssueBoards(ctx, scope, referencedIssueIDs)
	if err != nil {
		return planning.ApplySnapshot{}, err
	}
	return planning.ApplySnapshot{
		BoardID: r.boardID, Revision: revision, IssueIDs: ids, Issues: issues,
		ForeignIssueBoards: foreignBoards, ExternalKeys: externalKeys,
	}, nil
}

func (r *Repository) readForeignIssueBoards(
	ctx context.Context,
	scope queryScope,
	referencedIssueIDs []issue.ID,
) (out map[issue.ID]domainboard.ID, err error) {
	values := make(map[issue.ID]domainboard.ID)
	if len(referencedIssueIDs) == 0 {
		return values, nil
	}
	issueIDs := make([]string, len(referencedIssueIDs))
	for index, id := range referencedIssueIDs {
		issueIDs[index] = id.String()
	}
	rows, err := query.New(scope).BoardListApplyForeignIssueBoards(
		ctx,
		query.BoardListApplyForeignIssueBoardsParams{
			BoardID:  r.boardID.String(),
			IssueIDs: issueIDs,
		},
	)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		id := issue.ID(row.ID)
		parsed, err := domainboard.NewID(row.BoardID)
		if err != nil {
			return nil, fmt.Errorf("read board identity %q for issue %q: %w", row.BoardID, id, err)
		}
		values[id] = parsed
	}
	return values, nil
}
