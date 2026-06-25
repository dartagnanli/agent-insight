package session

import (
	"context"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestTracker_生命周期 ---
func TestTracker_生命周期(t *testing.T) {
	tracker := NewTracker(300)
	ctx := context.Background()

	// Track SessionStart
	err := tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-ss",
		SessionID: "sess-lifecycle",
		EventType: "SessionStart",
		Cwd:       "/home/user",
		Hostname:  "host",
		CreatedAt: "2026-06-18T10:00:00.000Z",
	})
	require.NoError(t, err)

	// Track PreToolUse
	err = tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-pre",
		SessionID: "sess-lifecycle",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		ToolInput: `{"command":"ls"}`,
		Cwd:       "/home/user",
		Hostname:  "host",
		CreatedAt: "2026-06-18T10:00:01.000Z",
	})
	require.NoError(t, err)

	// Track PostToolUse
	err = tracker.Track(ctx, &event.HookEvent{
		EventID:    "e-post",
		SessionID:  "sess-lifecycle",
		EventType:  "PostToolUse",
		ToolName:   "Bash",
		ToolOutput: `{"exit_code":0}`,
		Cwd:        "/home/user",
		Hostname:   "host",
		CreatedAt:  "2026-06-18T10:00:03.000Z",
	})
	require.NoError(t, err)

	// Track Stop
	err = tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-stop",
		SessionID: "sess-lifecycle",
		EventType: "Stop",
		Cwd:       "/home/user",
		Hostname:  "host",
		CreatedAt: "2026-06-18T10:05:00.000Z",
	})
	require.NoError(t, err)

	// Aggregate
	row, err := tracker.Aggregate("sess-lifecycle")
	require.NoError(t, err)
	assert.Equal(t, "sess-lifecycle", row.SessionID)
	assert.Equal(t, 4, row.TotalEvents)
	assert.Equal(t, 1, row.ToolCalls)
	assert.Equal(t, 0, row.BlockedCalls)
	assert.Equal(t, int64(300), row.DurationSecs) // 5min - start
	require.NotNil(t, row.EndedAt)
	assert.Equal(t, "2026-06-18T10:05:00.000Z", *row.EndedAt)
}

// --- TestTracker_聚合计算 ---
func TestTracker_聚合计算(t *testing.T) {
	tracker := NewTracker(300)
	ctx := context.Background()

	events := []*event.HookEvent{
		{EventID: "e1", SessionID: "sess-agg", EventType: "PreToolUse", ToolName: "Bash", HookDurationMs: 10, CreatedAt: event.Now()},
		{EventID: "e2", SessionID: "sess-agg", EventType: "PreToolUse", ToolName: "Bash", HookDurationMs: 20, CreatedAt: event.Now()},
		{EventID: "e3", SessionID: "sess-agg", EventType: "PreToolUse", ToolName: "Write", HookDurationMs: 5, Blocked: true, CreatedAt: event.Now()},
		{EventID: "e4", SessionID: "sess-agg", EventType: "PreToolUse", ToolName: "Write", HookDurationMs: 8, CreatedAt: event.Now()},
	}

	for _, evt := range events {
		require.NoError(t, tracker.Track(ctx, evt))
	}

	row, err := tracker.Aggregate("sess-agg")
	require.NoError(t, err)

	assert.Equal(t, 4, row.TotalEvents)
	assert.Equal(t, 4, row.ToolCalls) // 4 PreToolUse events
	assert.Equal(t, 1, row.BlockedCalls)
	assert.InDelta(t, 0.25, row.BlockRate, 0.01)
	assert.Contains(t, row.ToolsUsed, "Bash")
	assert.Contains(t, row.ToolsUsed, "Write")
}

// --- TestTracker_不存在session ---
func TestTracker_不存在session(t *testing.T) {
	tracker := NewTracker(300)
	_, err := tracker.Aggregate("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

// --- TestTracker_超时标记 ---
func TestTracker_超时标记(t *testing.T) {
	tracker := NewTracker(1) // 1 second timeout

	ctx := context.Background()
	require.NoError(t, tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-timeout",
		SessionID: "sess-timeout",
		EventType: "SessionStart",
		Cwd:      "/home/user",
		Hostname: "host",
		CreatedAt: "2026-06-18T10:00:00.000Z",
	}))

	// Wait for timeout
	time.Sleep(1500 * time.Millisecond)
	tracker.CheckTimeouts()

	row, err := tracker.Aggregate("sess-timeout")
	require.NoError(t, err)
	require.NotNil(t, row.EndedAt)
}

// --- TestTracker_ListSessions ---
func TestTracker_ListSessions(t *testing.T) {
	tracker := NewTracker(300)
	ctx := context.Background()

	for _, sid := range []string{"sess-a", "sess-b", "sess-c"} {
		require.NoError(t, tracker.Track(ctx, &event.HookEvent{
			EventID:   "e-" + sid,
			SessionID: sid,
			EventType: "SessionStart",
			Cwd:      "/home/user",
			Hostname: "host",
			CreatedAt: event.Now(),
		}))
	}

	sessions := tracker.ListSessions()
	assert.Len(t, sessions, 3)

	sessionIDs := make(map[string]bool)
	for _, s := range sessions {
		sessionIDs[s.SessionID] = true
	}
	assert.True(t, sessionIDs["sess-a"])
	assert.True(t, sessionIDs["sess-b"])
	assert.True(t, sessionIDs["sess-c"])
}

// --- TestTracker_Stop事件结束会话 ---
func TestTracker_Stop事件结束会话(t *testing.T) {
	tracker := NewTracker(300)
	ctx := context.Background()

	require.NoError(t, tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-start",
		SessionID: "sess-end",
		EventType: "SessionStart",
		Cwd:      "/home/user",
		Hostname: "host",
		CreatedAt: "2026-06-18T10:00:00.000Z",
	}))

	require.NoError(t, tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-stop",
		SessionID: "sess-end",
		EventType: "Stop",
		Cwd:      "/home/user",
		Hostname: "host",
		CreatedAt: "2026-06-18T10:10:00.000Z",
	}))

	row, err := tracker.Aggregate("sess-end")
	require.NoError(t, err)
	require.NotNil(t, row.EndedAt)
	assert.Equal(t, "2026-06-18T10:10:00.000Z", *row.EndedAt)
	assert.Equal(t, int64(600), row.DurationSecs)
}

// --- TestTracker_SubagentStop事件 ---
func TestTracker_SubagentStop事件(t *testing.T) {
	tracker := NewTracker(300)
	ctx := context.Background()

	require.NoError(t, tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-start",
		SessionID: "sess-sub",
		EventType: "SessionStart",
		Cwd:      "/home/user",
		Hostname: "host",
		CreatedAt: "2026-06-18T10:00:00.000Z",
	}))

	require.NoError(t, tracker.Track(ctx, &event.HookEvent{
		EventID:   "e-substop",
		SessionID: "sess-sub",
		EventType: "SubagentStop",
		Cwd:      "/home/user",
		Hostname: "host",
		CreatedAt: "2026-06-18T10:03:00.000Z",
	}))

	row, err := tracker.Aggregate("sess-sub")
	require.NoError(t, err)
	require.NotNil(t, row.EndedAt)
	assert.Equal(t, int64(180), row.DurationSecs)
}

// --- TestTracker_并发安全 ---
func TestTracker_并发安全(t *testing.T) {
	tracker := NewTracker(300)
	ctx := context.Background()

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			_ = tracker.Track(ctx, &event.HookEvent{
				EventID:   "e-concurrent",
				SessionID: "sess-concurrent",
				EventType: "PreToolUse",
				ToolName:  "Bash",
				Cwd:       "/home/user",
				Hostname:  "host",
				CreatedAt: event.Now(),
			})
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}

	row, err := tracker.Aggregate("sess-concurrent")
	require.NoError(t, err)
	assert.Equal(t, 50, row.TotalEvents)
}
