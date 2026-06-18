package trace

// PendingSpans 管理 pending span 的 FIFO 队列
// 逻辑已内嵌在 Tracer.pending 字段中

// PendingSpans 为空实现，队列逻辑在 Tracer 中完成
type PendingSpans struct{}
