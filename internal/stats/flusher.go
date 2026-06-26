package stats

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/pkg/event"
)

// Flusher 定期从 Engine 快照聚合小时级统计并写入数据库
type Flusher struct {
	storage storage.Storage
	engine  *Engine
}

// NewFlusher 创建统计刷新器
func NewFlusher(s storage.Storage) *Flusher {
	return &Flusher{
		storage: s,
		engine:  NewEngine(),
	}
}

// Ingest 添加事件到内存滑动窗口
func (f *Flusher) Ingest(ev *event.HookEvent) {
	f.engine.Ingest(ev)
}

// Flush 将当前窗口聚合写入 stats_hourly
func (f *Flusher) Flush(ctx context.Context) error {
	// 按小时/事件类型/工具名聚合
	type aggKey struct {
		bucket    string
		eventType string
		toolName  string
	}
	aggs := make(map[aggKey]*aggData)

	// 直接从数据库查询最近 24h 事件并聚合
	since := time.Now().UTC().Add(-24 * time.Hour)
	filter := storage.EventFilter{Since: &since, Limit: 10000}
	events, err := f.storage.QueryEvents(ctx, filter)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	for _, ev := range events {
		createdAt, _ := time.Parse("2006-01-02T15:04:05.000Z", ev.CreatedAt)
		if createdAt.IsZero() {
			createdAt, _ = time.Parse(time.RFC3339, ev.CreatedAt)
		}
		bucket := createdAt.Truncate(time.Hour).UTC().Format("2006-01-02T15:04:05Z")
		toolName := ""
		if ev.ToolName != "" {
			toolName = ev.ToolName
		}

		key := aggKey{bucket: bucket, eventType: ev.EventType, toolName: toolName}
		ad, ok := aggs[key]
		if !ok {
			ad = &aggData{}
			aggs[key] = ad
		}
		ad.count++
		if ev.Blocked {
			ad.blockCount++
		}
		if ev.CollectDurationMs > 0 {
			ad.durations = append(ad.durations, float64(ev.CollectDurationMs))
		}
	}

	// 构建 StatsHourlyRow 列表
	var rows []storage.StatsHourlyRow
	for key, ad := range aggs {
		row := storage.StatsHourlyRow{
			BucketHour: key.bucket,
			EventType:  key.eventType,
			EventCount: ad.count,
			BlockCount: ad.blockCount,
		}
		if key.toolName != "" {
			tn := key.toolName
			row.ToolName = &tn
		}
		if len(ad.durations) > 0 {
			sort.Float64s(ad.durations)
			avg := average(ad.durations)
			p50 := percentile(ad.durations, 50)
			p95 := percentile(ad.durations, 95)
			p99 := percentile(ad.durations, 99)
			row.AvgDurationMs = &avg
			row.P50DurationMs = &p50
			row.P95DurationMs = &p95
			row.P99DurationMs = &p99
		}
		rows = append(rows, row)
	}

	if len(rows) > 0 {
		if err := f.storage.UpsertStatsHourly(ctx, rows); err != nil {
			return err
		}
		slog.Debug("stats flushed", "rows", len(rows))
	}
	return nil
}

// Start 启动定期刷新循环
func (f *Flusher) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.Flush(ctx); err != nil {
				slog.Warn("stats flush error", "error", err)
			}
		}
	}
}

type aggData struct {
	count      int
	blockCount int
	durations  []float64
}
