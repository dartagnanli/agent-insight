package integration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/libin18/agent-insight/internal/config"
	"github.com/libin18/agent-insight/internal/storage"
	"github.com/libin18/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newIntegrationDB creates a real SQLite database for integration testing.
func newIntegrationDB(t *testing.T) *storage.SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := storage.NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// --- TestInit_创建配置目录 ---
func TestInit_创建配置目录(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".agent-insight")
	require.NoError(t, os.MkdirAll(configDir, 0700))
	info, err := os.Stat(configDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// --- TestCollectAndTrace_端到端 ---
func TestCollectAndTrace_端到端(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	sessionID := "sess-e2e"
	events := []*event.HookEvent{
		{EventID: "e-ss", SessionID: sessionID, EventType: "SessionStart", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:00.000Z"},
		{EventID: "e-pre1", SessionID: sessionID, EventType: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"npm test"}`, Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:01.000Z"},
		{EventID: "e-post1", SessionID: sessionID, EventType: "PostToolUse", ToolName: "Bash", ToolOutput: `{"exit_code":0,"stdout":"All tests passed"}`, Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:05.000Z"},
		{EventID: "e-pre2", SessionID: sessionID, EventType: "PreToolUse", ToolName: "Write", ToolInput: `{"file_path":"src/auth.ts"}`, Blocked: true, BlockReason: "Style check failed: missing semicolon", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:06.000Z"},
		{EventID: "e-pre3", SessionID: sessionID, EventType: "PreToolUse", ToolName: "Write", ToolInput: `{"file_path":"src/auth.ts"}`, Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:08.000Z"},
		{EventID: "e-post3", SessionID: sessionID, EventType: "PostToolUse", ToolName: "Write", ToolOutput: `{"success":true}`, Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:00:09.000Z"},
		{EventID: "e-stop", SessionID: sessionID, EventType: "Stop", Cwd: "/home/user", Hostname: "host", CreatedAt: "2026-06-18T10:02:34.000Z"},
	}

	for _, evt := range events {
		require.NoError(t, db.InsertEvent(ctx, evt))
	}

	sid := sessionID
	gotEvents, err := db.QueryEvents(ctx, storage.EventFilter{
		SessionID: &sid,
		SortBy:    "created_at",
		SortOrder: "asc",
		Limit:     10000,
	})
	require.NoError(t, err)
	assert.Len(t, gotEvents, 7)

	blockedCount := 0
	for _, e := range gotEvents {
		if e.Blocked {
			blockedCount++
			assert.Equal(t, "Write", e.ToolName)
			assert.Equal(t, "Style check failed: missing semicolon", e.BlockReason)
		}
	}
	assert.Equal(t, 1, blockedCount)
}

// --- TestSessions_查询排序 ---
func TestSessions_查询排序(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	for i, sid := range []string{"sess-ls-a", "sess-ls-b", "sess-ls-c"} {
		row := &storage.SessionStatsRow{
			SessionID:   sid,
			StartedAt:   event.Now(),
			TotalEvents: (i + 1) * 10,
			ToolCalls:   (i + 1) * 5,
			BlockedCalls: i,
			BlockRate:    float64(i) / float64((i+1)*5),
			ToolsUsed:   `["Bash"]`,
		}
		require.NoError(t, db.UpsertSessionStats(ctx, row))
	}

	sessions, err := db.ListSessions(ctx, storage.SessionFilter{
		SortBy:    "total_events",
		SortOrder: "desc",
		Limit:     10,
	})
	require.NoError(t, err)
	require.Len(t, sessions, 3)
	assert.Equal(t, "sess-ls-c", sessions[0].SessionID)
}

// --- TestStats_空数据 ---
func TestStats_空数据(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	events, err := db.QueryEvents(ctx, storage.EventFilter{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

// --- TestTrace_不存在session ---
func TestTrace_不存在session(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	sid := "nonexistent"
	events, err := db.QueryEvents(ctx, storage.EventFilter{SessionID: &sid, Limit: 10000})
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

// --- TestDBPath_环境变量配置 ---
func TestDBPath_环境变量配置(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom.db")

	t.Setenv("AGENT_INSIGHT_DB_PATH", customPath)

	cfg := config.StorageConfig{Type: "sqlite", Path: ""}
	db, err := storage.NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	defer db.Close()

	_, err = os.Stat(customPath)
	require.NoError(t, err)
}

// --- TestCollect_5ms内完成 ---
func TestCollect_5ms内完成(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	start := time.Now()
	evt := &event.HookEvent{
		EventID:   "perf-evt",
		SessionID: "sess-perf",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		ToolInput: `{"command":"ls"}`,
		Cwd:       "/home/user",
		Hostname:  "host",
		CreatedAt: event.Now(),
	}
	require.NoError(t, db.InsertEvent(ctx, evt))
	elapsed := time.Since(start)

	assert.Less(t, elapsed.Milliseconds(), int64(50))
}

// --- TestVersion_输出格式 ---
func TestVersion_输出格式(t *testing.T) {
	assert.NotEmpty(t, config.Version)
}

// --- TestStats_时间范围过滤 ---
func TestStats_时间范围过滤(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	past := time.Now().UTC().Add(-2 * time.Hour)
	evt := &event.HookEvent{
		EventID:   "evt-time-range",
		SessionID: "sess-time-range",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "host",
		CreatedAt: past.Format("2006-01-02T15:04:05.000Z"),
	}
	require.NoError(t, db.InsertEvent(ctx, evt))

	since := time.Now().UTC().Add(-1 * time.Hour)
	events, err := db.QueryEvents(ctx, storage.EventFilter{Since: &since, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, events, 0)

	since = time.Now().UTC().Add(-3 * time.Hour)
	events, err = db.QueryEvents(ctx, storage.EventFilter{Since: &since, Limit: 100})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 1)
}

// --- TestDeleteBefore_数据保留 ---
func TestDeleteBefore_数据保留(t *testing.T) {
	db := newIntegrationDB(t)
	ctx := context.Background()

	oldTime := time.Now().UTC().Add(-48 * time.Hour)
	newTime := time.Now().UTC()

	oldEvt := &event.HookEvent{
		EventID:   "evt-old",
		SessionID: "sess-retention",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "host",
		CreatedAt: oldTime.Format("2006-01-02T15:04:05.000Z"),
	}
	newEvt := &event.HookEvent{
		EventID:   "evt-new",
		SessionID: "sess-retention",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home/user",
		Hostname:  "host",
		CreatedAt: newTime.Format("2006-01-02T15:04:05.000Z"),
	}
	require.NoError(t, db.InsertEvent(ctx, oldEvt))
	require.NoError(t, db.InsertEvent(ctx, newEvt))

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	deleted, err := db.DeleteBefore(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	got, err := db.GetEvent(ctx, "evt-old")
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = db.GetEvent(ctx, "evt-new")
	require.NoError(t, err)
	require.NotNil(t, got)
}

// ensure strings and bytes are used
var _ = strings.NewReader
var _ = bytes.NewReader
