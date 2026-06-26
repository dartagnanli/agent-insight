package dashboard

import (
	"context"
	"log/slog"
	"time"

	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/pkg/event"
)

// Poller 轮询数据库获取新事件并广播到 Hub
type Poller struct {
	storage   storage.Storage
	hub       *Hub
	interval  time.Duration
	lastPollID int64
}

// NewPoller 创建轮询器
func NewPoller(s storage.Storage, h *Hub) *Poller {
	return &Poller{
		storage:  s,
		hub:      h,
		interval: 1 * time.Second,
	}
}

// Run 启动轮询循环
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				slog.Warn("poller error", "error", err)
			}
		}
	}
}

func (p *Poller) poll(ctx context.Context) error {
	events, err := p.storage.QueryRecentEvents(ctx, p.lastPollID, 1000)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	for _, ev := range events {
		p.hub.Broadcast(&WSMessage{
			Type:    "event",
			Payload: eventToMap(ev),
		})
		if ev.ID > p.lastPollID {
			p.lastPollID = ev.ID
		}
	}
	return nil
}

// eventToMap 将 HookEvent 转为 map 用于 JSON 序列化
func eventToMap(ev *event.HookEvent) map[string]any {
	return map[string]any{
		"id":                 ev.ID,
		"event_id":           ev.EventID,
		"session_id":         ev.SessionID,
		"event_type":         ev.EventType,
		"tool_name":          ev.ToolName,
		"tool_input":         ev.ToolInput,
		"tool_output":        ev.ToolOutput,
		"cwd":                ev.Cwd,
		"transcript_path":    ev.TranscriptPath,
		"blocked":            ev.Blocked,
		"block_reason":       ev.BlockReason,
		"hook_exit_code":     ev.HookExitCode,
		"hook_duration_ms":   ev.HookDurationMs,
		"collect_duration_ms": ev.CollectDurationMs,
		"pid":                ev.Pid,
		"hostname":           ev.Hostname,
		"created_at":         ev.CreatedAt,
	}
}
