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
		{SessionID: "s1", EventType: "SessionStart", HookDurationMs: 1},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", HookDurationMs: 2},
		{SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", HookDurationMs: 3},
		{SessionID: "s1", EventType: "PreToolUse", ToolName: "Write", HookDurationMs: 4, Blocked: true},
		{SessionID: "s2", EventType: "PreToolUse", ToolName: "Bash", HookDurationMs: 5},
		{SessionID: "s2", EventType: "Stop", HookDurationMs: 1},
	}

	snap := ComputeFromEvents(events)
	assert.Equal(t, 6, snap.TotalEvents)
	assert.Equal(t, 2, snap.TotalSessions)
	assert.Equal(t, 1, snap.TotalBlocked)
	assert.InDelta(t, 1.0/6.0, snap.BlockRate, 0.001)
	assert.Equal(t, 3, snap.ToolDist["Bash"])
	assert.Equal(t, 1, snap.ToolDist["Write"])
	assert.Equal(t, 3, snap.EventTypeDist["PreToolUse"])
	assert.Equal(t, 1, snap.BlockDist["Write"])
}

// --- TestComputeFromEvents_空事件 ---
func TestComputeFromEvents_空事件(t *testing.T) {
	snap := ComputeFromEvents(nil)
	assert.Equal(t, 0, snap.TotalEvents)
	assert.Equal(t, 0, snap.TotalSessions)
	assert.Equal(t, 0.0, snap.BlockRate)
	assert.Equal(t, 0.0, snap.AvgHookMs)
}

// --- TestComputeToolStats_工具统计 ---
func TestComputeToolStats_工具统计(t *testing.T) {
	events := []*event.HookEvent{
		{ToolName: "Bash", HookDurationMs: 10},
		{ToolName: "Bash", HookDurationMs: 20},
		{ToolName: "Bash", HookDurationMs: 30, Blocked: true},
		{ToolName: "Write", HookDurationMs: 5},
	}

	ts := ComputeToolStats(events)
	require.Contains(t, ts, "Bash")
	require.Contains(t, ts, "Write")

	bash := ts["Bash"]
	assert.Equal(t, 3, bash.Count)
	assert.Equal(t, 1, bash.Blocked)
	assert.InDelta(t, 20.0, bash.AvgMs, 0.01)
	// p99 of [10,20,30] is 30 * (0.99*2 - 1) + 30*(1 - (0.99*2 - 1)) ≈ 29.8
	assert.Greater(t, bash.P99Ms, 28.0)

	write := ts["Write"]
	assert.Equal(t, 1, write.Count)
	assert.Equal(t, 0, write.Blocked)
	assert.InDelta(t, 5.0, write.AvgMs, 0.01)
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

	e.Ingest(&event.HookEvent{SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", HookDurationMs: 10})
	e.Ingest(&event.HookEvent{SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", HookDurationMs: 5})

	snap := e.Snapshot(1 * time.Hour) // 1h window
	assert.Equal(t, 2, snap.TotalEvents)
	assert.Equal(t, 1, snap.TotalSessions)
}

// --- BenchmarkComputeFromEvents ---
func BenchmarkComputeFromEvents(b *testing.B) {
	events := make([]*event.HookEvent, 1000)
	for i := range events {
		events[i] = &event.HookEvent{
			SessionID:     "s1",
			EventType:     "PreToolUse",
			ToolName:      "Bash",
			HookDurationMs: i,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeFromEvents(events)
	}
}

// ensure math import is used
var _ = math.Floor
