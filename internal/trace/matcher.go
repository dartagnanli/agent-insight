package trace

// Matcher 实现 Pre/PostToolUse 的 FIFO 配对逻辑
// 逻辑已内嵌在 Tracer.ProcessEvent 中

// Matcher 为空实现，配对逻辑在 Tracer 中完成
type Matcher struct{}
