package hookinput

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/libin18/agent-insight/pkg/event"
)

// ParseHookInput reads JSON from an io.Reader and parses it into a HookInput.
func ParseHookInput(r io.Reader) (*event.HookInput, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return ParseHookInputFromBytes(data)
}

// ParseHookInputFromBytes parses a JSON byte slice into a HookInput.
func ParseHookInputFromBytes(data []byte) (*event.HookInput, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	if len(data) > 1*1024*1024 {
		return nil, fmt.Errorf("stdin payload too large: %d bytes (max 1MB)", len(data))
	}
	var input event.HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return &input, nil
}

// Validate checks that required fields are present.
func Validate(input *event.HookInput) error {
	if input.SessionID == "" {
		return fmt.Errorf("missing required field: session_id")
	}
	if input.Cwd == "" {
		return fmt.Errorf("missing required field: cwd")
	}
	if input.HookEventName == "" {
		return fmt.Errorf("missing required field: hook_event_name")
	}
	return nil
}
