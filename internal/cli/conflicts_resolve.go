package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/issue"
)

// resolveCommand sends a deliberate choice against the identity the caller read.
// Syntax checks belong here; the domain validates the resulting state.
type resolveCommand struct {
	ID                string    `arg:"" name:"conflict" help:"Reviewed conflict identity."`
	Take              *string   `help:"Select one displayed alternative."`
	File              *string   `type:"path" help:"Compose supported text from a UTF-8 file; - reads standard input."`
	Relationships     bool      `help:"Revalidate the reviewed relationship assignments, with optional repairs."`
	RemoveAssignment  []string  `help:"Remove a contributing relationship assignment (repeatable)."`
	Parent            []string  `help:"Choose CHILD:PARENT containment (repeatable)."`
	Unparent          []issueID `help:"Remove the child's parent (repeatable)."`
	Waiting           *string   `enum:"keep,clear" help:"Choose whether waiting remains in an execution repair."`
	WaitingAssignment string    `help:"Retained waiting assignment to keep when several reasons compete."`
	Kind              *string   `enum:"task,workstream,checkpoint,routine" help:"Choose a kind for the execution repair."`
	Lifecycle         *string   `enum:"open,closed,cancelled" help:"Choose a lifecycle for the execution repair."`
}

func (c *resolveCommand) Validate() error {
	count := 0
	if c.Take != nil {
		count++
	}
	if c.File != nil {
		count++
	}
	if c.Relationships || len(c.RemoveAssignment)+len(c.Parent)+len(c.Unparent) > 0 {
		count++
	}
	if c.Waiting != nil {
		count++
	}
	if count != 1 {
		return UsageErrorf("choose one of --take, --file, relationship repair flags, or --waiting")
	}
	if c.Waiting == nil && (c.WaitingAssignment != "" || c.Kind != nil || c.Lifecycle != nil) {
		return UsageErrorf("--waiting-assignment, --kind, and --lifecycle require --waiting")
	}
	return nil
}

func (c *resolveCommand) Run(inv *Invocation, service *boardconflict.Service) error {
	var result boardconflict.Resolution
	var err error
	switch {
	case c.Take != nil:
		result, err = service.Resolve(inv.Context, boardconflict.ResolveRequest{ConflictID: c.ID, Choice: boardconflict.TakeAlternative{ID: *c.Take}})
	case c.File != nil:
		var content []byte
		if *c.File == "-" {
			content, err = io.ReadAll(inv.Stdin)
		} else {
			content, err = os.ReadFile(*c.File)
		}
		if err != nil {
			return fmt.Errorf("read replacement text: %w", err)
		}
		result, err = service.Resolve(inv.Context, boardconflict.ResolveRequest{ConflictID: c.ID, Choice: boardconflict.ReplaceText{Body: string(content)}})
	case c.Waiting != nil:
		request := boardconflict.ExecutionRequest{ConflictID: c.ID, WaitingAssignment: boardconflict.AssignmentID(c.WaitingAssignment)}
		if *c.Waiting == "keep" {
			request.Waiting = boardconflict.KeepWaiting
		} else {
			request.Waiting = boardconflict.ClearWaiting
		}
		if c.Kind != nil {
			value, e := issue.NewKind(*c.Kind)
			if e != nil {
				return e
			}
			request.Kind = &value
		}
		if c.Lifecycle != nil {
			value, e := issue.NewLifecycle(*c.Lifecycle)
			if e != nil {
				return e
			}
			request.Lifecycle = &value
		}
		result, err = service.RepairExecution(inv.Context, request)
	default:
		request, e := c.relations()
		if e != nil {
			return e
		}
		result, err = service.RepairRelations(inv.Context, request)
	}
	if err != nil {
		return err
	}
	if inv.Output.JSON() {
		return inv.Output.WriteJSON(conflictResolutionOutput{ConflictID: result.ConflictID, Revision: result.Revision})
	}
	return inv.Output.Noticef("Resolved %s at revision %d", result.ConflictID, result.Revision)
}

func (c *resolveCommand) relations() (boardconflict.RelationsRequest, error) {
	request := boardconflict.RelationsRequest{ConflictID: c.ID}
	for _, id := range c.RemoveAssignment {
		request.Remove = append(request.Remove, boardconflict.AssignmentID(id))
	}
	for _, text := range c.Parent {
		child, parent, ok := strings.Cut(text, ":")
		if !ok {
			return request, UsageErrorf("--parent requires CHILD:PARENT")
		}
		childID, err := parseIssueID(child)
		if err != nil {
			return request, err
		}
		parentID, err := parseIssueID(parent)
		if err != nil {
			return request, err
		}
		parentValue := issue.ID(parentID)
		request.Parents = append(request.Parents, boardconflict.ParentSelection{ChildID: issue.ID(childID), ParentID: &parentValue})
	}
	for _, child := range c.Unparent {
		request.Parents = append(request.Parents, boardconflict.ParentSelection{ChildID: issue.ID(child)})
	}
	return request, nil
}
