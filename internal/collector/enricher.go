package collector

import (
	"encoding/json"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/libin18/agent-insight/pkg/event"
)

// Enricher 为 hook 事件补充元数据（UUID、PID、hostname、timestamp）
type Enricher struct {
	hostname string
	pid      int
}

// NewEnricher 创建 Enricher，缓存 hostname 和 PID
func NewEnricher() *Enricher {
	hostname, _ := os.Hostname()
	// 获取父进程 PID 即 Claude Code 进程
	pid := os.Getppid()
	return &Enricher{hostname: hostname, pid: pid}
}

// Enrich 将 HookInput 转为 HookEvent，补充元数据
func (e *Enricher) Enrich(input *event.HookInput, eventType string) *event.HookEvent {
	evt := &event.HookEvent{
		EventID:   uuid.New().String(),
		SessionID: input.SessionID,
		EventType: eventType,
		Cwd:       input.Cwd,
		Pid:       e.pid,
		Hostname:  e.hostname,
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}

	// 工具相关字段
	if input.ToolName != "" {
		evt.ToolName = input.ToolName
	}
	if input.ToolInput != nil {
		evt.ToolInput = mustMarshalJSON(input.ToolInput)
	}
	if input.ToolResponse != nil {
		evt.ToolOutput = mustMarshalJSON(input.ToolResponse)
	}
	if input.TranscriptPath != "" {
		evt.TranscriptPath = input.TranscriptPath
	}

	// 判断是否被拦截：exit code=2 且为 PreToolUse
	if ec := os.Getenv("CLAUDE_EXIT_CODE"); ec == "2" && eventType == "PreToolUse" {
		evt.Blocked = true
	}

	return evt
}

// mustMarshalJSON 序列化为 JSON，失败返回空字符串
func mustMarshalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
