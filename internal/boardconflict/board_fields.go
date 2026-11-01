package boardconflict

import (
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
)

// BoardField identifies the board setting assigned by this unit.
// Issue and collection units do not name a board setting.
func (u Unit) BoardField() (board.Field, error) {
	switch u.Kind {
	case boardhistory.EventBoardNameSet:
		return board.FieldName, nil
	case boardhistory.EventBoardDescriptionSet:
		return board.FieldDescription, nil
	case boardhistory.EventBoardArchiveSet:
		return board.FieldArchive, nil
	case boardhistory.EventBoardIssueIDPrefixSet:
		return board.FieldIssueIDPrefix, nil
	case boardhistory.EventBoardIssueIDStrategySet:
		return board.FieldIssueIDStrategy, nil
	case boardhistory.EventBoardSummaryLimitSet:
		return board.FieldIssueSummaryMaxBytes, nil
	case boardhistory.EventBoardAttachmentLimitSet:
		return board.FieldAttachmentMaxBytes, nil
	case boardhistory.EventBoardPinLimitSet:
		return board.FieldPinsMaxCount, nil
	default:
		return 0, fmt.Errorf("unsupported board conflict kind %q", u.Kind)
	}
}
