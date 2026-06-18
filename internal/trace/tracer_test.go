package trace

import (
	"context"
	"testing"

	"github.com/libin18/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestTracer_PrePost配对 ---
func TestTracer_PrePost配对(t *testing.T) {
	tracer := NewTracer(30)
	ctx := context.Background()

	events := []*event.HookEvent{
		{EventID: "pre-1", SessionID: "s1", EventType: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"ls"}`, CreatedAt: "2026-06-18T10:00:00.000Z"},
		{EventID: "post-1", SessionID: "s1", EventType: "PostToolUse", ToolName: "Bash", ToolOutput: `{"exit_code":0}`, CreatedAt: "2026-06-18T10:00:03.000Z"},
		{EventID: "start-1", SessionID: "s1", EventType: "SessionStart", CreatedAt: "2026-06-18T09:59:59.000Z"},
		{EventID: "stop-1", SessionID: "s1", EventType: "Stop", CreatedAt: "2026-06-18T10:00:10.000Z"},
	}

	trace, err := tracer.BuildTrace(ctx, events)
	require.NoError(t, err)
	require.NotNil(t, trace)

	assert.Equal(t, "s1", trace.SessionID)
	assert.Equal(t, 4, trace.TotalEvents)
	assert.Len(t, trace.Spans, 1)
	assert.Len(t, trace.StandaloneEvents, 2)

	span := trace.Spans[0]
	assert.Equal(t, "Bash", span.ToolName)
	assert.Equal(t, "pre-1", span.PreEventID)
	assert.Equal(t, "post-1", span.PostEventID)
	assert.Equal(t, int64(3000), span.DurationMs)
	assert.False(t, span.Blocked)
	assert.False(t, span.Orphan)
}

// --- TestTracer_Blocked事件 ---
func TestTracer_Blocked事件(t *testing.T) {
	tracer := NewTracer(30)
	ctx := context.Background()

	events := []*event.HookEvent{
		{EventID: "start-1", SessionID: "s2", EventType: "SessionStart", CreatedAt: "2026-06-18T10:00:00.000Z"},
		{EventID: "pre-1", SessionID: "s2", EventType: "PreToolUse", ToolName: "Write", ToolInput: `{"file_path":"src/auth.ts"}`, Blocked: true, BlockReason: "Style check failed", CreatedAt: "2026-06-18T10:00:01.000Z"},
		{EventID: "stop-1", SessionID: "s2", EventType: "Stop", CreatedAt: "2026-06-18T10:00:05.000Z"},
	}

	trace, err := tracer.BuildTrace(ctx, events)
	require.NoError(t, err)

	require.Len(t, trace.Spans, 1)
	span := trace.Spans[0]
	assert.True(t, span.Blocked)
	assert.True(t, span.Orphan)
	assert.Equal(t, "Style check failed", span.BlockReason)
	assert.Equal(t, int64(0), span.DurationMs)
}

// --- TestTracer_连续同名工具FIFO配对 ---
func TestTracer_连续同名工具FIFO配对(t *testing.T) {
	tracer := NewTracer(30)
	ctx := context.Background()

	events := []*event.HookEvent{
		{EventID: "pre-1", SessionID: "s3", EventType: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"ls"}`, CreatedAt: "2026-06-18T10:00:00.000Z"},
		{EventID: "pre-2", SessionID: "s3", EventType: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"pwd"}`, CreatedAt: "2026-06-18T10:00:01.000Z"},
		{EventID: "post-1", SessionID: "s3", EventType: "PostToolUse", ToolName: "Bash", ToolOutput: `{"exit_code":0,"stdout":"src"}`, CreatedAt: "2026-06-18T10:00:02.000Z"},
		{EventID: "post-2", SessionID: "s3", EventType: "PostToolUse", ToolName: "Bash", ToolOutput: `{"exit_code":0,"stdout":"/home"}`, CreatedAt: "2026-06-18T10:00:03.000Z"},
	}

	trace, err := tracer.BuildTrace(ctx, events)
	require.NoError(t, err)

	require.Len(t, trace.Spans, 2)
	// FIFO: 第一个 post 配对第一个 pre
	assert.Equal(t, "pre-1", trace.Spans[0].PreEventID)
	assert.Equal(t, `{"command":"ls"}`, trace.Spans[0].ToolInput)
	assert.Equal(t, "pre-2", trace.Spans[1].PreEventID)
	assert.Equal(t, `{"command":"pwd"}`, trace.Spans[1].ToolInput)
}

// --- TestTracer_OrphanPre事件 ---
func TestTracer_OrphanPre事件(t *testing.T) {
	tracer := NewTracer(30)
	ctx := context.Background()

	events := []*event.HookEvent{
		{EventID: "pre-orphan", SessionID: "s4", EventType: "PreToolUse", ToolName: "Read", ToolInput: `{"file_path":"src/main.go"}`, CreatedAt: "2026-06-18T10:00:00.000Z"},
	}

	trace, err := tracer.BuildTrace(ctx, events)
	require.NoError(t, err)

	require.Len(t, trace.Spans, 1)
	assert.True(t, trace.Spans[0].Orphan)
}

// --- TestTracer_空事件列表 ---
func TestTracer_空事件列表(t *testing.T) {
	tracer := NewTracer(30)
	ctx := context.Background()

	_, err := tracer.BuildTrace(ctx, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")

	_, err = tracer.BuildTrace(ctx, []*event.HookEvent{})
	assert.Error(t, err)
}

// --- TestTracer_ProcessEvent配对 ---
func TestTracer_ProcessEvent配对(t *testing.T) {
	tracer := NewTracer(30)
	ctx := context.Background()

	pre := &event.HookEvent{
		EventID:   "pre-1",
		SessionID: "s5",
		EventType: "PreToolUse",
		ToolName:  "Edit",
		ToolInput: `{"file_path":"src/app.ts"}`,
		CreatedAt: "2026-06-18T10:00:00.000Z",
	}
	require.NoError(t, tracer.ProcessEvent(ctx, pre))

	post := &event.HookEvent{
		EventID:    "post-1",
		SessionID: "s5",
		EventType:  "PostToolUse",
		ToolName:   "Edit",
		ToolOutput: `{"success":true}`,
		CreatedAt:  "2026-06-18T10:00:02.000Z",
	}
	require.NoError(t, tracer.ProcessEvent(ctx, post))

	pending := tracer.PendingSpans("s5")
	assert.Empty(t, pending)
}

// --- TestTracer_ProcessEvent_Blocked不进队列 ---
func TestTracer_ProcessEvent_Blocked不进队列(t *testing.T) {
	tracer := NewTracer(30)
	ctx := context.Background()

	blocked := &event.HookEvent{
		EventID:     "pre-blk",
		SessionID:   "s6",
		EventType:   "PreToolUse",
		ToolName:    "Write",
		Blocked:     true,
		BlockReason: "deny",
		CreatedAt:   "2026-06-18T10:00:00.000Z",
	}
	require.NoError(t, tracer.ProcessEvent(ctx, blocked))

	pending := tracer.PendingSpans("s6")
	assert.Empty(t, pending)
}
