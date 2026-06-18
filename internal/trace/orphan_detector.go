package trace

// OrphanDetector 检测超时未配对的 pending span 并标记为 orphan
// 逻辑已内嵌在 Tracer.markOrphans 中

// OrphanDetector 为空实现，orphan 检测在 Tracer 中完成
type OrphanDetector struct{}
