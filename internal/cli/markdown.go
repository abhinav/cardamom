package cli

import (
	"context"
	"fmt"
	"io"
)

// MarkdownInput selects prose from an argument or standard input without
// changing the Markdown bytes.
type MarkdownInput struct {
	// Context cancels input selection before or after a blocking read.
	Context context.Context // required

	// Stdin is read when the argument is "-" or when input is piped and the
	// argument is omitted.
	Stdin io.Reader // required

	// IsTerminal distinguishes omitted input from an implicit piped value.
	IsTerminal bool
}

// Read returns the selected Markdown and whether the caller supplied a value.
// A nil argument with terminal standard input means no value was supplied.
func (i MarkdownInput) Read(argument *string) (string, bool, error) {
	if argument != nil && *argument != "-" {
		return *argument, true, nil
	}
	if argument == nil && i.IsTerminal {
		return "", false, nil
	}
	if err := i.Context.Err(); err != nil {
		return "", false, err
	}

	value, err := io.ReadAll(i.Stdin)
	if err != nil {
		return "", false, fmt.Errorf("read Markdown from standard input: %w", err)
	}
	if err := i.Context.Err(); err != nil {
		return "", false, err
	}
	return string(value), true, nil
}

// ValidateSingleStdinConsumer rejects a value set that would read standard
// input more than once. Callers pass only optional values that are present.
func (i MarkdownInput) ValidateSingleStdinConsumer(arguments ...*string) error {
	consumers := 0
	for _, argument := range arguments {
		if argument == nil {
			if !i.IsTerminal {
				consumers++
			}
		} else if *argument == "-" {
			consumers++
		}
	}
	if consumers > 1 {
		return UsageErrorf("only one value may read from standard input")
	}
	return nil
}
