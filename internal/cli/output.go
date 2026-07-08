package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// Output keeps requested data, notices, diagnostics, and structured framing on
// their process streams.
type Output struct {
	stdout io.Writer
	stderr io.Writer
	json   bool
	quiet  bool
}

func newOutput(stdout, stderr io.Writer, jsonMode, quiet bool) *Output {
	return &Output{
		stdout: stdout,
		stderr: stderr,
		json:   jsonMode,
		quiet:  quiet,
	}
}

// Stdout returns the requested-result stream.
func (o *Output) Stdout() io.Writer {
	return o.stdout
}

// Stderr returns the diagnostic stream.
func (o *Output) Stderr() io.Writer {
	return o.stderr
}

// JSON reports whether the invocation requested structured output.
func (o *Output) JSON() bool {
	return o.json
}

// Quiet reports whether human status notices are suppressed.
func (o *Output) Quiet() bool {
	return o.quiet
}

// WriteString writes requested human output to standard output.
func (o *Output) WriteString(value string) error {
	if _, err := io.WriteString(o.stdout, value); err != nil {
		return fmt.Errorf("write standard output: %w", err)
	}
	return nil
}

// Noticef writes one human status notice unless quiet or JSON mode suppresses
// notices.
func (o *Output) Noticef(format string, args ...any) error {
	if o.quiet || o.json {
		return nil
	}
	if _, err := fmt.Fprintf(o.stdout, format+"\n", args...); err != nil {
		return fmt.Errorf("write status notice: %w", err)
	}
	return nil
}

// Errorf writes one diagnostic with the required error prefix.
func (o *Output) Errorf(format string, args ...any) error {
	if _, err := fmt.Fprintf(o.stderr, "error: "+format+"\n", args...); err != nil {
		return fmt.Errorf("write diagnostic: %w", err)
	}
	return nil
}

// WriteJSON writes one JSON value followed by a newline.
func (o *Output) WriteJSON(value any) error {
	if err := json.NewEncoder(o.stdout).Encode(value); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

// WriteJSONLines writes one JSON value per record and no bytes for an empty
// collection.
func WriteJSONLines[T any](output *Output, records []T) error {
	encoder := json.NewEncoder(output.stdout)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("write JSON Lines: %w", err)
		}
	}
	return nil
}
