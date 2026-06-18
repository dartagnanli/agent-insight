package event

import (
	"fmt"
	"time"
)

// HookEvent 对应 hook_events 表的一行记录
type HookEvent struct {
	ID                int64  `json:"id" db:"id"`
	EventID           string `json:"event_id" db:"event_id"`
	SessionID         string `json:"session_id" db:"session_id"`
	EventType         string `json:"event_type" db:"event_type"`
	ToolName          string `json:"tool_name" db:"tool_name"`
	ToolInput         string `json:"tool_input" db:"tool_input"`
	ToolOutput        string `json:"tool_output" db:"tool_output"`
	Cwd               string `json:"cwd" db:"cwd"`
	TranscriptPath    string `json:"transcript_path" db:"transcript_path"`
	Blocked           bool   `json:"blocked" db:"blocked"`
	BlockReason       string `json:"block_reason" db:"block_reason"`
	HookExitCode      int    `json:"hook_exit_code" db:"hook_exit_code"`
	HookDurationMs    int    `json:"hook_duration_ms" db:"hook_duration_ms"`
	CollectDurationMs int    `json:"collect_duration_ms" db:"collect_duration_ms"`
	Pid               int    `json:"pid" db:"pid"`
	Hostname          string `json:"hostname" db:"hostname"`
	CreatedAt         string `json:"created_at" db:"created_at"`
}

// HookInput 对应 Claude Code 通过 stdin 传入的 JSON
type HookInput struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	ToolInput      any    `json:"tool_input,omitempty"`
	ToolResponse   any    `json:"tool_response,omitempty"`
}

// Span 表示配对后的完整工具调用
type Span struct {
	SpanID      string `json:"span_id"`
	ToolName    string `json:"tool_name"`
	ToolInput   string `json:"tool_input"`
	ToolOutput  string `json:"tool_output,omitempty"`
	StartedAt   string `json:"started_at"`
	EndedAt     string `json:"ended_at,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	Blocked     bool   `json:"blocked"`
	BlockReason string `json:"block_reason,omitempty"`
	Orphan      bool   `json:"orphan"`
	PreEventID  string `json:"pre_event_id"`
	PostEventID string `json:"post_event_id,omitempty"`
}

// Trace 表示一次会话的完整调用链
type Trace struct {
	SessionID        string             `json:"session_id"`
	StartedAt        string             `json:"started_at"`
	EndedAt          string             `json:"ended_at,omitempty"`
	DurationSecs     int64              `json:"duration_secs"`
	TotalEvents      int                `json:"total_events"`
	Spans            []*Span            `json:"spans"`
	StandaloneEvents []*StandaloneEvent `json:"standalone_events"`
}

// StandaloneEvent 非工具调用的事件（SessionStart/Stop 等）
type StandaloneEvent struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	CreatedAt string `json:"created_at"`
}

// Now 返回当前 UTC 时间 ISO 8601 格式（毫秒精度）
func Now() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// FormatDuration 将秒数格式化为人类可读字符串
func FormatDuration(secs int64) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	remSecs := secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, remSecs)
	}
	hrs := mins / 60
	remMins := mins % 60
	return fmt.Sprintf("%dh %dm %ds", hrs, remMins, remSecs)
}
