package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/libin18/agent-insight/internal/config"
	"github.com/libin18/agent-insight/internal/storage"
	"github.com/libin18/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestAggregatorDB creates a temporary SQLite for aggregator tests.
func newTestAggregatorDB(t *testing.T) *storage.SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := storage.NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// --- TestAggregator_从事件聚合 ---
func TestAggregator_从事件聚合(t *testing.T) {
	db := newTestAggregatorDB(t)
	agg := NewAggregator(db)
	ctx := context.Background()

	events := []*event.HookEvent{
		{EventID: "e1", SessionID: "sess-agg-1", EventType: "SessionStart", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:00.000Z"},
		{EventID: "e2", SessionID: "sess-agg-1", EventType: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"ls"}`, Cwd: "/home/user", Hostname: "host", HookDurationMs: 2, CreatedAt: "2026-06-18T10:00:01.000Z"},
		{EventID: "e3", SessionID: "sess-agg-1", EventType: "PostToolUse", ToolName: "Bash", ToolOutput: `{"exit_code":0}`, Cwd: "/home/user", Hostname: "host", HookDurationMs: 3, CreatedAt: "2026-06-18T10:00:04.000Z"},
		{EventID: "e4", SessionID: "sess-agg-1", EventType: "PreToolUse", ToolName: "Write", ToolInput: `{"file_path":"src/x.ts"}`, Cwd: "/home/user", Hostname: "host", Blocked: true, BlockReason: "check", HookDurationMs: 1, CreatedAt: "2026-06-18T10:00:05.000Z"},
		{EventID: "e5", SessionID: "sess-agg-1", EventType: "Stop", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:05:00.000Z"},
	}

	err := agg.AggregateFromEvents(ctx, events)
	require.NoError(t, err)

	rows, err := db.QuerySessionStats(ctx, storage.SessionFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, "sess-agg-1", row.SessionID)
	assert.Equal(t, 5, row.TotalEvents)
	assert.Equal(t, 2, row.ToolCalls)
	assert.Equal(t, 1, row.BlockedCalls)
	assert.Contains(t, row.ToolsUsed, "Bash")
	assert.Contains(t, row.ToolsUsed, "Write")
}

// --- TestAggregator_空事件列表报错 ---
func TestAggregator_空事件列表报错(t *testing.T) {
	db := newTestAggregatorDB(t)
	agg := NewAggregator(db)

	err := agg.AggregateFromEvents(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no events")
}

// --- TestAggregator_Upsert覆盖 ---
func TestAggregator_Upsert覆盖(t *testing.T) {
	db := newTestAggregatorDB(t)
	agg := NewAggregator(db)
	ctx := context.Background()

	events1 := []*event.HookEvent{
		{EventID: "e1", SessionID: "sess-upsert", EventType: "SessionStart", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:00.000Z"},
	}
	require.NoError(t, agg.AggregateFromEvents(ctx, events1))

	events2 := []*event.HookEvent{
		{EventID: "e1", SessionID: "sess-upsert", EventType: "SessionStart", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:00.000Z"},
		{EventID: "e2", SessionID: "sess-upsert", EventType: "PreToolUse", ToolName: "Bash", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:01.000Z"},
		{EventID: "e3", SessionID: "sess-upsert", EventType: "Stop", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:01:00.000Z"},
	}
	require.NoError(t, agg.AggregateFromEvents(ctx, events2))

	rows, err := db.QuerySessionStats(ctx, storage.SessionFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 3, rows[0].TotalEvents)
	assert.Equal(t, 1, rows[0].ToolCalls)
}

// ensure os and filepath imports are used
var _ = os.ReadFile
var _ = filepath.Join
