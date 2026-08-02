package cli

import "encoding/json"

const cardamomHookContext = "Load and follow the Cardamom skill for work in this checkout."

type hookCommand struct {
	Context hookContextCommand `cmd:"" help:"Emit Cardamom context for a lifecycle event."`
}

// HookContextOperation determines whether the current checkout should receive
// Cardamom lifecycle context.
type HookContextOperation interface {
	// Associated reports whether local checkout markers associate the current
	// working directory with Cardamom.
	Associated() bool
}

type hookContextCommand struct{}

type hookContextOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	Event   string `json:"hookEventName"`
	Context string `json:"additionalContext"`
}

func (*hookContextCommand) Run(
	invocation *Invocation,
	operation HookContextOperation,
) error {
	var input struct {
		Event string `json:"hook_event_name"`
	}
	if err := json.NewDecoder(invocation.Stdin).Decode(&input); err != nil {
		return nil
	}
	switch input.Event {
	case "SessionStart", "SubagentStart":
	default:
		return nil
	}
	if !operation.Associated() {
		return nil
	}

	return invocation.Output.WriteJSON(hookContextOutput{
		HookSpecificOutput: hookSpecificOutput{
			Event: input.Event, Context: cardamomHookContext,
		},
	})
}
