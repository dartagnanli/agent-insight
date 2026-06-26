package session

import (
	"context"
	"log/slog"
	"time"

	"github.com/dartagnanli/agent-insight/internal/storage"
)

// Scanner 扫描并修复未正常结束的 session
type Scanner struct {
	storage    storage.Storage
	aggregator *Aggregator
}

// NewScanner 创建 session 扫描器
func NewScanner(s storage.Storage) *Scanner {
	return &Scanner{
		storage:    s,
		aggregator: NewAggregator(s),
	}
}

// ScanAndRepair 查找 ended_at IS NULL 的 session，检查超时后聚合修复
func (s *Scanner) ScanAndRepair(ctx context.Context) error {
	// 查询所有 session
	filter := storage.SessionFilter{Limit: 1000}
	sessions, err := s.storage.ListSessions(ctx, filter)
	if err != nil {
		return err
	}

	timeoutThreshold := 30 * time.Minute
	now := time.Now()

	for _, sess := range sessions {
		if sess.EndedAt != nil {
			continue
		}

		// 检查 session 是否超时
		startedAt, err := time.Parse("2006-01-02T15:04:05.000Z", sess.StartedAt)
		if err != nil {
			startedAt, _ = time.Parse(time.RFC3339, sess.StartedAt)
		}
		if startedAt.IsZero() {
			continue
		}

		if now.Sub(startedAt) > timeoutThreshold {
			// 超时 session：聚合事件并更新统计
			sid := sess.SessionID
			evtFilter := storage.EventFilter{SessionID: &sid, Limit: 10000, SortOrder: "asc"}
			events, err := s.storage.QueryEvents(ctx, evtFilter)
			if err != nil {
				slog.Warn("scanner: query events failed", "session_id", sid, "error", err)
				continue
			}
			if len(events) == 0 {
				continue
			}

			if err := s.aggregator.AggregateFromEvents(ctx, events); err != nil {
				slog.Warn("scanner: aggregate failed", "session_id", sid, "error", err)
			}
		}
	}

	return nil
}

// Start 启动定期扫描循环
func (s *Scanner) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 启动时立即执行一次
	if err := s.ScanAndRepair(ctx); err != nil {
		slog.Warn("scanner initial scan error", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ScanAndRepair(ctx); err != nil {
				slog.Warn("scanner error", "error", err)
			}
		}
	}
}
