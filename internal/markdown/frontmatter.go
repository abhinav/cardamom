package markdown

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/goccy/go-yaml"
)

const (
	frontmatterFence         = "---\n"
	frontmatterBodySeparator = "\n"
)

// EncodeFrontmatter encodes metadata as YAML frontmatter followed by body.
func EncodeFrontmatter(metadata any, body []byte) ([]byte, error) {
	payload, err := yaml.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode YAML frontmatter: %w", err)
	}
	payload = bytes.TrimRight(payload, "\n")
	result := make([]byte, 0, 2*len(frontmatterFence)+len(payload)+1+len(frontmatterBodySeparator)+len(body))
	result = append(result, frontmatterFence...)
	result = append(result, payload...)
	result = append(result, '\n')
	result = append(result, frontmatterFence...)
	result = append(result, frontmatterBodySeparator...)
	result = append(result, body...)
	return result, nil
}

// DecodeFrontmatter parses YAML frontmatter into destination and returns the
// authored Markdown body that follows it. Found is false when source has no
// frontmatter.
func DecodeFrontmatter(source []byte, destination any) (body []byte, found bool, err error) {
	bodyReader, found, err := DecodeFrontmatterReader(bytes.NewReader(source), destination)
	if err != nil || !found {
		return nil, found, err
	}
	body, err = io.ReadAll(bodyReader)
	if err != nil {
		return nil, true, fmt.Errorf("read Markdown body: %w", err)
	}
	return body, true, nil
}

// DecodeFrontmatterReader parses YAML frontmatter from source and returns the
// remaining Markdown body without retaining that body in memory.
//
// Body is nil when source has no frontmatter.
// DecodeFrontmatterReader does not close source. The source owner retains close
// ownership through body consumption.
func DecodeFrontmatterReader(
	source io.Reader,
	destination any,
) (body io.Reader, found bool, err error) {
	buffered := bufio.NewReader(source)
	line, err := buffered.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, fmt.Errorf("read YAML frontmatter: %w", err)
	}
	if line != frontmatterFence {
		return nil, false, nil
	}

	var payload bytes.Buffer
	for {
		line, err = buffered.ReadString('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, true, fmt.Errorf("read YAML frontmatter: %w", err)
			}
			return nil, true, errors.New("frontmatter is malformed")
		}
		if line == frontmatterFence {
			break
		}
		payload.WriteString(line)
	}
	line, err = buffered.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, true, fmt.Errorf("read YAML frontmatter: %w", err)
	}
	if line != frontmatterBodySeparator {
		return nil, true, errors.New("frontmatter is not followed by a blank line")
	}
	if err := yaml.Unmarshal(payload.Bytes(), destination); err != nil {
		return nil, true, fmt.Errorf("decode YAML frontmatter: %w", err)
	}
	return buffered, true, nil
}
