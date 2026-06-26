package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dartagnanli/agent-insight/internal/config"
	"github.com/dartagnanli/agent-insight/pkg/event"
)

// SQLite 实现 Storage 接口
type SQLite struct {
	db  *sql.DB
	cfg config.StorageConfig
}

// NewSQLite 创建 SQLite 存储实例
func NewSQLite(ctx context.Context, cfg config.StorageConfig) (*SQLite, error) {
	dbPath := cfg.Path
	if dbPath == "" {
		return nil, fmt.Errorf("db path must not be empty")
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_journal_size_limit=67108864&_cache_size=-32000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &SQLite{db: db, cfg: cfg}
	if err := s.Init(ctx); err != nil {
		db.Close()
		return nil, err
	}

	// 设置数据库文件权限 0600
	if err := os.Chmod(dbPath, 0600); err != nil {
		slog.Warn("failed to set db file permission", "path", dbPath, "error", err)
	}

	return s, nil
}

// NewStorage 根据配置创建存储实例
func NewStorage(ctx context.Context, cfg config.StorageConfig) (Storage, error) {
	switch cfg.Type {
	case "sqlite":
		return NewSQLite(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}

func (s *SQLite) Init(ctx context.Context) error {
	// 检测数据库完整性
	var ok string
	err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok)
	if err != nil || ok != "ok" {
		dbPath := s.cfg.Path
		if dbPath != "" {
			backupPath := dbPath + ".corrupt." + time.Now().Format("20060102-150405")
			if renameErr := os.Rename(dbPath, backupPath); renameErr == nil {
				slog.Warn("corrupt database backed up", "backup", backupPath)
			}
		}
		// 重建连接并迁移
		s.db.Close()
		dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_journal_size_limit=67108864&_cache_size=-32000", dbPath)
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return fmt.Errorf("reopen sqlite after corrupt: %w", err)
		}
		s.db = db
	}

	return s.migrate(ctx)
}

func (s *SQLite) migrate(ctx context.Context) error {
	var currentVersion int
	err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		currentVersion = 0
	}

	for i := currentVersion; i < len(schemaMigrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration tx v%d: %w", i+1, err)
		}
		if _, err := tx.Exec(schemaMigrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("exec migration v%d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration v%d: %w", i+1, err)
		}
		slog.Info("database migrated", "version", i+1)
	}
	return nil
}

func (s *SQLite) InsertEvent(ctx context.Context, ev *event.HookEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hook_events (event_id, session_id, event_type, tool_name, tool_input, tool_output,
		 cwd, transcript_path, blocked, block_reason, hook_exit_code, hook_duration_ms,
		 collect_duration_ms, pid, hostname, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.EventID, ev.SessionID, ev.EventType, ev.ToolName, ev.ToolInput, ev.ToolOutput,
		ev.Cwd, ev.TranscriptPath, boolToInt(ev.Blocked), ev.BlockReason, ev.HookExitCode,
		ev.HookDurationMs, ev.CollectDurationMs, ev.Pid, ev.Hostname, ev.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (s *SQLite) QueryEvents(ctx context.Context, f EventFilter) ([]*event.HookEvent, error) {
	q := "SELECT id, event_id, session_id, event_type, tool_name, tool_input, tool_output, " +
		"cwd, transcript_path, blocked, block_reason, hook_exit_code, hook_duration_ms, " +
		"collect_duration_ms, pid, hostname, created_at FROM hook_events WHERE 1=1"
	args := []any{}

	if f.SessionID != nil {
		q += " AND session_id = ?"
		args = append(args, *f.SessionID)
	}
	if f.EventType != nil {
		q += " AND event_type = ?"
		args = append(args, *f.EventType)
	}
	if f.ToolName != nil {
		q += " AND tool_name = ?"
		args = append(args, *f.ToolName)
	}
	if f.Blocked != nil {
		q += " AND blocked = ?"
		args = append(args, boolToInt(*f.Blocked))
	}
	if f.Cwd != nil {
		q += " AND cwd = ?"
		args = append(args, *f.Cwd)
	}
	if f.Since != nil {
		q += " AND created_at >= ?"
		args = append(args, f.Since.Format(time.RFC3339Nano))
	}
	if f.Until != nil {
		q += " AND created_at <= ?"
		args = append(args, f.Until.Format(time.RFC3339Nano))
	}

	sortBy := "created_at"
	if f.SortBy == "hook_duration_ms" {
		sortBy = "hook_duration_ms"
	}
	sortOrder := "DESC"
	if f.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	q += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	limit := 100
	if f.Limit > 0 && f.Limit <= 10000 {
		limit = f.Limit
	}
	q += " LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []*event.HookEvent
	for rows.Next() {
		ev := &event.HookEvent{}
		var blocked int
		err := rows.Scan(&ev.ID, &ev.EventID, &ev.SessionID, &ev.EventType, &ev.ToolName,
			&ev.ToolInput, &ev.ToolOutput, &ev.Cwd, &ev.TranscriptPath, &blocked,
			&ev.BlockReason, &ev.HookExitCode, &ev.HookDurationMs, &ev.CollectDurationMs,
			&ev.Pid, &ev.Hostname, &ev.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		ev.Blocked = blocked == 1
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *SQLite) CountEvents(ctx context.Context, f EventFilter) (int, error) {
	q := "SELECT COUNT(*) FROM hook_events WHERE 1=1"
	args := []any{}

	if f.SessionID != nil {
		q += " AND session_id = ?"
		args = append(args, *f.SessionID)
	}
	if f.EventType != nil {
		q += " AND event_type = ?"
		args = append(args, *f.EventType)
	}
	if f.ToolName != nil {
		q += " AND tool_name = ?"
		args = append(args, *f.ToolName)
	}
	if f.Blocked != nil {
		q += " AND blocked = ?"
		args = append(args, boolToInt(*f.Blocked))
	}
	if f.Cwd != nil {
		q += " AND cwd = ?"
		args = append(args, *f.Cwd)
	}
	if f.Since != nil {
		q += " AND created_at >= ?"
		args = append(args, f.Since.Format(time.RFC3339Nano))
	}
	if f.Until != nil {
		q += " AND created_at <= ?"
		args = append(args, f.Until.Format(time.RFC3339Nano))
	}

	var count int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count events: %w", err)
	}
	return count, nil
}

func (s *SQLite) QueryRecentEvents(ctx context.Context, afterID int64, limit int) ([]*event.HookEvent, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, event_id, session_id, event_type, tool_name, tool_input, tool_output, "+
			"cwd, transcript_path, blocked, block_reason, hook_exit_code, hook_duration_ms, "+
			"collect_duration_ms, pid, hostname, created_at FROM hook_events WHERE id > ? ORDER BY id ASC LIMIT ?",
		afterID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent events: %w", err)
	}
	defer rows.Close()

	var events []*event.HookEvent
	for rows.Next() {
		ev := &event.HookEvent{}
		var blocked int
		err := rows.Scan(&ev.ID, &ev.EventID, &ev.SessionID, &ev.EventType, &ev.ToolName,
			&ev.ToolInput, &ev.ToolOutput, &ev.Cwd, &ev.TranscriptPath, &blocked,
			&ev.BlockReason, &ev.HookExitCode, &ev.HookDurationMs, &ev.CollectDurationMs,
			&ev.Pid, &ev.Hostname, &ev.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan recent event: %w", err)
		}
		ev.Blocked = blocked == 1
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *SQLite) GetEvent(ctx context.Context, eventID string) (*event.HookEvent, error) {
	ev := &event.HookEvent{}
	var blocked int
	err := s.db.QueryRowContext(ctx,
		"SELECT id, event_id, session_id, event_type, tool_name, tool_input, tool_output, "+
			"cwd, transcript_path, blocked, block_reason, hook_exit_code, hook_duration_ms, "+
			"collect_duration_ms, pid, hostname, created_at FROM hook_events WHERE event_id = ?",
		eventID,
	).Scan(&ev.ID, &ev.EventID, &ev.SessionID, &ev.EventType, &ev.ToolName,
		&ev.ToolInput, &ev.ToolOutput, &ev.Cwd, &ev.TranscriptPath, &blocked,
		&ev.BlockReason, &ev.HookExitCode, &ev.HookDurationMs, &ev.CollectDurationMs,
		&ev.Pid, &ev.Hostname, &ev.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}
	ev.Blocked = blocked == 1
	return ev, nil
}

func (s *SQLite) UpsertStatsHourly(ctx context.Context, rows []StatsHourlyRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO stats_hourly (bucket_hour, event_type, tool_name, event_count, block_count,
		 avg_duration_ms, p50_duration_ms, p95_duration_ms, p99_duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(bucket_hour, event_type, tool_name) DO UPDATE SET
		 event_count=excluded.event_count, block_count=excluded.block_count,
		 avg_duration_ms=excluded.avg_duration_ms, p50_duration_ms=excluded.p50_duration_ms,
		 p95_duration_ms=excluded.p95_duration_ms, p99_duration_ms=excluded.p99_duration_ms`)
	if err != nil {
		return fmt.Errorf("prepare upsert stats: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		_, err := stmt.ExecContext(ctx, r.BucketHour, r.EventType, r.ToolName,
			r.EventCount, r.BlockCount, r.AvgDurationMs, r.P50DurationMs,
			r.P95DurationMs, r.P99DurationMs)
		if err != nil {
			return fmt.Errorf("upsert stats hourly: %w", err)
		}
	}
	return tx.Commit()
}

func (s *SQLite) UpsertSessionStats(ctx context.Context, row *SessionStatsRow) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO session_stats (session_id, started_at, ended_at, duration_secs,
		 total_events, tool_calls, blocked_calls, block_rate, tools_used,
		 avg_tool_duration_ms, p99_tool_duration_ms, project_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		 ended_at=excluded.ended_at, duration_secs=excluded.duration_secs,
		 total_events=excluded.total_events, tool_calls=excluded.tool_calls,
		 blocked_calls=excluded.blocked_calls, block_rate=excluded.block_rate,
		 tools_used=excluded.tools_used, avg_tool_duration_ms=excluded.avg_tool_duration_ms,
		 p99_tool_duration_ms=excluded.p99_tool_duration_ms, project_path=excluded.project_path`,
		row.SessionID, row.StartedAt, row.EndedAt, row.DurationSecs,
		row.TotalEvents, row.ToolCalls, row.BlockedCalls, row.BlockRate,
		row.ToolsUsed, row.AvgToolDurationMs, row.P99ToolDurationMs, row.ProjectPath,
	)
	if err != nil {
		return fmt.Errorf("upsert session stats: %w", err)
	}
	return nil
}

func (s *SQLite) QueryStatsHourly(ctx context.Context, f StatsFilter) ([]StatsHourlyRow, error) {
	q := "SELECT bucket_hour, event_type, tool_name, event_count, block_count, " +
		"avg_duration_ms, p50_duration_ms, p95_duration_ms, p99_duration_ms " +
		"FROM stats_hourly WHERE 1=1"
	args := []any{}

	if f.Since != nil {
		q += " AND bucket_hour >= ?"
		args = append(args, f.Since.Format(time.RFC3339))
	}
	if f.Until != nil {
		q += " AND bucket_hour <= ?"
		args = append(args, f.Until.Format(time.RFC3339))
	}
	if f.EventType != nil {
		q += " AND event_type = ?"
		args = append(args, *f.EventType)
	}
	if f.ToolName != nil {
		q += " AND tool_name = ?"
		args = append(args, *f.ToolName)
	}
	q += " ORDER BY bucket_hour ASC"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query stats hourly: %w", err)
	}
	defer rows.Close()

	var result []StatsHourlyRow
	for rows.Next() {
		var r StatsHourlyRow
		var toolName sql.NullString
		err := rows.Scan(&r.BucketHour, &r.EventType, &toolName, &r.EventCount,
			&r.BlockCount, &r.AvgDurationMs, &r.P50DurationMs, &r.P95DurationMs, &r.P99DurationMs)
		if err != nil {
			return nil, fmt.Errorf("scan stats hourly: %w", err)
		}
		if toolName.Valid {
			t := toolName.String
			r.ToolName = &t
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SQLite) QuerySessionStats(ctx context.Context, f SessionFilter) ([]SessionStatsRow, error) {
	q := "SELECT session_id, started_at, ended_at, duration_secs, total_events, tool_calls, " +
		"blocked_calls, block_rate, tools_used, avg_tool_duration_ms, p99_tool_duration_ms, " +
		"project_path FROM session_stats WHERE 1=1"
	args := []any{}

	if f.ProjectPath != nil {
		q += " AND project_path = ?"
		args = append(args, *f.ProjectPath)
	}
	if f.Since != nil {
		q += " AND started_at >= ?"
		args = append(args, f.Since.Format(time.RFC3339))
	}
	if f.Until != nil {
		q += " AND started_at <= ?"
		args = append(args, f.Until.Format(time.RFC3339))
	}

	sortBy := "started_at"
	switch f.SortBy {
	case "total_events":
		sortBy = "total_events"
	case "duration_secs":
		sortBy = "duration_secs"
	}
	sortOrder := "DESC"
	if f.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	q += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	limit := 100
	if f.Limit > 0 && f.Limit <= 1000 {
		limit = f.Limit
	}
	q += " LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query session stats: %w", err)
	}
	defer rows.Close()

	var result []SessionStatsRow
	for rows.Next() {
		var r SessionStatsRow
		var endedAt, projectPath sql.NullString
		var avgDur, p99Dur sql.NullFloat64
		err := rows.Scan(&r.SessionID, &r.StartedAt, &endedAt, &r.DurationSecs,
			&r.TotalEvents, &r.ToolCalls, &r.BlockedCalls, &r.BlockRate,
			&r.ToolsUsed, &avgDur, &p99Dur, &projectPath)
		if err != nil {
			return nil, fmt.Errorf("scan session stats: %w", err)
		}
		if endedAt.Valid {
			r.EndedAt = &endedAt.String
		}
		if projectPath.Valid {
			r.ProjectPath = &projectPath.String
		}
		if avgDur.Valid {
			r.AvgToolDurationMs = &avgDur.Float64
		}
		if p99Dur.Valid {
			r.P99ToolDurationMs = &p99Dur.Float64
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SQLite) ListSessions(ctx context.Context, f SessionFilter) ([]SessionStatsRow, error) {
	q := "SELECT session_id, started_at, ended_at, duration_secs, total_events, tool_calls, " +
		"blocked_calls, block_rate, tools_used, avg_tool_duration_ms, p99_tool_duration_ms, " +
		"project_path FROM session_stats WHERE 1=1"
	args := []any{}

	if f.ProjectPath != nil {
		q += " AND project_path = ?"
		args = append(args, *f.ProjectPath)
	}
	if f.Since != nil {
		q += " AND started_at >= ?"
		args = append(args, f.Since.Format(time.RFC3339))
	}
	if f.Until != nil {
		q += " AND started_at <= ?"
		args = append(args, f.Until.Format(time.RFC3339))
	}

	sortBy := "started_at"
	switch f.SortBy {
	case "total_events":
		sortBy = "total_events"
	case "duration_secs":
		sortBy = "duration_secs"
	}
	sortOrder := "DESC"
	if f.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	q += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	limit := 100
	if f.Limit > 0 && f.Limit <= 1000 {
		limit = f.Limit
	}
	q += " LIMIT ? OFFSET ?"
	args = append(args, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var result []SessionStatsRow
	for rows.Next() {
		var r SessionStatsRow
		var endedAt, projectPath sql.NullString
		var avgDur, p99Dur sql.NullFloat64
		err := rows.Scan(&r.SessionID, &r.StartedAt, &endedAt, &r.DurationSecs,
			&r.TotalEvents, &r.ToolCalls, &r.BlockedCalls, &r.BlockRate,
			&r.ToolsUsed, &avgDur, &p99Dur, &projectPath)
		if err != nil {
			return nil, fmt.Errorf("scan session stats: %w", err)
		}
		if endedAt.Valid {
			r.EndedAt = &endedAt.String
		}
		if projectPath.Valid {
			r.ProjectPath = &projectPath.String
		}
		if avgDur.Valid {
			r.AvgToolDurationMs = &avgDur.Float64
		}
		if p99Dur.Valid {
			r.P99ToolDurationMs = &p99Dur.Float64
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SQLite) CountSessions(ctx context.Context, f SessionFilter) (int, error) {
	q := "SELECT COUNT(*) FROM session_stats WHERE 1=1"
	args := []any{}

	if f.ProjectPath != nil {
		q += " AND project_path = ?"
		args = append(args, *f.ProjectPath)
	}
	if f.Since != nil {
		q += " AND started_at >= ?"
		args = append(args, f.Since.Format(time.RFC3339))
	}
	if f.Until != nil {
		q += " AND started_at <= ?"
		args = append(args, f.Until.Format(time.RFC3339))
	}

	var count int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return count, nil
}

func (s *SQLite) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	ts := before.UTC().Format("2006-01-02T15:04:05.000Z")

	res, err := s.db.ExecContext(ctx, "DELETE FROM hook_events WHERE created_at < ?", ts)
	if err != nil {
		return 0, fmt.Errorf("delete old events: %w", err)
	}
	affected, _ := res.RowsAffected()

	s.db.ExecContext(ctx, "DELETE FROM stats_hourly WHERE bucket_hour < ?", ts)
	s.db.ExecContext(ctx, "DELETE FROM session_stats WHERE ended_at < ?", ts)

	// 回收 WAL 空间
	s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	s.db.ExecContext(ctx, "PRAGMA optimize")

	return affected, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ParseDuration 解析人类可读的持续时间字符串（如 "1h", "24h", "7d"）
func ParseDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	var val int
	var unit string
	if _, err := fmt.Sscanf(s, "%d%s", &val, &unit); err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	switch unit {
	case "d":
		return time.Duration(val) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported duration unit %q", unit)
	}
}
