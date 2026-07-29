package issueconnect

import (
	"cmp"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"google.golang.org/protobuf/proto"
)

const (
	issuePageTokenVersion = 1
	defaultIssuePageSize  = 100
	maximumIssuePageSize  = 250
)

// issuePage is the validated slice of one stable ordered collection.
type issuePage struct {
	size   int
	offset int
	query  string
	// revisions is the coherent board vector encoded into a next-page token.
	revisions []issuePageRevision
	// expected is the board vector decoded from an incoming token.
	expected []issuePageRevision
}

// issuePageToken is the opaque continuation state carried by the protocol.
type issuePageToken struct {
	// Version selects the token schema understood by this server.
	Version int `json:"version"`
	// Query binds the token to every query dimension except page size.
	Query string `json:"query"`
	// Offset is the next global ordered position to return.
	Offset int `json:"offset"`
	// Revisions pin every selected board to the token's coherent snapshot.
	Revisions []issuePageRevision `json:"revisions"`
}

// issuePageRevision pins one selected board to its coherent event revision.
type issuePageRevision struct {
	// BoardID identifies the board whose canonical revision was observed.
	BoardID string `json:"board_id"`
	// Revision is the latest canonical event in the repository read scope.
	Revision int64 `json:"revision"`
}

func newIssuePage(
	request *privatev1.ListIssuesRequest,
) (issuePage, error) {
	query, err := issuePageQuery(request)
	if err != nil {
		return issuePage{}, err
	}
	page := issuePage{
		size: issuePageSize(request.GetLimit()), query: query,
	}
	if request.PageToken == nil {
		return page, nil
	}
	token, err := decodeIssuePageToken(request.GetPageToken())
	if err != nil {
		return issuePage{}, err
	}
	if token.Query != query {
		return issuePage{}, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: page token does not match the issue query",
		)
	}
	page.offset = token.Offset
	page.expected = token.Revisions
	return page, nil
}

func issuePageSize(requested uint32) int {
	if requested == 0 {
		return defaultIssuePageSize
	}
	return min(int(requested), maximumIssuePageSize)
}

func issuePageQuery(request *privatev1.ListIssuesRequest) (string, error) {
	query := proto.Clone(request).(*privatev1.ListIssuesRequest)
	query.Limit = 0
	query.PageToken = nil
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(query)
	if err != nil {
		return "", fmt.Errorf("encode issue page query: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func (p *issuePage) setRevisions(revisions []issuePageRevision) error {
	slices.SortFunc(revisions, func(left, right issuePageRevision) int {
		return cmp.Compare(left.BoardID, right.BoardID)
	})
	if p.expected != nil && !slices.Equal(p.expected, revisions) {
		return errkind.Errorf(
			errkind.Conflict,
			"page token is stale; restart the issue query",
		)
	}
	p.revisions = slices.Clone(revisions)
	return nil
}

func (p *issuePage) nextToken(offset int) (string, error) {
	encoded, err := json.Marshal(issuePageToken{
		Version: issuePageTokenVersion, Query: p.query,
		Offset: offset, Revisions: p.revisions,
	})
	if err != nil {
		return "", fmt.Errorf("encode issue page token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeIssuePageToken(encoded string) (issuePageToken, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return issuePageToken{}, invalidIssuePageToken(err)
	}
	var token issuePageToken
	if err := json.Unmarshal(data, &token); err != nil {
		return issuePageToken{}, invalidIssuePageToken(err)
	}
	if token.Version != issuePageTokenVersion {
		return issuePageToken{}, invalidIssuePageToken(fmt.Errorf(
			"unsupported version %d",
			token.Version,
		))
	}
	if token.Query == "" || token.Offset <= 0 {
		return issuePageToken{}, invalidIssuePageToken(
			errors.New("query and positive offset are required"),
		)
	}
	return token, nil
}

func invalidIssuePageToken(cause error) error {
	return errkind.Errorf(errkind.InvalidInput, "invalid input: invalid page token: %w", cause)
}
