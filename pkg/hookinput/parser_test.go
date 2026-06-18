package hookinput

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/libin18/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestParseHookInput_正常解析 ---
func TestParseHookInput_正常解析(t *testing.T) {
	data := `{"session_id":"s1","cwd":"/tmp","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`
	r := strings.NewReader(data)
	input, err := ParseHookInput(r)
	require.NoError(t, err)
	assert.Equal(t, "s1", input.SessionID)
	assert.Equal(t, "/tmp", input.Cwd)
	assert.Equal(t, "PreToolUse", input.HookEventName)
	assert.Equal(t, "Bash", input.ToolName)
}

// --- TestParseHookInput_空输入 ---
func TestParseHookInput_空输入(t *testing.T) {
	r := strings.NewReader("")
	_, err := ParseHookInput(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty input")
}

// --- TestParseHookInput_无效JSON ---
func TestParseHookInput_无效JSON(t *testing.T) {
	r := strings.NewReader("{invalid}")
	_, err := ParseHookInput(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse JSON")
}

// --- TestParseHookInput_超大输入 ---
func TestParseHookInput_超大输入(t *testing.T) {
	bigData := strings.Repeat("x", 2*1024*1024)
	r := strings.NewReader(bigData)
	_, err := ParseHookInput(r)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

// --- TestParseHookInputFromBytes_正常解析 ---
func TestParseHookInputFromBytes_正常解析(t *testing.T) {
	data := []byte(`{"session_id":"s2","cwd":"/home","hook_event_name":"SessionStart"}`)
	input, err := ParseHookInputFromBytes(data)
	require.NoError(t, err)
	assert.Equal(t, "s2", input.SessionID)
	assert.Equal(t, "SessionStart", input.HookEventName)
}

// --- TestParseHookInputFromBytes_空输入 ---
func TestParseHookInputFromBytes_空输入(t *testing.T) {
	_, err := ParseHookInputFromBytes([]byte{})
	assert.Error(t, err)
}

// --- TestValidate_缺少session_id ---
func TestValidate_缺少session_id(t *testing.T) {
	input := &event.HookInput{Cwd: "/tmp", HookEventName: "PreToolUse"}
	err := Validate(input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session_id")
}

// --- TestValidate_缺少cwd ---
func TestValidate_缺少cwd(t *testing.T) {
	input := &event.HookInput{SessionID: "s1", HookEventName: "PreToolUse"}
	err := Validate(input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cwd")
}

// --- TestValidate_缺少hook_event_name ---
func TestValidate_缺少hook_event_name(t *testing.T) {
	input := &event.HookInput{SessionID: "s1", Cwd: "/tmp"}
	err := Validate(input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hook_event_name")
}

// --- TestValidate_全部字段合法 ---
func TestValidate_全部字段合法(t *testing.T) {
	input := &event.HookInput{SessionID: "s1", Cwd: "/tmp", HookEventName: "PreToolUse"}
	err := Validate(input)
	assert.NoError(t, err)
}

// --- TestParseHookInput_从文件固件 ---
func TestParseHookInput_从文件固件(t *testing.T) {
	testdataDir := "../../test/testdata"

	tests := []struct {
		name     string
		file     string
		wantType string
		wantTool string
	}{
		{"SessionStart事件", "session_start.json", "SessionStart", ""},
		{"PreToolUse_Bash事件", "pre_tool_use_bash.json", "PreToolUse", "Bash"},
		{"PostToolUse_Bash事件", "post_tool_use_bash.json", "PostToolUse", "Bash"},
		{"PreToolUse_Write_blocked事件", "pre_tool_use_write_blocked.json", "PreToolUse", "Write"},
		{"Stop事件", "stop.json", "Stop", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(testdataDir + "/" + tt.file)
			require.NoError(t, err)
			input, err := ParseHookInputFromBytes(data)
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, input.HookEventName)
			assert.Equal(t, tt.wantTool, input.ToolName)
		})
	}
}

// --- TestParseHookInput_无效JSON文件 ---
func TestParseHookInput_无效JSON文件(t *testing.T) {
	data, err := os.ReadFile("../../test/testdata/invalid_json.txt")
	require.NoError(t, err)
	_, err = ParseHookInputFromBytes(data)
	assert.Error(t, err)
}

// --- TestParseHookInput_超大输入文件 ---
func TestParseHookInput_超大输入文件(t *testing.T) {
	data, err := os.ReadFile("../../test/testdata/oversized_tool_input.json")
	require.NoError(t, err)
	input, err := ParseHookInputFromBytes(data)
	require.NoError(t, err)
	// tool_input 中的 content 超过 10KB，验证能被解析但后续会被 sanitizer 截断
	assert.Equal(t, "sess-biginput", input.SessionID)
	toolInputMap, ok := input.ToolInput.(map[string]any)
	require.True(t, ok)
	content, _ := toolInputMap["content"].(string)
	assert.Greater(t, len(content), 10240)
}

// --- TestParseHookInput_保留未知字段 ---
func TestParseHookInput_保留未知字段(t *testing.T) {
	data := []byte(`{"session_id":"s1","cwd":"/tmp","hook_event_name":"PreToolUse","unknown_field":"value"}`)
	input, err := ParseHookInputFromBytes(data)
	require.NoError(t, err)
	assert.Equal(t, "s1", input.SessionID)
	// 未知字段不影响解析
}

// --- TestParseHookInputFromBytes_tool_response解析 ---
func TestParseHookInputFromBytes_tool_response解析(t *testing.T) {
	data := []byte(`{
		"session_id":"s1",
		"cwd":"/tmp",
		"hook_event_name":"PostToolUse",
		"tool_name":"Bash",
		"tool_input":{"command":"echo hi"},
		"tool_response":{"exit_code":0,"stdout":"hi\n"}
	}`)
	input, err := ParseHookInputFromBytes(data)
	require.NoError(t, err)
	assert.Equal(t, "PostToolUse", input.HookEventName)
	resp, ok := input.ToolResponse.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(0), resp["exit_code"])
}

// --- Benchmark ---
func BenchmarkParseHookInput(b *testing.B) {
	data := []byte(`{"session_id":"s1","cwd":"/tmp","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"npm test"}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, _ = ParseHookInput(r)
	}
}

// ensure json import is used
var _ = json.Marshal
