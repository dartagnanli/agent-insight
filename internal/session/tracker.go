package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/libin18/agent-insight/internal/storage"
	"github.com/libin18/agent-insight/pkg/event"
)

// Tracker tracks session lifecycle and computes aggregate stats.
type Tracker struct {
	mu          sync.Mutex
	sessions    map[string]*sessionState
	timeoutSec  int64 // session timeout without Stop event
}

type sessionState struct {
	sessionID  string
	startedAt  string
	lastSeen   time.Time
	events     []*event.HookEvent
	ended      bool
	endedAt    string
}

// NewTracker creates a new session Tracker.
func NewTracker(timeoutSec int64) *Tracker {
	if timeoutSec <= 0 {
		timeoutSec = 300 // 5 minutes default
	}
	return &Tracker{
		sessions:   make(map[string]*sessionState),
		timeoutSec: timeoutSec,
	}
}

// Track processes a new event for session tracking.
func (t *Tracker) Track(ctx context.Context, evt *event.HookEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ss, ok := t.sessions[evt.SessionID]
	if !ok {
		ss = &sessionState{
			sessionID: evt.SessionID,
			events:    make([]*event.HookEvent, 0),
		}
		t.sessions[evt.SessionID] = ss
	}

	ss.events = append(ss.events, evt)
	ss.lastSeen = time.Now()

	if evt.EventType == "SessionStart" && ss.startedAt == "" {
		ss.startedAt = evt.CreatedAt
	}

	if evt.EventType == "Stop" || evt.EventType == "SubagentStop" {
		ss.ended = true
		ss.endedAt = evt.CreatedAt
	}

	return nil
}

// Aggregate computes the session stats for a given session.
func (t *Tracker) Aggregate(sessionID string) (*storage.SessionStatsRow, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ss, ok := t.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return t.aggregateState(ss), nil
}

// CheckTimeouts marks sessions as ended if their last event is older than the timeout.
func (t *Tracker) CheckTimeouts() {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(t.timeoutSec) * time.Second)
	for _, ss := range t.sessions {
		if !ss.ended && ss.lastSeen.Before(cutoff) {
			ss.ended = true
			if ss.lastSeen.Unix() > 0 {
				ss.endedAt = ss.lastSeen.Add(time.Duration(t.timeoutSec) * time.Second).UTC().Format("2006-01-02T15:04:05.000Z")
			}
		}
	}
}

// ListSessions returns all tracked sessions.
func (t *Tracker) ListSessions() []*storage.SessionStatsRow {
	t.mu.Lock()
	defer t.mu.Unlock()

	var results []*storage.SessionStatsRow
	for _, ss := range t.sessions {
		results = append(results, t.aggregateState(ss))
	}
	return results
}

func (t *Tracker) aggregateState(ss *sessionState) *storage.SessionStatsRow {
	row := &storage.SessionStatsRow{
		SessionID:   ss.sessionID,
		StartedAt:   ss.startedAt,
		TotalEvents: len(ss.events),
	}

	if ss.endedAt != "" {
		row.EndedAt = &ss.endedAt
	}

	var toolCalls, blockedCalls int
	var toolSet map[string]bool = make(map[string]bool)
	var toolDurations []float64

	for _, evt := range ss.events {
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
	}

	row.ToolCalls = toolCalls
	row.BlockedCalls = blockedCalls
	if toolCalls > 0 {
		row.BlockRate = float64(blockedCalls) / float64(toolCalls)
	}

	// Build tools_used JSON array
	tools := make([]string, 0, len(toolSet))
	for t := range toolSet {
		tools = append(tools, t)
	}
	sort.Strings(tools)
	toolsJSON, _ := json.Marshal(tools)
	row.ToolsUsed = string(toolsJSON)

	if len(toolDurations) > 0 {
		sort.Float64s(toolDurations)
		avg := average(toolDurations)
		p99 := percentile(toolDurations, 99)
		row.AvgToolDurationMs = &avg
		row.P99ToolDurationMs = &p99
	}

	// Duration
	if ss.startedAt != "" && ss.endedAt != "" {
		row.DurationSecs = calcDurSec(ss.startedAt, ss.endedAt)
	}

	// Project path from first event
	if len(ss.events) > 0 {
		pp := ss.events[0].Cwd
		row.ProjectPath = &pp
	}

	return row
}

func calcDurSec(start, end string) int64 {
	t1, err1 := time.Parse("2006-01-02T15:04:05.000Z", start)
	t2, err2 := time.Parse("2006-01-02T15:04:05.000Z", end)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int64(t2.Sub(t1).Seconds())
}
