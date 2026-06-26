package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/internal/config"
	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDashboardDB 创建内存 SQLite 数据库用于 dashboard handler 测试
func newTestDashboardDB(t *testing.T) storage.Storage {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "insight.db")
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := storage.NewSQLite(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// insertTestEvents 批量插入测试事件
func insertTestEvents(t *testing.T, s storage.Storage, count int, baseTime time.Time) {
	t.Helper()
	ctx := t.Context()
	for i := 0; i < count; i++ {
		evt := &event.HookEvent{
			EventID:   fmt.Sprintf("evt-api-%d", i),
			SessionID: "sess-api-test",
			EventType: "PreToolUse",
			ToolName:  "Bash",
			ToolInput: `{"command":"ls"}`,
			Cwd:       "/home/user",
			Hostname:  "test-host",
			CreatedAt: baseTime.Add(time.Duration(i) * time.Second).Format("2006-01-02T15:04:05.000Z"),
		}
		if i%5 == 0 {
			evt.Blocked = true
			evt.BlockReason = "style check"
		}
		require.NoError(t, s.InsertEvent(ctx, evt))
	}
}

// --- TestHandleListEvents_返回JSON结构 ---
func TestHandleListEvents_返回JSON结构(t *testing.T) {
	db := newTestDashboardDB(t)
	insertTestEvents(t, db, 10, time.Now().Add(-5*time.Minute))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleListEvents))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?limit=5&offset=0")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Events []*event.HookEvent `json:"events"`
		Total  int                `json:"total"`
		Limit  int                `json:"limit"`
		Offset int                `json:"offset"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, 5, result.Limit)
	assert.Equal(t, 0, result.Offset)
	assert.Equal(t, 10, result.Total)
	assert.Len(t, result.Events, 5)
}

// --- TestHandleListEvents_分页 ---
func TestHandleListEvents_分页(t *testing.T) {
	db := newTestDashboardDB(t)
	insertTestEvents(t, db, 15, time.Now().Add(-5*time.Minute))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleListEvents))
	defer srv.Close()

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
		wantTotal int
	}{
		{"第一页", 5, 0, 5, 15},
		{"第二页", 5, 5, 5, 15},
		{"最后一页", 5, 10, 5, 15},
		{"超出范围", 5, 15, 0, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := fmt.Sprintf("%s?limit=%d&offset=%d", srv.URL, tt.limit, tt.offset)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer resp.Body.Close()

			var result struct {
				Events []*event.HookEvent `json:"events"`
				Total  int                `json:"total"`
			}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)

			assert.Len(t, result.Events, tt.wantCount)
			assert.Equal(t, tt.wantTotal, result.Total)
		})
	}
}

// --- TestHandleListEvents_按event_type过滤 ---
func TestHandleListEvents_按event_type过滤(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-filter-1", SessionID: "sess-f", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))
	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-filter-2", SessionID: "sess-f", EventType: "PostToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleListEvents))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?event_type=PreToolUse")
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Events []*event.HookEvent `json:"events"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	assert.Equal(t, "PreToolUse", result.Events[0].EventType)
}

// --- TestHandleListEvents_按tool_name过滤 ---
func TestHandleListEvents_按tool_name过滤(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-tn-1", SessionID: "sess-tn", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))
	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-tn-2", SessionID: "sess-tn", EventType: "PreToolUse",
		ToolName: "Write", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleListEvents))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?tool_name=Bash")
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Events []*event.HookEvent `json:"events"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	assert.Equal(t, "Bash", result.Events[0].ToolName)
}

// --- TestHandleListEvents_按blocked过滤 ---
func TestHandleListEvents_按blocked过滤(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-blk-1", SessionID: "sess-blk", EventType: "PreToolUse",
		ToolName: "Write", Cwd: "/home", Hostname: "h", Blocked: true, BlockReason: "deny", CreatedAt: event.Now(),
	}))
	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-blk-2", SessionID: "sess-blk", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleListEvents))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?blocked=true")
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Events []*event.HookEvent `json:"events"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	require.Len(t, result.Events, 1)
	assert.True(t, result.Events[0].Blocked)
}

// --- TestHandleListEvents_时间范围过滤 ---
func TestHandleListEvents_时间范围过滤(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	recent := now.Add(-5 * time.Minute)

	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-time-1", SessionID: "sess-time", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h",
		CreatedAt: past.Format("2006-01-02T15:04:05.000Z"),
	}))
	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-time-2", SessionID: "sess-time", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h",
		CreatedAt: recent.Format("2006-01-02T15:04:05.000Z"),
	}))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleListEvents))
	defer srv.Close()

	since := now.Add(-1 * time.Hour)
	url := fmt.Sprintf("%s?since=%s", srv.URL, since.Format(time.RFC3339))
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Events []*event.HookEvent `json:"events"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Len(t, result.Events, 1) // 只有 recent 的那条
}

// --- TestHandleGetEvent_200返回 ---
func TestHandleGetEvent_200返回(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-get-1", SessionID: "sess-get", EventType: "PreToolUse",
		ToolName: "Bash", ToolInput: `{"command":"ls"}`, Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))

	h := NewAPIHandler(db)
	// HandleGetEvent 使用 r.PathValue("eventID")，需要用 ServeMux 注册
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{eventID}", h.HandleGetEvent)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/evt-get-1")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var evt event.HookEvent
	err = json.NewDecoder(resp.Body).Decode(&evt)
	require.NoError(t, err)
	assert.Equal(t, "evt-get-1", evt.EventID)
	assert.Equal(t, "Bash", evt.ToolName)
}

// --- TestHandleGetEvent_404返回 ---
func TestHandleGetEvent_404返回(t *testing.T) {
	db := newTestDashboardDB(t)

	h := NewAPIHandler(db)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{eventID}", h.HandleGetEvent)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nonexistent-id")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- TestHandleStats_返回JSON字段 ---
func TestHandleStats_返回JSON字段(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-stat-1", SessionID: "sess-stat", EventType: "PreToolUse",
		ToolName: "Bash", Cwd: "/home", Hostname: "h", CreatedAt: event.Now(),
	}))
	require.NoError(t, db.InsertEvent(ctx, &event.HookEvent{
		EventID: "evt-stat-2", SessionID: "sess-stat", EventType: "PreToolUse",
		ToolName: "Write", Cwd: "/home", Hostname: "h", Blocked: true, CreatedAt: event.Now(),
	}))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleStats))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		TotalEvents           int            `json:"total_events"`
		BlockRate             float64        `json:"block_rate"`
		ToolDistribution      map[string]int `json:"tool_distribution"`
		EventTypeDistribution map[string]int `json:"event_type_distribution"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, 2, result.TotalEvents)
	assert.GreaterOrEqual(t, result.BlockRate, 0.0)
	assert.Contains(t, result.ToolDistribution, "Bash")
	assert.Contains(t, result.ToolDistribution, "Write")
	assert.Contains(t, result.EventTypeDistribution, "PreToolUse")
}

// --- TestHandleStatsHourly_返回buckets ---
func TestHandleStatsHourly_返回buckets(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	bucket := "2026-06-18T10:00:00Z"
	toolName := "Bash"
	avg := 15.0
	p50 := 10.0
	p95 := 30.0
	p99 := 45.0

	rows := []storage.StatsHourlyRow{
		{
			BucketHour:    bucket,
			EventType:     "PreToolUse",
			ToolName:      &toolName,
			EventCount:    50,
			BlockCount:    3,
			AvgDurationMs: &avg,
			P50DurationMs: &p50,
			P95DurationMs: &p95,
			P99DurationMs: &p99,
		},
	}
	require.NoError(t, db.UpsertStatsHourly(ctx, rows))

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleStatsHourly))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Buckets []storage.StatsHourlyRow `json:"buckets"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Buckets)
	assert.Equal(t, bucket, result.Buckets[0].BucketHour)
	assert.Equal(t, 50, result.Buckets[0].EventCount)
}

// --- TestHandleTrace_200返回 ---
func TestHandleTrace_200返回(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	events := []*event.HookEvent{
		{EventID: "tr-pre-1", SessionID: "sess-trace", EventType: "PreToolUse", ToolName: "Bash", ToolInput: `{"command":"ls"}`, Cwd: "/home", Hostname: "h", CreatedAt: "2026-06-18T10:00:00.000Z"},
		{EventID: "tr-post-1", SessionID: "sess-trace", EventType: "PostToolUse", ToolName: "Bash", ToolOutput: `{"exit_code":0}`, Cwd: "/home", Hostname: "h", CreatedAt: "2026-06-18T10:00:01.000Z"},
		{EventID: "tr-start", SessionID: "sess-trace", EventType: "SessionStart", Cwd: "/home", Hostname: "h", CreatedAt: "2026-06-18T09:59:59.000Z"},
	}
	for _, evt := range events {
		require.NoError(t, db.InsertEvent(ctx, evt))
	}

	h := NewAPIHandler(db)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{sessionID}", h.HandleTrace)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sess-trace")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Spans            []*event.Span            `json:"spans"`
		StandaloneEvents []*event.StandaloneEvent `json:"standalone_events"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Spans)
	assert.NotEmpty(t, result.StandaloneEvents)
}

// --- TestHandleTrace_404返回 ---
func TestHandleTrace_404返回(t *testing.T) {
	db := newTestDashboardDB(t)

	h := NewAPIHandler(db)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{sessionID}", h.HandleTrace)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nonexistent-session")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- TestHandleListSessions_返回数组和total ---
func TestHandleListSessions_返回数组和total(t *testing.T) {
	db := newTestDashboardDB(t)
	ctx := t.Context()

	for i, sid := range []string{"sess-ls-1", "sess-ls-2", "sess-ls-3"} {
		row := &storage.SessionStatsRow{
			SessionID:   sid,
			StartedAt:   event.Now(),
			TotalEvents: (i + 1) * 5,
			ToolCalls:   (i + 1) * 3,
			BlockedCalls: i,
			BlockRate:    float64(i) / float64((i+1)*3),
			ToolsUsed:    `["Bash"]`,
		}
		require.NoError(t, db.UpsertSessionStats(ctx, row))
	}

	h := NewAPIHandler(db)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleListSessions))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?limit=10")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Sessions []storage.SessionStatsRow `json:"sessions"`
		Total   int                        `json:"total"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Len(t, result.Sessions, 3)
	assert.Equal(t, 3, result.Total)
}
