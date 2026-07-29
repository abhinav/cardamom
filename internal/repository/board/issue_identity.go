package board

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) allocateIssueID(
	ctx context.Context,
	mutation *mutation,
	policy configuration.IssueIDConfiguration,
) (issue.ID, error) {
	prefix := policy.Prefix.String()
	if prefix == "" {
		prefix = r.idPrefix
	}
	strategy := policy.Strategy.String()
	if strategy == "" {
		strategy = r.idStrategy
	}
	if strategy == "sequential" {
		number, err := mutation.change.ReserveIssueNumber(ctx)
		if err != nil {
			return "", err
		}
		return issue.NewID(prefix + strconv.FormatInt(number, 10))
	}

	queries := query.New(mutation.change)
	count, err := queries.BoardCountAllIssues(ctx)
	if err != nil {
		return "", err
	}
	length := randomSuffixLength(count)
	for range 32 {
		suffix, err := randomIssueSuffix(r.entropy, length)
		if err != nil {
			return "", err
		}
		candidate, err := issue.NewID(prefix + suffix)
		if err != nil {
			return "", err
		}
		if _, reserved := mutation.reservedIssueIDs[candidate]; reserved {
			continue
		}
		exists, err := queries.BoardIssueIDExists(ctx, candidate.String())
		if err != nil {
			return "", err
		}
		if !exists {
			mutation.reservedIssueIDs[candidate] = struct{}{}
			return candidate, nil
		}
	}
	return "", errors.New("allocate random issue ID: collision limit reached")
}

func randomSuffixLength(issueCount int64) int {
	length := 4
	for threshold := int64(512); issueCount >= threshold; threshold *= 32 {
		length++
	}
	return length
}

func randomIssueSuffix(source io.Reader, length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	body := make([]byte, length)
	random := make([]byte, length)
	if _, err := io.ReadFull(source, random); err != nil {
		return "", fmt.Errorf("generate issue identity: %w", err)
	}
	for index, value := range random {
		body[index] = alphabet[int(value)&31]
	}
	return string(body), nil
}

func (r *Repository) listBoardIssueIDs(ctx context.Context, scope queryScope) ([]issue.ID, error) {
	values, err := query.New(scope).BoardListIssueIDs(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	var ids []issue.ID
	for _, value := range values {
		ids = append(ids, issue.ID(value))
	}
	return ids, nil
}
