package collector

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestReadAllStdin_正常读取 ---
func TestReadAllStdin_正常读取(t *testing.T) {
	data := "hello world"
	r := strings.NewReader(data)
	got, err := ReadAllStdin(r, 1024)
	require.NoError(t, err)
	assert.Equal(t, data, string(got))
}

// --- TestReadAllStdin_空输入 ---
func TestReadAllStdin_空输入(t *testing.T) {
	r := strings.NewReader("")
	got, err := ReadAllStdin(r, 1024)
	require.NoError(t, err)
	assert.Equal(t, "", string(got))
}

// --- TestReadAllStdin_超过限制 ---
func TestReadAllStdin_超过限制(t *testing.T) {
	bigData := strings.Repeat("x", 2000)
	r := strings.NewReader(bigData)
	_, err := ReadAllStdin(r, 100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

// --- TestReadAllStdin_正好等于限制 ---
func TestReadAllStdin_正好等于限制(t *testing.T) {
	data := strings.Repeat("x", 100)
	r := strings.NewReader(data)
	got, err := ReadAllStdin(r, 100)
	require.NoError(t, err)
	assert.Len(t, got, 100)
}

// --- TestNewEnricher_缓存PID和hostname ---
func TestNewEnricher_缓存PID和hostname(t *testing.T) {
	e := NewEnricher()
	assert.NotEmpty(t, e.hostname)
	assert.Greater(t, e.pid, 0)
}

// --- TestEnricher_Enrich_正常PreToolUse ---
func TestEnricher_Enrich_正常PreToolUse(t *testing.T) {
	e := NewEnricher()
	input := &event.HookInput{
		SessionID:     "sess-e1",
		Cwd:           "/home/user",
		HookEventName: "PreToolUse",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "ls -la"},
	}
	evt := e.Enrich(input, "PreToolUse")
	assert.Equal(t, "sess-e1", evt.SessionID)
	assert.Equal(t, "PreToolUse", evt.EventType)
	assert.Equal(t, "Bash", evt.ToolName)
	assert.NotEmpty(t, evt.EventID)
	assert.NotEmpty(t, evt.CreatedAt)
	assert.Contains(t, evt.ToolInput, "command")
}

// --- TestEnricher_Enrich_PostToolUse含tool_output ---
func TestEnricher_Enrich_PostToolUse含tool_output(t *testing.T) {
	e := NewEnricher()
	input := &event.HookInput{
		SessionID:     "sess-e2",
		Cwd:           "/home/user",
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "echo hi"},
		ToolResponse:  map[string]any{"exit_code": 0, "stdout": "hi"},
	}
	evt := e.Enrich(input, "PostToolUse")
	assert.Equal(t, "PostToolUse", evt.EventType)
	assert.NotEmpty(t, evt.ToolOutput)
	assert.Contains(t, evt.ToolOutput, "exit_code")
}

// --- TestEnricher_Enrich_SessionStart无tool字段 ---
func TestEnricher_Enrich_SessionStart无tool字段(t *testing.T) {
	e := NewEnricher()
	input := &event.HookInput{
		SessionID:     "sess-e3",
		Cwd:           "/home/user",
		HookEventName: "SessionStart",
	}
	evt := e.Enrich(input, "SessionStart")
	assert.Equal(t, "SessionStart", evt.EventType)
	assert.Empty(t, evt.ToolName)
	assert.Empty(t, evt.ToolInput)
	assert.Empty(t, evt.ToolOutput)
}

// --- TestEnricher_Enrich_唯一EventID ---
func TestEnricher_Enrich_唯一EventID(t *testing.T) {
	e := NewEnricher()
	input := &event.HookInput{
		SessionID:     "sess-uuid",
		Cwd:           "/home/user",
		HookEventName: "PreToolUse",
		ToolName:      "Bash",
	}
	evt1 := e.Enrich(input, "PreToolUse")
	evt2 := e.Enrich(input, "PreToolUse")
	assert.NotEqual(t, evt1.EventID, evt2.EventID)
}

// --- TestNewSanitizer_默认值 ---
func TestNewSanitizer_默认值(t *testing.T) {
	s := NewSanitizer(10240, 10240)
	assert.Equal(t, 10240, s.maxInputSize)
	assert.Equal(t, 10240, s.maxOutputSize)
	assert.Equal(t, "...[truncated]", s.truncateSuffix)
}

// --- TestSanitizer_Sanitize_无需截断 ---
func TestSanitizer_Sanitize_无需截断(t *testing.T) {
	s := NewSanitizer(100, 100)
	evt := &event.HookEvent{
		ToolInput:  "short input",
		ToolOutput: "short output",
	}
	s.Sanitize(evt)
	assert.Equal(t, "short input", evt.ToolInput)
	assert.Equal(t, "short output", evt.ToolOutput)
}

// --- TestSanitizer_Sanitize_tool_input截断 ---
func TestSanitizer_Sanitize_tool_input截断(t *testing.T) {
	s := NewSanitizer(10, 100)
	evt := &event.HookEvent{
		ToolInput:  strings.Repeat("a", 20),
		ToolOutput: "short",
	}
	s.Sanitize(evt)
	assert.Len(t, evt.ToolInput, 10+len("...[truncated]"))
	assert.Contains(t, evt.ToolInput, "...[truncated]")
	assert.Equal(t, "short", evt.ToolOutput)
}

// --- TestSanitizer_Sanitize_tool_output截断 ---
func TestSanitizer_Sanitize_tool_output截断(t *testing.T) {
	s := NewSanitizer(100, 10)
	evt := &event.HookEvent{
		ToolInput:  "short",
		ToolOutput: strings.Repeat("b", 20),
	}
	s.Sanitize(evt)
	assert.Equal(t, "short", evt.ToolInput)
	assert.Len(t, evt.ToolOutput, 10+len("...[truncated]"))
}

// --- TestSanitizer_Sanitize_两者都截断 ---
func TestSanitizer_Sanitize_两者都截断(t *testing.T) {
	s := NewSanitizer(5, 5)
	evt := &event.HookEvent{
		ToolInput:  "1234567890",
		ToolOutput: "abcdefghij",
	}
	s.Sanitize(evt)
	assert.Contains(t, evt.ToolInput, "...[truncated]")
	assert.Contains(t, evt.ToolOutput, "...[truncated]")
}

// --- TestTruncateString_正常截断 ---
func TestTruncateString_正常截断(t *testing.T) {
	result := TruncateString("hello world", 5, "...")
	assert.Equal(t, "he...", result)
}

// --- TestTruncateString_无需截断 ---
func TestTruncateString_无需截断(t *testing.T) {
	result := TruncateString("hi", 10, "...")
	assert.Equal(t, "hi", result)
}

// --- TestTruncateString_suffix超过maxLen ---
func TestTruncateString_suffix超过maxLen(t *testing.T) {
	result := TruncateString("hello world", 2, "...")
	assert.Equal(t, "he", result)
}

// --- TestFormatDuration_秒 ---
func TestFormatDuration_秒(t *testing.T) {
	assert.Equal(t, "45s", FormatDuration(45*time.Second))
}

// --- TestFormatDuration_分钟 ---
func TestFormatDuration_分钟(t *testing.T) {
	assert.Equal(t, "2m 30s", FormatDuration(150*time.Second))
}

// --- TestFormatDuration_小时 ---
func TestFormatDuration_小时(t *testing.T) {
	assert.Equal(t, "1h 30m 0s", FormatDuration(90*time.Minute))
}

// --- BenchmarkSanitize ---
func BenchmarkSanitize(b *testing.B) {
	s := NewSanitizer(10240, 10240)
	evt := &event.HookEvent{
		ToolInput:  strings.Repeat("a", 20000),
		ToolOutput: strings.Repeat("b", 20000),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evt.ToolInput = strings.Repeat("a", 20000)
		evt.ToolOutput = strings.Repeat("b", 20000)
		s.Sanitize(evt)
	}
}

// --- BenchmarkEnrich ---
func BenchmarkEnrich(b *testing.B) {
	e := NewEnricher()
	input := &event.HookInput{
		SessionID:     "bench-sess",
		Cwd:           "/home/user",
		HookEventName: "PreToolUse",
		ToolName:      "Bash",
		ToolInput:     map[string]any{"command": "npm test"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.Enrich(input, "PreToolUse")
	}
}

// ensure bytes is used
var _ = bytes.NewReader
