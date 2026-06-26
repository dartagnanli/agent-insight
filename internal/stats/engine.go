package stats

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/dartagnanli/agent-insight/pkg/event"
)

// StatsSnapshot 保存某一时刻的统计快照
type StatsSnapshot struct {
	TotalEvents    int            `json:"total_events"`
	TotalSessions  int            `json:"total_sessions"`
	TotalBlocked   int            `json:"total_blocked"`
	BlockRate      float64        `json:"block_rate"`
	AvgHookMs      float64        `json:"avg_hook_duration_ms"`
	P50HookMs      float64        `json:"p50_hook_duration_ms"`
	P95HookMs      float64        `json:"p95_hook_duration_ms"`
	P99HookMs      float64        `json:"p99_hook_duration_ms"`
	ToolDist       map[string]int `json:"tool_distribution"`
	EventTypeDist  map[string]int `json:"event_type_distribution"`
	BlockDist      map[string]int `json:"block_distribution"`
	Durations      []float64      `json:"-"`
	WindowStart    time.Time      `json:"window_start"`
	WindowEnd      time.Time      `json:"window_end"`
}

// Engine 滑动窗口统计引擎
type Engine struct {
	mu       sync.Mutex
	events   []*timedEvent
	sessions map[string]bool
}

type timedEvent struct {
	event    *event.HookEvent
	ingested time.Time
}

// NewEngine 创建统计引擎
func NewEngine() *Engine {
	return &Engine{
		events:   make([]*timedEvent, 0),
		sessions: make(map[string]bool),
	}
}

// Ingest 添加事件到滑动窗口
func (e *Engine) Ingest(evt *event.HookEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, &timedEvent{event: evt, ingested: time.Now()})
	e.sessions[evt.SessionID] = true
}

// Snapshot 返回当前时间窗口的统计快照
func (e *Engine) Snapshot(window time.Duration) *StatsSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()

	cutoff := time.Now().Add(-window)
	var filtered []*event.HookEvent
	for _, te := range e.events {
		if te.ingested.After(cutoff) {
			filtered = append(filtered, te.event)
		}
	}

	snap := &StatsSnapshot{
		TotalEvents:   len(filtered),
		TotalSessions: len(e.sessions),
		ToolDist:      make(map[string]int),
		EventTypeDist: make(map[string]int),
		BlockDist:     make(map[string]int),
		WindowStart:   cutoff,
		WindowEnd:     time.Now(),
	}

	var durations []float64
	for _, evt := range filtered {
		snap.EventTypeDist[evt.EventType]++
		if evt.EventType == "PreToolUse" && evt.ToolName != "" {
			snap.ToolDist[evt.ToolName]++
		}
		if evt.Blocked {
			snap.TotalBlocked++
			if evt.ToolName != "" {
				snap.BlockDist[evt.ToolName]++
			}
		}
		if evt.CollectDurationMs > 0 {
			durations = append(durations, float64(evt.CollectDurationMs))
		}
	}

	if snap.TotalEvents > 0 {
		snap.BlockRate = float64(snap.TotalBlocked) / float64(snap.TotalEvents)
	}

	if len(durations) > 0 {
		sort.Float64s(durations)
		snap.AvgHookMs = average(durations)
		snap.P50HookMs = percentile(durations, 50)
		snap.P95HookMs = percentile(durations, 95)
		snap.P99HookMs = percentile(durations, 99)
		snap.Durations = durations
	}

	return snap
}

// Reset 清空所有事件
func (e *Engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = make([]*timedEvent, 0)
	e.sessions = make(map[string]bool)
}

// ComputeFromEvents 直接从事件列表计算统计，M1 无需内存滑动窗口
func ComputeFromEvents(events []*event.HookEvent) *StatsSnapshot {
	snap := &StatsSnapshot{
		TotalEvents:   len(events),
		ToolDist:      make(map[string]int),
		EventTypeDist: make(map[string]int),
		BlockDist:     make(map[string]int),
	}

	sessions := make(map[string]bool)
	var durations []float64

	for _, evt := range events {
		sessions[evt.SessionID] = true
		snap.EventTypeDist[evt.EventType]++
		if evt.EventType == "PreToolUse" && evt.ToolName != "" {
			snap.ToolDist[evt.ToolName]++
		}
		if evt.Blocked {
			snap.TotalBlocked++
			if evt.ToolName != "" {
				snap.BlockDist[evt.ToolName]++
			}
		}
		if evt.CollectDurationMs > 0 {
			durations = append(durations, float64(evt.CollectDurationMs))
		}
	}

	snap.TotalSessions = len(sessions)
	if snap.TotalEvents > 0 {
		snap.BlockRate = float64(snap.TotalBlocked) / float64(snap.TotalEvents)
	}

	if len(durations) > 0 {
		sort.Float64s(durations)
		snap.AvgHookMs = average(durations)
		snap.P50HookMs = percentile(durations, 50)
		snap.P95HookMs = percentile(durations, 95)
		snap.P99HookMs = percentile(durations, 99)
		snap.Durations = durations
	}

	return snap
}

// ComputeToolStats 计算各工具的统计信息，通过 Pre/Post 事件配对计算实际执行耗时
func ComputeToolStats(events []*event.HookEvent) map[string]*ToolStats {
	result := make(map[string]*ToolStats)

	// 按 CreatedAt 排序确保 Pre 在 Post 之前
	sorted := make([]*event.HookEvent, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt < sorted[j].CreatedAt
	})

	// Pre/Post FIFO 配对，按 session+tool 分组
	type preRef struct{ createdAt string }
	pending := make(map[string][]*preRef)

	for _, evt := range sorted {
		if evt.ToolName == "" {
			continue
		}
		ts, ok := result[evt.ToolName]
		if !ok {
			ts = &ToolStats{Durations: make([]float64, 0)}
			result[evt.ToolName] = ts
		}

		switch evt.EventType {
		case "PreToolUse":
			ts.Count++
			if evt.Blocked {
				ts.Blocked++
			} else {
				key := evt.SessionID + "/" + evt.ToolName
				pending[key] = append(pending[key], &preRef{createdAt: evt.CreatedAt})
			}
		case "PostToolUse":
			key := evt.SessionID + "/" + evt.ToolName
			queue := pending[key]
			if len(queue) > 0 {
				pre := queue[0]
				pending[key] = queue[1:]
				if len(pending[key]) == 0 {
					delete(pending, key)
				}
				dur := calcDuration(pre.createdAt, evt.CreatedAt)
				if dur > 0 {
					ts.Durations = append(ts.Durations, float64(dur))
				}
			}
		}
	}

	for name, ts := range result {
		if len(ts.Durations) > 0 {
			sort.Float64s(ts.Durations)
			ts.AvgMs = average(ts.Durations)
			ts.P99Ms = percentile(ts.Durations, 99)
		}
		result[name] = ts
	}
	return result
}

// ToolStats 单个工具的统计信息
type ToolStats struct {
	Count     int       `json:"count"`
	Blocked   int       `json:"blocked"`
	Durations []float64 `json:"-"`
	AvgMs     float64   `json:"avg_duration_ms"`
	P99Ms     float64   `json:"p99_duration_ms"`
}

// Percentile 计算百分位数（公开接口）
func Percentile(sorted []float64, p int) float64 {
	return percentile(sorted, p)
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
	lower := int(math.Floor(rank))
	upper := lower + 1
	if upper >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
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

func calcDuration(startedAt, endedAt string) int64 {
	t1, err1 := time.Parse("2006-01-02T15:04:05.000Z", startedAt)
	t2, err2 := time.Parse("2006-01-02T15:04:05.000Z", endedAt)
	if err1 != nil || err2 != nil {
		return 0
	}
	return t2.Sub(t1).Milliseconds()
}
