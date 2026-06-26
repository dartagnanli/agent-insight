package stats

import (
	"math"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestComputeFromEvents_基本统计 ---
func TestComputeFromEvents_基本统计(t *testing.T) {
	events := []*event.HookEvent{
		{SessionID: "s1", EventType: "SessionStart", CollectDurationMs: 1},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", CollectDurationMs: 2},
		{SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", CollectDurationMs: 3},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Write", CollectDurationMs: 4, Blocked: true},
		{SessionID: "s2", EventType: "PreToolUse", ToolName: "Bash", CollectDurationMs: 5},
		{SessionID: "s2", EventType: "Stop", CollectDurationMs: 1},
	}

	snap := ComputeFromEvents(events)
	assert.Equal(t, 6, snap.TotalEvents)
	assert.Equal(t, 2, snap.TotalSessions)
	assert.Equal(t, 1, snap.TotalBlocked)
	assert.InDelta(t, 1.0/6.0, snap.BlockRate, 0.001)
	assert.Equal(t, 2, snap.ToolDist["Bash"])
	assert.Equal(t, 1, snap.ToolDist["Write"])
	assert.Equal(t, 3, snap.EventTypeDist["PreToolUse"])
	assert.Equal(t, 1, snap.BlockDist["Write"])
	assert.Greater(t, snap.AvgHookMs, 0.0)
}

// --- TestComputeFromEvents_空事件 ---
func TestComputeFromEvents_空事件(t *testing.T) {
	snap := ComputeFromEvents(nil)
	assert.Equal(t, 0, snap.TotalEvents)
	assert.Equal(t, 0, snap.TotalSessions)
	assert.Equal(t, 0.0, snap.BlockRate)
	assert.Equal(t, 0.0, snap.AvgHookMs)
}

// --- TestComputeToolStats_PrePost配对 ---
func TestComputeToolStats_PrePost配对(t *testing.T) {
	events := []*event.HookEvent{
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", CreatedAt: "2026-01-01T00:00:00.000Z"},
		{SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", CreatedAt: "2026-01-01T00:00:00.010Z"},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", CreatedAt: "2026-01-01T00:00:01.000Z"},
		{SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", CreatedAt: "2026-01-01T00:00:01.020Z"},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", CreatedAt: "2026-01-01T00:00:02.000Z", Blocked: true},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Write", CreatedAt: "2026-01-01T00:00:03.000Z"},
		{SessionID: "s1", EventType: "PostToolUse", ToolName: "Write", CreatedAt: "2026-01-01T00:00:03.005Z"},
	}

	ts := ComputeToolStats(events)
	require.Contains(t, ts, "Bash")
	require.Contains(t, ts, "Write")

	bash := ts["Bash"]
	assert.Equal(t, 3, bash.Count)
	assert.Equal(t, 1, bash.Blocked)
	assert.InDelta(t, 15.0, bash.AvgMs, 0.01) // (10+20)/2
	assert.Greater(t, bash.P99Ms, 19.0)

	write := ts["Write"]
	assert.Equal(t, 1, write.Count)
	assert.Equal(t, 0, write.Blocked)
	assert.InDelta(t, 5.0, write.AvgMs, 0.01)
}

// --- TestComputeToolStats_乱序事件 ---
func TestComputeToolStats_乱序事件(t *testing.T) {
	events := []*event.HookEvent{
		{SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", CreatedAt: "2026-01-01T00:00:00.050Z"},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", CreatedAt: "2026-01-01T00:00:00.000Z"},
	}

	ts := ComputeToolStats(events)
	bash := ts["Bash"]
	assert.Equal(t, 1, bash.Count)
	assert.InDelta(t, 50.0, bash.AvgMs, 0.01)
}

// --- TestPercentile_边界值 ---
func TestPercentile_边界值(t *testing.T) {
	assert.Equal(t, 0.0, Percentile(nil, 50))
	assert.Equal(t, 42.0, Percentile([]float64{42}, 50))
	assert.Equal(t, 1.0, Percentile([]float64{1, 2, 3, 4, 5}, 0))
	assert.Equal(t, 5.0, Percentile([]float64{1, 2, 3, 4, 5}, 100))
}

// --- TestPercentile_线性插值 ---
func TestPercentile_线性插值(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p50 := Percentile(vals, 50)
	assert.InDelta(t, 5.5, p50, 0.01)
}

// --- TestEngine_SlidingWindow ---
func TestEngine_SlidingWindow(t *testing.T) {
	e := NewEngine()

	e.Ingest(&event.HookEvent{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", CollectDurationMs: 10})
	e.Ingest(&event.HookEvent{SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", CollectDurationMs: 5})

	snap := e.Snapshot(1 * time.Hour)
	assert.Equal(t, 2, snap.TotalEvents)
	assert.Equal(t, 1, snap.TotalSessions)
	assert.Equal(t, 1, snap.ToolDist["Bash"])
	assert.InDelta(t, 7.5, snap.AvgHookMs, 0.01)
}

// --- BenchmarkComputeFromEvents ---
func BenchmarkComputeFromEvents(b *testing.B) {
	events := make([]*event.HookEvent, 1000)
	for i := range events {
		events[i] = &event.HookEvent{
			SessionID:        "s1",
			EventType:        "PreToolUse",
			ToolName:         "Bash",
			CollectDurationMs: i,
			CreatedAt:        "2026-01-01T00:00:00.000Z",
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeFromEvents(events)
	}
}

// ensure math import is used
var _ = math.Floor
