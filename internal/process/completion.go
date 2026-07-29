package process

import (
	"context"
	"os"
	"strings"

	"go.abhg.dev/cardamom/internal/repository/lease"
	"go.abhg.dev/cardamom/internal/repository/mail"
	"go.abhg.dev/komplete"
)

func completionOptions(config *Config) []komplete.Option {
	predictor := func(kind string) komplete.Predictor {
		return komplete.PredictFunc(func(args komplete.Args) []string {
			return predict(config, kind, args.Completed)
		})
	}
	return []komplete.Option{
		komplete.WithPredictor("projects", predictor("projects")),
		komplete.WithPredictor("boards", predictor("boards")),
		komplete.WithPredictor("issues", predictor("issues")),
		komplete.WithPredictor("labels", predictor("labels")),
		komplete.WithPredictor("actor", predictor("actors")),
		komplete.WithPredictor("subscription", predictor("subscriptions")),
		komplete.WithPredictor("lease", predictor("leases")),
	}
}

func predict(config *Config, kind string, completed []string) []string {
	storeSelector, boardSelector := completionSelectors(completed)
	runtime, err := openNamespace(context.Background(), *config, storeSelector)
	if err != nil {
		return nil
	}
	defer func() { _ = runtime.close() }()

	switch kind {
	case "projects":
		projects, err := runtime.projects.List(context.Background())
		if err != nil {
			return nil
		}
		values := make([]string, len(projects))
		for index, value := range projects {
			values[index] = value.ID().String()
		}
		return values
	case "boards":
		boards, err := runtime.boards.List(context.Background())
		if err != nil {
			return nil
		}
		values := make([]string, len(boards))
		for index, value := range boards {
			values[index] = value.ID().String()
		}
		return values
	case "subscriptions":
		repository := mail.New(runtime.store, mail.Config{Clock: config.Clock})
		values, err := repository.ListSubscriptions(context.Background())
		if err != nil {
			return nil
		}
		patterns := make([]string, len(values))
		for index, value := range values {
			patterns[index] = value.Pattern
		}
		return patterns
	case "leases":
		repository := lease.New(runtime.store, config.Clock)
		values, err := repository.List(context.Background())
		if err != nil {
			return nil
		}
		names := make([]string, len(values))
		for index, value := range values {
			names[index] = value.Name
		}
		return names
	}

	board, err := runtime.selectBoard(context.Background(), boardSelector, nil)
	if err != nil {
		return nil
	}
	repository, err := runtime.boardRepository(board.ID())
	if err != nil {
		return nil
	}
	switch kind {
	case "issues":
		values, _ := repository.ListIssueIDs(context.Background())
		return values
	case "labels":
		values, _ := repository.ListLabels(context.Background())
		return values
	case "actors":
		values, _ := repository.ListActors(context.Background())
		return values
	default:
		return nil
	}
}

func completionSelectors(completed []string) (storeSelector, boardSelector string) {
	storeSelector = os.Getenv("CARDAMOM_STORE")
	boardSelector = os.Getenv("CARDAMOM_BOARD")
	for index := 0; index < len(completed); index++ {
		switch value := completed[index]; {
		case value == "--store" && index+1 < len(completed):
			index++
			storeSelector = completed[index]
		case strings.HasPrefix(value, "--store="):
			storeSelector = strings.TrimPrefix(value, "--store=")
		case value == "--board" && index+1 < len(completed):
			index++
			boardSelector = completed[index]
		case strings.HasPrefix(value, "--board="):
			boardSelector = strings.TrimPrefix(value, "--board=")
		}
	}
	return storeSelector, boardSelector
}
