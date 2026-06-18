package storage

import "time"

// EventFilter 事件查询过滤条件
type EventFilter struct {
	SessionID *string    // 精确匹配
	EventType *string    // 精确匹配
	ToolName  *string    // 精确匹配
	Blocked   *bool      // 精确匹配
	Cwd       *string    // 按项目路径过滤
	Since     *time.Time // created_at >=
	Until     *time.Time // created_at <=
	Limit     int        // 默认 100，最大 10000
	Offset    int        // 分页偏移
	SortBy    string     // "created_at"(默认) | "hook_duration_ms"
	SortOrder string     // "asc" | "desc"(默认)
}

// StatsFilter 统计查询过滤条件
type StatsFilter struct {
	Since     *time.Time // bucket_hour >=
	Until     *time.Time // bucket_hour <=
	EventType *string    // 精确匹配
	ToolName  *string    // 精确匹配
}

// SessionFilter 会话查询过滤条件
type SessionFilter struct {
	ProjectPath *string    // 精确匹配
	Since       *time.Time // started_at >=
	Until       *time.Time // started_at <=
	SortBy      string     // "started_at"(默认) | "total_events" | "duration_secs"
	SortOrder   string     // "asc" | "desc"(默认)
	Limit       int        // 默认 100，最大 1000
	Offset      int        // 分页偏移
}

// SessionSummary 会话列表摘要
type SessionSummary struct {
	SessionID   string    `json:"session_id"`
	StartedAt   time.Time `json:"started_at"`
	TotalEvents int       `json:"total_events"`
	DurationSec int64     `json:"duration_secs"`
	Blocked     int       `json:"blocked"`
}

// StatsHourlyRow stats_hourly 表的一行
type StatsHourlyRow struct {
	BucketHour    string  `json:"bucket_hour"    db:"bucket_hour"`
	EventType     string  `json:"event_type"     db:"event_type"`
	ToolName      *string `json:"tool_name"      db:"tool_name"`
	EventCount    int     `json:"event_count"    db:"event_count"`
	BlockCount    int     `json:"block_count"    db:"block_count"`
	AvgDurationMs *float64 `json:"avg_duration_ms" db:"avg_duration_ms"`
	P50DurationMs *float64 `json:"p50_duration_ms" db:"p50_duration_ms"`
	P95DurationMs *float64 `json:"p95_duration_ms" db:"p95_duration_ms"`
	P99DurationMs *float64 `json:"p99_duration_ms" db:"p99_duration_ms"`
}

// SessionStatsRow session_stats 表的一行
type SessionStatsRow struct {
	SessionID        string   `json:"session_id"         db:"session_id"`
	StartedAt        string   `json:"started_at"         db:"started_at"`
	EndedAt          *string  `json:"ended_at"           db:"ended_at"`
	DurationSecs     int64    `json:"duration_secs"      db:"duration_secs"`
	TotalEvents      int      `json:"total_events"       db:"total_events"`
	ToolCalls        int      `json:"tool_calls"         db:"tool_calls"`
	BlockedCalls     int      `json:"blocked_calls"      db:"blocked_calls"`
	BlockRate        float64  `json:"block_rate"         db:"block_rate"`
	ToolsUsed        string   `json:"tools_used"         db:"tools_used"`
	AvgToolDurationMs *float64 `json:"avg_tool_duration_ms" db:"avg_tool_duration_ms"`
	P99ToolDurationMs *float64 `json:"p99_tool_duration_ms" db:"p99_tool_duration_ms"`
	ProjectPath      *string  `json:"project_path"       db:"project_path"`
}
