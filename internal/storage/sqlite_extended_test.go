package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/internal/config"
	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newExtendedTestDB creates a temporary SQLite database for extended tests.
func newExtendedTestDB(t *testing.T) *SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// --- TestCountEvents_返回正确总数 ---
func TestCountEvents_返回正确总数(t *testing.T) {
	s := newExtendedTestDB(t)
	ctx := context.Background()

	// 空数据库返回 0
	count, err := s.CountEvents(ctx, EventFilter{})
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// 插入 5 条事件
	for i := 0; i < 5; i++ {
		require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
			EventID:   fmt.Sprintf("evt-count-%d", i),
			SessionID: "sess-count",
			EventType: "PreToolUse",
			ToolName:  "Bash",
			Cwd:       "/home/user",
			Hostname:  "test-host",
			CreatedAt: event.Now(),
		}))
	}

	count, err = s.CountEvents(ctx, EventFilter{})
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

// --- TestCountEvents_按过滤条件计数 ---
func TestCountEvents_按过滤条件计数(t *testing.T) {
	s := newExtendedTestDB(t)
	ctx := context.Background()

	require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-cf-1", SessionID: "sess-cf", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))
	require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-cf-2", SessionID: "sess-cf", EventType: "PostToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))
	require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-cf-3", SessionID: "sess-cf", EventType: "PreToolUse",
		ToolName: "Write", Cwd: "/home", Hostname: "h", Blocked: true, CreatedAt: event.Now(),
	}))

	tests := []struct {
		name      string
		filter    EventFilter
		wantCount int
	}{
		{"按EventType过滤", EventFilter{EventType: ptrStr("PreToolUse")}, 2},
		{"按ToolName过滤", EventFilter{ToolName: ptrStr("Bash")}, 2},
		{"按Blocked过滤", EventFilter{Blocked: ptrBool(true)}, 1},
		{"按SessionID过滤", EventFilter{SessionID: ptrStr("sess-cf")}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := s.CountEvents(ctx, tt.filter)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}

// --- TestQueryRecentEvents_按id递增查询 ---
func TestQueryRecentEvents_按id递增查询(t *testing.T) {
	s := newExtendedTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
			EventID:   fmt.Sprintf("evt-recent-%d", i),
			SessionID: "sess-recent",
			EventType: "PreToolUse",
			ToolName:  "Bash",
			Cwd:       "/home/user",
			Hostname:  "test-host",
			CreatedAt: now.Add(time.Duration(i) * time.Second).Format("2006-01-02T15:04:05.000Z"),
		}))
	}

	// 查询 id > 0 的最近 5 条
	events, err := s.QueryRecentEvents(ctx, 0, 5)
	require.NoError(t, err)
	assert.Len(t, events, 5)

	// 验证按 id 递增排序
	for i := 1; i < len(events); i++ {
		assert.Greater(t, events[i].ID, events[i-1].ID)
	}

	// 用最后一条的 id 作为 afterID 继续查询
	lastID := events[len(events)-1].ID
	events2, err := s.QueryRecentEvents(ctx, lastID, 10)
	require.NoError(t, err)
	assert.Len(t, events2, 5) // 剩余的 5 条

	// 验证所有事件的 id 都大于 afterID
	for _, evt := range events2 {
		assert.Greater(t, evt.ID, lastID)
	}
}

// --- TestQueryRecentEvents_默认limit ---
func TestQueryRecentEvents_默认limit(t *testing.T) {
	s := newExtendedTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
			EventID:   fmt.Sprintf("evt-rl-%d", i),
			SessionID: "sess-rl",
			EventType: "PreToolUse",
			ToolName:  "Bash",
			Cwd:       "/home",
			Hostname:  "h",
			CreatedAt: event.Now(),
		}))
	}

	// limit=0 应使用默认值 1000
	events, err := s.QueryRecentEvents(ctx, 0, 0)
	require.NoError(t, err)
	assert.Len(t, events, 5)

	// limit=-1 应使用默认值 1000
	events, err = s.QueryRecentEvents(ctx, 0, -1)
	require.NoError(t, err)
	assert.Len(t, events, 5)
}

// --- TestListSessionsExtended_返回完整字段 ---
func TestListSessionsExtended_返回完整字段(t *testing.T) {
	s := newExtendedTestDB(t)
	ctx := context.Background()

	startedAt := "2026-06-18T10:00:00.000Z"
	endedAt := "2026-06-18T10:05:00.000Z"
	pp := "/home/user/project"
	avg := 1800.0
	p99 := 5100.0

	row := &SessionStatsRow{
		SessionID:   "sess-ext-1",
		StartedAt:   startedAt,
		EndedAt:     &endedAt,
		DurationSecs: 300,
		TotalEvents: 42,
		ToolCalls:   35,
		BlockedCalls: 3,
		BlockRate:    0.086,
		ToolsUsed:    `["Bash","Write","Edit"]`,
		AvgToolDurationMs: &avg,
		P99ToolDurationMs: &p99,
		ProjectPath:  &pp,
	}
	require.NoError(t, s.UpsertSessionStats(ctx, row))

	sessions, err := s.ListSessions(ctx, SessionFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	sm := sessions[0]
	assert.Equal(t, "sess-ext-1", sm.SessionID)
	assert.Equal(t, 42, sm.TotalEvents)
	assert.Equal(t, int64(300), sm.DurationSecs)
	assert.Equal(t, 3, sm.BlockedCalls)
	assert.NotEmpty(t, sm.StartedAt)
}

// --- TestCountSessions_按条件计数 ---
func TestCountSessions_按条件计数(t *testing.T) {
	s := newExtendedTestDB(t)
	ctx := context.Background()

	pp1 := "/home/user/project1"
	pp2 := "/home/user/project2"

	for i, pp := range []string{pp1, pp1, pp2} {
		row := &SessionStatsRow{
			SessionID:   fmt.Sprintf("sess-cs-%d", i),
			StartedAt:   event.Now(),
			TotalEvents: (i + 1) * 5,
			ToolCalls:   (i + 1) * 2,
			BlockedCalls: i,
			BlockRate:    float64(i) / float64((i+1)*2),
			ToolsUsed:    `["Bash"]`,
			ProjectPath:  &pp,
		}
		require.NoError(t, s.UpsertSessionStats(ctx, row))
	}

	tests := []struct {
		name      string
		filter    SessionFilter
		wantCount int
	}{
		{"全部", SessionFilter{}, 3},
		{"按ProjectPath", SessionFilter{ProjectPath: &pp1}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := s.CountSessions(ctx, tt.filter)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}

// --- TestQueryRecentEvents_无匹配返回空 ---
func TestQueryRecentEvents_无匹配返回空(t *testing.T) {
	s := newExtendedTestDB(t)

	events, err := s.QueryRecentEvents(t.Context(), 9999, 100)
	require.NoError(t, err)
	assert.Empty(t, events)
}

// --- TestCountEvents_时间范围 ---
func TestCountEvents_时间范围(t *testing.T) {
	s := newExtendedTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	recent := now.Add(-5 * time.Minute)

	require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-ct-1", SessionID: "sess-ct", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h",
		CreatedAt: past.Format("2006-01-02T15:04:05.000Z"),
	}))
	require.NoError(t, s.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-ct-2", SessionID: "sess-ct", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h",
		CreatedAt: recent.Format("2006-01-02T15:04:05.000Z"),
	}))

	since := now.Add(-1 * time.Hour)
	count, err := s.CountEvents(ctx, EventFilter{Since: &since})
	require.NoError(t, err)
	assert.Equal(t, 1, count) // 只有 recent 的那条
}

// ptrStr returns a pointer to the given string value.
func ptrStr(s string) *string {
	return &s
}

// ptrBool returns a pointer to the given bool value.
func ptrBool(b bool) *bool {
	return &b
}
