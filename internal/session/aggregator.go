package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/pkg/event"
)

// Aggregator 会话指标聚合器，将 session 统计数据写入 session_stats 表
type Aggregator struct {
	mu      sync.Mutex
	storage *storage.SQLite
}

// NewAggregator 创建聚合器
func NewAggregator(s *storage.SQLite) *Aggregator {
	return &Aggregator{storage: s}
}

// AggregateFromEvents 从事件列表直接聚合 session 统计，用于 M1
func (a *Aggregator) AggregateFromEvents(ctx context.Context, events []*event.HookEvent) error {
	if len(events) == 0 {
		return fmt.Errorf("no events to aggregate")
	}

	sessionID := events[0].SessionID
	row := &storage.SessionStatsRow{
		SessionID:   sessionID,
		StartedAt:   events[0].CreatedAt,
		TotalEvents: len(events),
	}

	// 计算工具调用和拦截
	var toolCalls, blockedCalls int
	toolSet := make(map[string]bool)
	var toolDurations []float64

	for _, evt := range events {
		if evt.EventType == "PreToolUse" {
			toolCalls++
			if evt.Blocked {
				blockedCalls++
			}
			toolSet[evt.ToolName] = true
		}
		if evt.HookDurationMs > 0 {
			toolDurations = append(toolDurations, float64(evt.HookDurationMs))
		}
		if evt.EventType == "Stop" || evt.EventType == "SubagentStop" {
			endedAt := evt.CreatedAt
			row.EndedAt = &endedAt
		}
	}

	row.ToolCalls = toolCalls
	row.BlockedCalls = blockedCalls
	if toolCalls > 0 {
		row.BlockRate = float64(blockedCalls) / float64(toolCalls)
	}

	// 构建 tools_used JSON
	tools := make([]string, 0, len(toolSet))
	for t := range toolSet {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	toolsJSON, _ := json.Marshal(tools)
	row.ToolsUsed = string(toolsJSON)

	// 计算工具耗时统计
	if len(toolDurations) > 0 {
		sort.Float64s(toolDurations)
		avg := average(toolDurations)
		p99 := percentile(toolDurations, 99)
		row.AvgToolDurationMs = &avg
		row.P99ToolDurationMs = &p99
	}

	// 计算会话时长
	if row.StartedAt != "" && row.EndedAt != nil {
		t1, _ := time.Parse("2006-01-02T15:04:05.000Z", row.StartedAt)
		t2, _ := time.Parse("2006-01-02T15:04:05.000Z", *row.EndedAt)
		if !t1.IsZero() && !t2.IsZero() {
			row.DurationSecs = int64(t2.Sub(t1).Seconds())
		}
	}

	// 项目路径
	if len(events) > 0 {
		pp := events[0].Cwd
		row.ProjectPath = &pp
	}

	if err := a.storage.UpsertSessionStats(ctx, row); err != nil {
		slog.Warn("failed to upsert session stats", "session_id", sessionID, "error", err)
		return err
	}

	return nil
}

// AggregateAllSessions 聚合所有未聚合的 session（用于启动时补全）
func (a *Aggregator) AggregateAllSessions(ctx context.Context, store storage.Storage) error {
	// 查询所有 session
	filter := storage.SessionFilter{Limit: 1000}
	sessions, err := store.ListSessions(ctx, filter)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	for _, sm := range sessions {
		// 查询该 session 的所有事件
		sid := sm.SessionID
		evtFilter := storage.EventFilter{SessionID: &sid, Limit: 10000}
		events, err := store.QueryEvents(ctx, evtFilter)
		if err != nil {
			slog.Warn("failed to query events for session", "session_id", sid, "error", err)
			continue
		}
		if len(events) == 0 {
			continue
		}
		if err := a.AggregateFromEvents(ctx, events); err != nil {
			slog.Warn("failed to aggregate session", "session_id", sid, "error", err)
		}
	}

	return nil
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func percentile(sorted []float64, p int) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	rank := float64(p) / 100.0 * float64(n-1)
	lower := int(rank)
	upper := lower + 1
	if upper >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}
