package process

import (
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardconflict"
)

func provideConflicts(runtime *namespaceRuntime, selected *board.State) (*boardconflict.Service, error) {
	return (&conflictServiceFactory{runtime: runtime}).Conflicts(selected.ID())
}

// conflictServiceFactory preserves board scope for CLI and HTTP operations.
type conflictServiceFactory struct{ runtime *namespaceRuntime }

func (f *conflictServiceFactory) Conflicts(id board.ID) (*boardconflict.Service, error) {
	repository, err := f.runtime.boardRepository(id)
	if err != nil {
		return nil, err
	}
	return &boardconflict.Service{Repository: repository, Configuration: f.runtime.configuration}, nil
}
