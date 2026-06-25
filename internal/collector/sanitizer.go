package collector

import (
	"github.com/dartagnanli/agent-insight/pkg/event"
)

// Sanitizer 截断过大的 tool_input 和 tool_output
type Sanitizer struct {
	maxInputSize   int
	maxOutputSize  int
	truncateSuffix string
}

// NewSanitizer 创建 Sanitizer
func NewSanitizer(maxInputSize, maxOutputSize int) *Sanitizer {
	return &Sanitizer{
		maxInputSize:   maxInputSize,
		maxOutputSize:  maxOutputSize,
		truncateSuffix: "...[truncated]",
	}
}

// Sanitize 对 HookEvent 执行截断
// 截断时保留 maxLen 字节原始内容，再追加后缀
func (s *Sanitizer) Sanitize(evt *event.HookEvent) {
	if len(evt.ToolInput) > s.maxInputSize {
		evt.ToolInput = evt.ToolInput[:s.maxInputSize] + s.truncateSuffix
	}
	if len(evt.ToolOutput) > s.maxOutputSize {
		evt.ToolOutput = evt.ToolOutput[:s.maxOutputSize] + s.truncateSuffix
	}
}

// TruncateString 截断字符串，使总长度（含 suffix）不超过 maxLen
func TruncateString(s string, maxLen int, suffix string) string {
	if len(s) <= maxLen {
		return s
	}
	usable := maxLen - len(suffix)
	if usable <= 0 {
		return s[:maxLen]
	}
	return s[:usable] + suffix
}
