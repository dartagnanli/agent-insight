package collector

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/dartagnanli/agent-insight/pkg/hookinput"
)

// StorageWriter 是 collector 所需的最小存储接口
type StorageWriter interface {
	InsertEvent(ctx context.Context, evt *event.HookEvent) error
}

// Collector 负责 hook 事件采集
type Collector struct {
	storage    StorageWriter
	enricher   *Enricher
	sanitizer  *Sanitizer
	eventType  string
	stdinLimit int
}

// NewCollector 创建采集器
func NewCollector(sw StorageWriter, eventType string, maxInputSize int) *Collector {
	if maxInputSize <= 0 {
		maxInputSize = 10240
	}
	return &Collector{
		storage:    sw,
		enricher:   NewEnricher(),
		sanitizer:  NewSanitizer(maxInputSize, maxInputSize),
		eventType:  eventType,
		stdinLimit: 1 * 1024 * 1024,
	}
}

// Collect 执行采集流程：stdin 读取 -> 解析 -> 补充元数据 -> 截断 -> 写入
// 任何步骤失败仅 slog.Warn，不抛出错误
func (c *Collector) Collect(ctx context.Context) (*event.HookEvent, error) {
	start := time.Now()

	// 1. 读取 stdin
	data, err := ReadAllStdin(os.Stdin, c.stdinLimit)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}

	// 2. 解析 JSON
	input, err := hookinput.ParseHookInputFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}

	// 3. 校验
	if err := hookinput.Validate(input); err != nil {
		return nil, fmt.Errorf("validate input: %w", err)
	}

	// 4. 补充元数据
	evt := c.enricher.Enrich(input, c.eventType)

	// 5. 截断
	c.sanitizer.Sanitize(evt)

	// 6. 记录采集耗时
	evt.CollectDurationMs = int(time.Since(start).Milliseconds())

	// 7. 写入 SQLite
	if err := c.storage.InsertEvent(ctx, evt); err != nil {
		return nil, fmt.Errorf("store event: %w", err)
	}

	return evt, nil
}

// Run 采集并退出，始终 exit 0
func (c *Collector) Run(ctx context.Context, eventType string) {
	evt, err := c.Collect(ctx)
	if err != nil {
		slog.Warn("collection failed", "error", err)
		os.Exit(0)
	}
	slog.Debug("event collected", "event_id", evt.EventID, "type", evt.EventType)
	os.Exit(0)
}

// RunAsync 执行异步处理（trace/stats/session），最多等待 100ms
func (c *Collector) RunAsync(ctx context.Context, evt *event.HookEvent, processors []EventProcessor) {
	wg := sync.WaitGroup{}
	for _, p := range processors {
		wg.Add(1)
		go func(proc EventProcessor) {
			defer wg.Done()
			if err := proc.Process(ctx, evt); err != nil {
				slog.Warn("async processor failed", "name", proc.Name(), "error", err)
			}
		}(p)
	}

	waitCh := make(chan struct{})
	go func() { wg.Wait(); close(waitCh) }()

	select {
	case <-waitCh:
	case <-time.After(100 * time.Millisecond):
		slog.Warn("async processing timed out")
	case <-ctx.Done():
	}
}

// EventProcessor 异步事件处理器接口
type EventProcessor interface {
	Process(ctx context.Context, evt *event.HookEvent) error
	Name() string
}

// FormatDuration 格式化持续时间为人类可读格式
func FormatDuration(d time.Duration) string {
	secs := int64(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	remSecs := secs % 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, remSecs)
	}
	hrs := mins / 60
	remMins := mins % 60
	return fmt.Sprintf("%dh %dm %ds", hrs, remMins, remSecs)
}
