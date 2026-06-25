package trace

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/dartagnanli/agent-insight/pkg/event"
)

// Tracer processes hook events to build call traces via Pre/Post pairing.
type Tracer struct {
	mu       sync.Mutex
	pending  map[string][]*pendingSpan // key: session_id+"/"+tool_name
	orphanMs int64                     // timeout for orphan detection
}

type pendingSpan struct {
	span      *event.Span
	createdAt time.Time
}

// NewTracer creates a new Tracer with the given orphan timeout in seconds.
func NewTracer(orphanTimeoutSec int64) *Tracer {
	if orphanTimeoutSec <= 0 {
		orphanTimeoutSec = 30
	}
	return &Tracer{
		pending:  make(map[string][]*pendingSpan),
		orphanMs: orphanTimeoutSec * 1000,
	}
}

// ProcessEvent handles a new event, attempting Pre/Post pairing.
func (t *Tracer) ProcessEvent(ctx context.Context, evt *event.HookEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch evt.EventType {
	case "PreToolUse":
		span := &event.Span{
			SpanID:     uuid.New().String(),
			ToolName:   evt.ToolName,
			ToolInput:  evt.ToolInput,
			StartedAt:  evt.CreatedAt,
			PreEventID: evt.EventID,
		}
		if evt.Blocked {
			span.Blocked = true
			span.BlockReason = evt.BlockReason
			span.Orphan = true
			span.DurationMs = 0
		} else {
			key := evt.SessionID + "/" + evt.ToolName
			t.pending[key] = append(t.pending[key], &pendingSpan{
				span:      span,
				createdAt: time.Now(),
			})
		}

	case "PostToolUse":
		key := evt.SessionID + "/" + evt.ToolName
		queue := t.pending[key]
		if len(queue) > 0 {
			// FIFO: pair with the earliest pending span
			ps := queue[0]
			t.pending[key] = queue[1:]
			if len(t.pending[key]) == 0 {
				delete(t.pending, key)
			}
			ps.span.EndedAt = evt.CreatedAt
			ps.span.ToolOutput = evt.ToolOutput
			ps.span.PostEventID = evt.EventID
			ps.span.DurationMs = calcDurationMs(ps.span.StartedAt, evt.CreatedAt)
		}
	}

	// Check for orphaned pending spans
	t.markOrphans()

	return nil
}

// BuildTrace constructs the full call trace for a session from stored events.
func (t *Tracer) BuildTrace(ctx context.Context, events []*event.HookEvent) (*event.Trace, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("session not found")
	}

	trace := &event.Trace{
		SessionID:   events[0].SessionID,
		StartedAt:   events[0].CreatedAt,
		TotalEvents: len(events),
	}

	var spans []*event.Span
	var standalone []*event.StandaloneEvent

	// Process events to create spans
	tempPending := make(map[string][]*pendingSpan)
	for _, evt := range events {
		switch evt.EventType {
		case "PreToolUse":
			span := &event.Span{
				SpanID:     uuid.New().String(),
				ToolName:   evt.ToolName,
				ToolInput:  evt.ToolInput,
				StartedAt:  evt.CreatedAt,
				PreEventID: evt.EventID,
			}
			if evt.Blocked {
				span.Blocked = true
				span.BlockReason = evt.BlockReason
				span.Orphan = true
				span.DurationMs = 0
				spans = append(spans, span)
			} else {
				key := evt.SessionID + "/" + evt.ToolName
				tempPending[key] = append(tempPending[key], &pendingSpan{span: span})
			}

		case "PostToolUse":
			key := evt.SessionID + "/" + evt.ToolName
			queue := tempPending[key]
			if len(queue) > 0 {
				ps := queue[0]
				tempPending[key] = queue[1:]
				ps.span.EndedAt = evt.CreatedAt
				ps.span.ToolOutput = evt.ToolOutput
				ps.span.PostEventID = evt.EventID
				ps.span.DurationMs = calcDurationMs(ps.span.StartedAt, evt.CreatedAt)
				spans = append(spans, ps.span)
			} else {
				// Orphan Post without Pre -- treat as standalone
				standalone = append(standalone, &event.StandaloneEvent{
					EventID:   evt.EventID,
					EventType: evt.EventType,
					CreatedAt: evt.CreatedAt,
				})
			}

		default:
			standalone = append(standalone, &event.StandaloneEvent{
				EventID:   evt.EventID,
				EventType: evt.EventType,
				CreatedAt: evt.CreatedAt,
			})
		}
	}

	// Remaining pending spans are orphans
	for _, queue := range tempPending {
		for _, ps := range queue {
			ps.span.Orphan = true
			spans = append(spans, ps.span)
		}
	}

	trace.Spans = spans
	trace.StandaloneEvents = standalone

	// Calculate trace duration
	last := events[len(events)-1]
	trace.EndedAt = last.CreatedAt
	trace.DurationSecs = calcDurationSec(trace.StartedAt, last.CreatedAt)

	return trace, nil
}

// PendingSpans returns the current pending spans for a session (debug use).
func (t *Tracer) PendingSpans(sessionID string) []*event.Span {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result []*event.Span
	for key, queue := range t.pending {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 && parts[0] == sessionID {
			for _, ps := range queue {
				result = append(result, ps.span)
			}
		}
	}
	return result
}

func (t *Tracer) markOrphans() {
	now := time.Now()
	for key, queue := range t.pending {
		var remaining []*pendingSpan
		for _, ps := range queue {
			if now.Sub(ps.createdAt).Milliseconds() > t.orphanMs {
				ps.span.Orphan = true
			} else {
				remaining = append(remaining, ps)
			}
		}
		t.pending[key] = remaining
		if len(remaining) == 0 {
			delete(t.pending, key)
		}
	}
}

func calcDurationMs(startedAt, endedAt string) int64 {
	t1, err1 := time.Parse("2006-01-02T15:04:05.000Z", startedAt)
	t2, err2 := time.Parse("2006-01-02T15:04:05.000Z", endedAt)
	if err1 != nil || err2 != nil {
		return 0
	}
	return t2.Sub(t1).Milliseconds()
}

func calcDurationSec(startedAt, endedAt string) int64 {
	ms := calcDurationMs(startedAt, endedAt)
	if ms <= 0 {
		return 0
	}
	return ms / 1000
}
