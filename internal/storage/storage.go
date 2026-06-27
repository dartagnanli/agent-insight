package storage

import (
	"context"
	"time"

	"github.com/dartagnanli/agent-insight/pkg/event"
)

// Storage 存储接口，支持未来替换为 Postgres/ClickHouse
type Storage interface {
	Init(ctx context.Context) error
	InsertEvent(ctx context.Context, ev *event.HookEvent) error
	QueryEvents(ctx context.Context, filter EventFilter) ([]*event.HookEvent, error)
	CountEvents(ctx context.Context, filter EventFilter) (int, error)
	QueryRecentEvents(ctx context.Context, afterID int64, limit int) ([]*event.HookEvent, error)
	GetEvent(ctx context.Context, eventID string) (*event.HookEvent, error)
	UpsertStatsHourly(ctx context.Context, rows []StatsHourlyRow) error
	UpsertSessionStats(ctx context.Context, row *SessionStatsRow) error
	QueryStatsHourly(ctx context.Context, filter StatsFilter) ([]StatsHourlyRow, error)
	QuerySessionStats(ctx context.Context, filter SessionFilter) ([]SessionStatsRow, error)
	ListSessions(ctx context.Context, filter SessionFilter) ([]SessionStatsRow, error)
	CountSessions(ctx context.Context, filter SessionFilter) (int, error)
	DistinctSessions(ctx context.Context) ([]string, error)
	DeleteBefore(ctx context.Context, before time.Time) (int64, error)
	Close() error
}
