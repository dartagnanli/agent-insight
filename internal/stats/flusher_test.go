package stats

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

// newFlusherTestDB 创建内存 SQLite 数据库用于 Flusher 测试
func newFlusherTestDB(t *testing.T) *storage.SQLite {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := storage.NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// insertEventsForFlusher 插入一批事件供 flush 聚合
func insertEventsForFlusher(t *testing.T, db *storage.SQLite, count int, baseTime time.Time) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < count; i++ {
		evt := &event.HookEvent{
			EventID:         "evt-flush-" + time.Now().Format("150405") + "-" + string(rune(i)),
			SessionID:       "sess-flush",
			EventType:       "PreToolUse",
			ToolName:        "Bash",
			Cwd:             "/home/user",
			Hostname:        "test-host",
			HookDurationMs:  i * 10,
			CollectDurationMs: i * 10,
			CreatedAt:       baseTime.Add(time.Duration(i) * time.Second).Format("2006-01-02T15:04:05.000Z"),
		}
		require.NoError(t, db.InsertEvent(ctx, evt))
	}
}

// --- TestFlusherFlush_flush后stats_hourly表有数据 ---
func TestFlusherFlush_flush后stats_hourly表有数据(t *testing.T) {
	db := newFlusherTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	insertEventsForFlusher(t, db, 20, now)

	flusher := NewFlusher(db)

	err := flusher.Flush(ctx)
	require.NoError(t, err)

	// 验证 stats_hourly 表有数据
	rows, err := db.QueryStatsHourly(ctx, storage.StatsFilter{})
	require.NoError(t, err)
	assert.NotEmpty(t, rows, "flush 后 stats_hourly 表应该有数据")

	if len(rows) > 0 {
		row := rows[0]
		assert.Greater(t, row.EventCount, 0)
		assert.Equal(t, "PreToolUse", row.EventType)
	}
}

// --- TestFlusherStart_启动后定时flush ---
func TestFlusherStart_启动后定时flush(t *testing.T) {
	db := newFlusherTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	insertEventsForFlusher(t, db, 10, now)

	flusher := NewFlusher(db)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		flusher.Start(ctx, 100*time.Millisecond)
		close(done)
	}()

	// 等待至少一次 flush 周期
	time.Sleep(300 * time.Millisecond)

	rows, err := db.QueryStatsHourly(ctx, storage.StatsFilter{})
	require.NoError(t, err)
	assert.NotEmpty(t, rows, "启动 flusher 后 stats_hourly 表应该有数据")

	cancel()
	<-done
}

// --- TestFlusherFlush_空数据库不报错 ---
func TestFlusherFlush_空数据库不报错(t *testing.T) {
	db := newFlusherTestDB(t)
	flusher := NewFlusher(db)

	err := flusher.Flush(t.Context())
	require.NoError(t, err)

	rows, err := db.QueryStatsHourly(t.Context(), storage.StatsFilter{})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// --- TestFlusherFlush_多次flush不重复 ---
func TestFlusherFlush_多次flush不重复(t *testing.T) {
	db := newFlusherTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	insertEventsForFlusher(t, db, 10, now)

	flusher := NewFlusher(db)

	require.NoError(t, flusher.Flush(ctx))
	require.NoError(t, flusher.Flush(ctx))

	rows, err := db.QueryStatsHourly(ctx, storage.StatsFilter{})
	require.NoError(t, err)
	assert.NotEmpty(t, rows)

	// 由于 UPSERT 语义，重复 flush 不应产生重复行
	uniqueKeys := make(map[string]bool)
	for _, row := range rows {
		key := row.BucketHour + "|" + row.EventType
		if row.ToolName != nil {
			key += "|" + *row.ToolName
		}
		uniqueKeys[key] = true
	}
	assert.Equal(t, len(rows), len(uniqueKeys), "多次 flush 不应产生重复的聚合行")
}
