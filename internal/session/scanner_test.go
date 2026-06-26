package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/internal/config"
	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newScannerTestDB 创建内存 SQLite 数据库用于 Scanner 测试
func newScannerTestDB(t *testing.T) *storage.SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := storage.NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// --- TestScannerScanAndRepair_扫描补全未聚合session ---
func TestScannerScanAndRepair_扫描补全未聚合session(t *testing.T) {
	db := newScannerTestDB(t)
	ctx := context.Background()

	// 插入事件但不执行聚合（使用足够早的时间以超过超时阈值）
	pastTime := time.Now().Add(-time.Hour).UTC()
	events := []*event.HookEvent{
		{EventID: "e-scan-1", SessionID: "sess-scan-1", EventType: "SessionStart", Cwd: "/home/user", Hostname: "h", CreatedAt: pastTime.Format("2006-01-02T15:04:05.000Z")},
		{EventID: "e-scan-2", SessionID: "sess-scan-1", EventType: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"ls"}`, Cwd: "/home/user", Hostname: "h", HookDurationMs: 5, CreatedAt: pastTime.Add(time.Second).Format("2006-01-02T15:04:05.000Z")},
		{EventID: "e-scan-3", SessionID: "sess-scan-1", EventType: "Stop", Cwd: "/home/user", Hostname: "h", CreatedAt: pastTime.Add(time.Minute).Format("2006-01-02T15:04:05.000Z")},
	}
	for _, evt := range events {
		require.NoError(t, db.InsertEvent(ctx, evt))
	}

	// 先在 session_stats 中插入一条没有 ended_at 的记录
	pp := "/home/user"
	row := &storage.SessionStatsRow{
		SessionID:   "sess-scan-1",
		StartedAt:   events[0].CreatedAt,
		TotalEvents: 0, // 未聚合
		ProjectPath: &pp,
	}
	require.NoError(t, db.UpsertSessionStats(ctx, row))

	scanner := NewScanner(db)
	err := scanner.ScanAndRepair(ctx)
	require.NoError(t, err)

	// 验证 session_stats 表已补全
	rows, err := db.QuerySessionStats(ctx, storage.SessionFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	r := rows[0]
	assert.Equal(t, "sess-scan-1", r.SessionID)
	assert.Equal(t, 3, r.TotalEvents)
	assert.Equal(t, 1, r.ToolCalls)
}

// --- TestScannerScanAndRepair_已聚合session不重复 ---
func TestScannerScanAndRepair_已聚合session不重复(t *testing.T) {
	db := newScannerTestDB(t)
	ctx := context.Background()

	// 先手动聚合
	pastTime := time.Now().Add(-time.Hour).UTC()
	events := []*event.HookEvent{
		{EventID: "e-dup-1", SessionID: "sess-dup", EventType: "SessionStart", Cwd: "/home/user", Hostname: "h", CreatedAt: pastTime.Format("2006-01-02T15:04:05.000Z")},
		{EventID: "e-dup-2", SessionID: "sess-dup", EventType: "PreToolUse", ToolName: "Bash", Cwd: "/home/user", Hostname: "h", CreatedAt: pastTime.Add(time.Second).Format("2006-01-02T15:04:05.000Z")},
	}
	for _, evt := range events {
		require.NoError(t, db.InsertEvent(ctx, evt))
	}

	agg := NewAggregator(db)
	require.NoError(t, agg.AggregateFromEvents(ctx, events))

	// 扫描修复
	scanner := NewScanner(db)
	require.NoError(t, scanner.ScanAndRepair(ctx))

	// 验证只有一行
	rows, err := db.QuerySessionStats(ctx, storage.SessionFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

// --- TestScannerScanAndRepair_无数据不报错 ---
func TestScannerScanAndRepair_无数据不报错(t *testing.T) {
	db := newScannerTestDB(t)
	scanner := NewScanner(db)

	err := scanner.ScanAndRepair(t.Context())
	require.NoError(t, err)
}

// --- TestScannerTimeout_超时session被重新聚合 ---
func TestScannerTimeout_超时session被重新聚合(t *testing.T) {
	db := newScannerTestDB(t)
	ctx := context.Background()

	// 插入一个没有 Stop 事件的 session（模拟超时），使用 1 小时前的时间
	pastTime := time.Now().Add(-time.Hour).UTC()
	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "e-timeout-1", SessionID: "sess-timeout", EventType: "SessionStart",
		Cwd: "/home/user", Hostname: "h", CreatedAt: pastTime.Format("2006-01-02T15:04:05.000Z"),
	}))
	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "e-timeout-2", SessionID: "sess-timeout", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home/user", Hostname: "h", CreatedAt: pastTime.Add(time.Second).Format("2006-01-02T15:04:05.000Z"),
	}))

	// 先插入一条未聚合的 session_stats 记录
	pp := "/home/user"
	row := &storage.SessionStatsRow{
		SessionID:   "sess-timeout",
		StartedAt:   pastTime.Format("2006-01-02T15:04:05.000Z"),
		TotalEvents: 0,
		ProjectPath: &pp,
	}
	require.NoError(t, db.UpsertSessionStats(ctx, row))

	scanner := NewScanner(db)
	require.NoError(t, scanner.ScanAndRepair(ctx))

	// 验证超时 session 已被重新聚合，统计数据已更新
	rows, err := db.QuerySessionStats(ctx, storage.SessionFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 2, rows[0].TotalEvents, "超时 session 应该被重新聚合")
	assert.Equal(t, 1, rows[0].ToolCalls)
}
