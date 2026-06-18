package storage

var schemaV1 = `
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);

CREATE TABLE IF NOT EXISTS hook_events (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id          TEXT    NOT NULL,
    session_id        TEXT    NOT NULL,
    event_type        TEXT    NOT NULL,
    tool_name         TEXT    DEFAULT NULL,
    tool_input        TEXT    DEFAULT NULL,
    tool_output       TEXT    DEFAULT NULL,
    cwd               TEXT    NOT NULL,
    transcript_path   TEXT    DEFAULT NULL,
    blocked           INTEGER NOT NULL DEFAULT 0,
    block_reason      TEXT    DEFAULT NULL,
    hook_exit_code    INTEGER NOT NULL DEFAULT 0,
    hook_duration_ms  INTEGER NOT NULL DEFAULT 0,
    collect_duration_ms INTEGER NOT NULL DEFAULT 0,
    pid               INTEGER NOT NULL DEFAULT 0,
    hostname          TEXT    NOT NULL DEFAULT '',
    created_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_events_session      ON hook_events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_type         ON hook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_tool         ON hook_events(tool_name);
CREATE INDEX IF NOT EXISTS idx_events_time         ON hook_events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_blocked      ON hook_events(blocked);
CREATE INDEX IF NOT EXISTS idx_events_session_time ON hook_events(session_id, created_at);

CREATE TABLE IF NOT EXISTS stats_hourly (
    bucket_hour       TEXT    NOT NULL,
    event_type        TEXT    NOT NULL,
    tool_name         TEXT    DEFAULT NULL,
    event_count       INTEGER NOT NULL DEFAULT 0,
    block_count       INTEGER NOT NULL DEFAULT 0,
    avg_duration_ms   REAL    DEFAULT NULL,
    p50_duration_ms   REAL    DEFAULT NULL,
    p95_duration_ms   REAL    DEFAULT NULL,
    p99_duration_ms   REAL    DEFAULT NULL,
    PRIMARY KEY (bucket_hour, event_type, tool_name)
);

CREATE INDEX IF NOT EXISTS idx_stats_hourly_time ON stats_hourly(bucket_hour);

CREATE TABLE IF NOT EXISTS session_stats (
    session_id          TEXT    PRIMARY KEY,
    started_at          TEXT    NOT NULL,
    ended_at            TEXT    DEFAULT NULL,
    duration_secs       INTEGER DEFAULT 0,
    total_events        INTEGER NOT NULL DEFAULT 0,
    tool_calls          INTEGER NOT NULL DEFAULT 0,
    blocked_calls       INTEGER NOT NULL DEFAULT 0,
    block_rate          REAL    NOT NULL DEFAULT 0.0,
    tools_used          TEXT    DEFAULT '[]',
    avg_tool_duration_ms REAL   DEFAULT NULL,
    p99_tool_duration_ms REAL   DEFAULT NULL,
    project_path        TEXT    DEFAULT NULL
);

INSERT INTO schema_version VALUES (1);
`

// schemaMigrations 按版本号递增，每个版本在事务内执行
var schemaMigrations = []string{schemaV1}
