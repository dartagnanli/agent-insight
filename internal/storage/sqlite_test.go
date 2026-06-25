package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/internal/config"
	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDB creates a temporary SQLite database for testing.
func newTestDB(t *testing.T) *SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// --- TestSQLite_Init_建表成功 ---
func TestSQLite_Init_建表成功(t *testing.T) {
	s := newTestDB(t)
	// Verify tables exist
	var name string
	err := s.db.QueryRowContext(context.Background(), "SELECT name FROM sqlite_master WHERE type='table' AND name='hook_events'").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "hook_events", name)

	err = s.db.QueryRowContext(context.Background(), "SELECT name FROM sqlite_master WHERE type='table' AND name='session_stats'").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "session_stats", name)

	err = s.db.QueryRowContext(context.Background(), "SELECT name FROM sqlite_master WHERE type='table' AND name='stats_hourly'").Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "stats_hourly", name)
}

// --- TestSQLite_InsertEvent_正常写入 ---
func TestSQLite_InsertEvent_正常写入(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	evt := &event.HookEvent{
		EventID:   "evt-001",
		SessionID: "sess-001",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		ToolInput: `{"command":"ls"}`,
		Cwd:       "/home/user",
		Hostname:  "test-host",
		CreatedAt: event.Now(),
	}
	err := s.InsertEvent(ctx, evt)
	require.NoError(t, err)

	got, err := s.GetEvent(ctx, "evt-001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "evt-001", got.EventID)
	assert.Equal(t, "PreToolUse", got.EventType)
	assert.Equal(t, "Bash", got.ToolName)
	assert.False(t, got.Blocked)
}

// --- TestSQLite_InsertEvent_写入blocked事件 ---
func TestSQLite_InsertEvent_写入blocked事件(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	evt := &event.HookEvent{
		EventID:     "evt-002",
		SessionID:   "sess-001",
		EventType:   "PreToolUse",
		ToolName:    "Write",
		ToolInput:   `{"file_path":"src/auth.ts"}`,
		Blocked:     true,
		BlockReason: "Style check failed",
		Cwd:        "/home/user",
		Hostname:   "test-host",
		CreatedAt:  event.Now(),
	}
	err := s.InsertEvent(ctx, evt)
	require.NoError(t, err)

	got, err := s.GetEvent(ctx, "evt-002")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Blocked)
	assert.Equal(t, "Style check failed", got.BlockReason)
}

// --- TestSQLite_QueryEvents_按session过滤 ---
func TestSQLite_QueryEvents_按session过滤(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		evt := &event.HookEvent{
			EventID:   fmt.Sprintf("evt-a-%d", i),
			SessionID: "sess-a",
			EventType: "PreToolUse",
			ToolName:  "Bash",
			Cwd:       "/home/user",
			Hostname:  "test-host",
			CreatedAt: event.Now(),
		}
		require.NoError(t, s.InsertEvent(ctx, evt))
	}
	for i := 0; i < 3; i++ {
		evt := &event.HookEvent{
			EventID:   fmt.Sprintf("evt-b-%d", i),
			SessionID: "sess-b",
			EventType: "PostToolUse",
			ToolName:  "Write",
			Cwd:       "/home/user",
			Hostname:  "test-host",
			CreatedAt: event.Now(),
		}
		require.NoError(t, s.InsertEvent(ctx, evt))
	}

	sid := "sess-a"
	events, err := s.QueryEvents(ctx, EventFilter{SessionID: &sid, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, events, 5)
	for _, e := range events {
		assert.Equal(t, "sess-a", e.SessionID)
	}
}

// --- TestSQLite_QueryEvents_按event_type过滤 ---
func TestSQLite_QueryEvents_按event_type过滤(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	et := "PostToolUse"
	events, err := s.QueryEvents(ctx, EventFilter{EventType: &et, Limit: 100})
	require.NoError(t, err)
	// No data yet for PostToolUse
	assert.Len(t, events, 0)

	// Insert a PostToolUse event
	evt := &event.HookEvent{
		EventID:    "evt-post-1",
		SessionID: "sess-a",
		EventType:  "PostToolUse",
		ToolName:   "Bash",
		ToolOutput: `{"exit_code":0}`,
		Cwd:        "/home/user",
		Hostname:   "test-host",
		CreatedAt:  event.Now(),
	}
	require.NoError(t, s.InsertEvent(ctx, evt))

	events, err = s.QueryEvents(ctx, EventFilter{EventType: &et, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

// --- TestSQLite_QueryEvents_按blocked过滤 ---
func TestSQLite_QueryEvents_按blocked过滤(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	blocked := true
	evt := &event.HookEvent{
		EventID:     "evt-blk-1",
		SessionID:   "sess-c",
		EventType:   "PreToolUse",
		ToolName:    "Write",
		Blocked:     true,
		BlockReason: "test block",
		Cwd:         "/home/user",
		Hostname:    "test-host",
		CreatedAt:   event.Now(),
	}
	require.NoError(t, s.InsertEvent(ctx, evt))

	events, err := s.QueryEvents(ctx, EventFilter{Blocked: &blocked, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.True(t, events[0].Blocked)
}

// --- TestSQLite_QueryEvents_时间范围过滤 ---
func TestSQLite_QueryEvents_时间范围过滤(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(1 * time.Hour)

	evt := &event.HookEvent{
		EventID:   "evt-time-1",
		SessionID: "sess-d",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "test-host",
		CreatedAt: past.Format("2006-01-02T15:04:05.000Z"),
	}
	require.NoError(t, s.InsertEvent(ctx, evt))

	events, err := s.QueryEvents(ctx, EventFilter{Since: &past, Until: &future, Limit: 100})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 1)
}

// --- TestSQLite_QueryEvents_排序 ---
func TestSQLite_QueryEvents_排序(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	evt1 := &event.HookEvent{
		EventID:   "evt-sort-1",
		SessionID: "sess-sort",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "test-host",
		HookDurationMs: 10,
		CreatedAt: event.Now(),
	}
	evt2 := &event.HookEvent{
		EventID:   "evt-sort-2",
		SessionID: "sess-sort",
		EventType: "PreToolUse",
		ToolName:  "Write",
		Cwd:       "/home/user",
		Hostname:  "test-host",
		HookDurationMs: 50,
		CreatedAt: event.Now(),
	}
	require.NoError(t, s.InsertEvent(ctx, evt1))
	require.NoError(t, s.InsertEvent(ctx, evt2))

	sid := "sess-sort"
	events, err := s.QueryEvents(ctx, EventFilter{
		SessionID: &sid,
		SortBy:    "hook_duration_ms",
		SortOrder: "asc",
		Limit:     100,
	})
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.LessOrEqual(t, events[0].HookDurationMs, events[1].HookDurationMs)
}

// --- TestSQLite_GetEvent_不存在返回nil ---
func TestSQLite_GetEvent_不存在返回nil(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	got, err := s.GetEvent(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// --- TestSQLite_DeleteBefore_数据保留清理 ---
func TestSQLite_DeleteBefore_数据保留清理(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	past := time.Now().UTC().Add(-48 * time.Hour)
	evt := &event.HookEvent{
		EventID:   "evt-old-1",
		SessionID: "sess-old",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "test-host",
		CreatedAt: past.Format("2006-01-02T15:04:05.000Z"),
	}
	require.NoError(t, s.InsertEvent(ctx, evt))

	// Insert a recent event
	evt2 := &event.HookEvent{
		EventID:   "evt-new-1",
		SessionID: "sess-new",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "test-host",
		CreatedAt: event.Now(),
	}
	require.NoError(t, s.InsertEvent(ctx, evt2))

	// Delete events older than 24 hours
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	deleted, err := s.DeleteBefore(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Old event should be gone
	got, err := s.GetEvent(ctx, "evt-old-1")
	require.NoError(t, err)
	assert.Nil(t, got)

	// New event should remain
	got, err = s.GetEvent(ctx, "evt-new-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "evt-new-1", got.EventID)
}

// --- TestSQLite_UpsertSessionStats_正常写入 ---
func TestSQLite_UpsertSessionStats_正常写入(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	row := &SessionStatsRow{
		SessionID:   "sess-stats-1",
		StartedAt:   event.Now(),
		TotalEvents: 42,
		ToolCalls:   35,
		BlockedCalls: 3,
		BlockRate:    0.086,
		ToolsUsed:   `["Bash","Write","Edit"]`,
	}
	avg := 1800.0
	p99 := 5100.0
	row.AvgToolDurationMs = &avg
	row.P99ToolDurationMs = &p99
	pp := "/home/user/project"
	row.ProjectPath = &pp

	err := s.UpsertSessionStats(ctx, row)
	require.NoError(t, err)

	rows, err := s.QuerySessionStats(ctx, SessionFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "sess-stats-1", rows[0].SessionID)
	assert.Equal(t, 42, rows[0].TotalEvents)
	assert.Equal(t, 35, rows[0].ToolCalls)
	assert.InDelta(t, 0.086, rows[0].BlockRate, 0.001)
}

// --- TestSQLite_UpsertSessionStats_更新覆盖 ---
func TestSQLite_UpsertSessionStats_更新覆盖(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	row := &SessionStatsRow{
		SessionID:   "sess-upsert",
		StartedAt:   event.Now(),
		TotalEvents: 10,
		ToolCalls:   8,
		BlockedCalls: 1,
		BlockRate:    0.125,
		ToolsUsed:   `["Bash"]`,
	}
	require.NoError(t, s.UpsertSessionStats(ctx, row))

	// Update the same session
	row.TotalEvents = 20
	row.ToolCalls = 16
	row.BlockedCalls = 2
	row.BlockRate = 0.125
	require.NoError(t, s.UpsertSessionStats(ctx, row))

	rows, err := s.QuerySessionStats(ctx, SessionFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 20, rows[0].TotalEvents)
	assert.Equal(t, 16, rows[0].ToolCalls)
}

// --- TestSQLite_UpsertStatsHourly_正常写入 ---
func TestSQLite_UpsertStatsHourly_正常写入(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	bucket := "2026-06-18T10:00:00Z"
	toolName := "Bash"
	avg := 2.1
	p50 := 1.5
	p95 := 3.8
	p99 := 4.6

	rows := []StatsHourlyRow{
		{
			BucketHour:    bucket,
			EventType:     "PreToolUse",
			ToolName:      &toolName,
			EventCount:    120,
			BlockCount:    8,
			AvgDurationMs: &avg,
			P50DurationMs: &p50,
			P95DurationMs: &p95,
			P99DurationMs: &p99,
		},
	}

	err := s.UpsertStatsHourly(ctx, rows)
	require.NoError(t, err)

	got, err := s.QueryStatsHourly(ctx, StatsFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, bucket, got[0].BucketHour)
	assert.Equal(t, 120, got[0].EventCount)
	require.NotNil(t, got[0].ToolName)
	assert.Equal(t, "Bash", *got[0].ToolName)
}

// --- TestSQLite_并发写入安全 ---
func TestSQLite_并发写入安全(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	count := 100
	errCh := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			evt := &event.HookEvent{
				EventID:   fmt.Sprintf("evt-concurrent-%d", i),
				SessionID: "sess-concurrent",
				EventType: "PreToolUse",
				ToolName:  "Bash",
				Cwd:       "/home/user",
				Hostname:  "test-host",
				CreatedAt: event.Now(),
			}
			if err := s.InsertEvent(ctx, evt); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent write error: %v", err)
	}

	sid := "sess-concurrent"
	events, err := s.QueryEvents(ctx, EventFilter{SessionID: &sid, Limit: 1000})
	require.NoError(t, err)
	assert.Len(t, events, count)
}

// --- TestSQLite_数据库文件权限 ---
func TestSQLite_数据库文件权限(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	defer s.Close()

	info, err := os.Stat(dbPath)
	require.NoError(t, err)
	// Verify file permission is 0600
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

// --- TestSQLite_损坏数据库恢复 ---
func TestSQLite_损坏数据库恢复(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")

	// Write garbage data to simulate corruption
	require.NoError(t, os.WriteFile(dbPath, []byte("not a valid sqlite database"), 0600))

	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	defer s.Close()

	// After recovery, should be able to insert
	err = s.InsertEvent(context.Background(), &event.HookEvent{
		EventID:   "evt-recover-1",
		SessionID: "sess-recover",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "test-host",
		CreatedAt: event.Now(),
	})
	require.NoError(t, err)
}

// --- TestSQLite_Schema迁移_版本递增 ---
func TestSQLite_Schema迁移_版本递增(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	// Check version is 1
	var version int
	err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_version").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, 2, version)
}

// --- TestSQLite_ListSessions_排序 ---
func TestSQLite_ListSessions_排序(t *testing.T) {
	s := newTestDB(t)
	ctx := context.Background()

	// Insert session stats
	for i, sid := range []string{"sess-ls-1", "sess-ls-2", "sess-ls-3"} {
		row := &SessionStatsRow{
			SessionID:   sid,
			StartedAt:   event.Now(),
			TotalEvents: (i + 1) * 10,
			ToolCalls:   (i + 1) * 5,
			BlockedCalls: i,
			BlockRate:    float64(i) / float64((i+1)*5),
			ToolsUsed:    `["Bash"]`,
		}
		require.NoError(t, s.UpsertSessionStats(ctx, row))
	}

	sessions, err := s.ListSessions(ctx, SessionFilter{SortBy: "total_events", SortOrder: "desc", Limit: 10})
	require.NoError(t, err)
	require.Len(t, sessions, 3)
	// Should be ordered by total_events descending
	assert.Equal(t, "sess-ls-3", sessions[0].SessionID)
	assert.Equal(t, "sess-ls-1", sessions[2].SessionID)
}
