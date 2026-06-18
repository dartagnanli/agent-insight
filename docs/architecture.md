# agent-insight 系统架构设计

> 版本: v1.0.0
> 日期: 2026-06-18
> 作者: 架构师
> 状态: Approved

---

## 1. 技术选型

### 1.1 核心依赖

| 领域 | 选型 | 版本 | 选型理由 |
|------|------|------|---------|
| 语言 | Go | >= 1.22 | PRD 硬性要求；泛型支持成熟，交叉编译简便 |
| SQLite 驱动 | modernc.org/sqlite | v1.34+ | 纯 Go 实现，禁 CGO，满足单二进制发布；通过 database/sql 标准接口接入 |
| CLI 框架 | spf13/cobra | v1.9+ | Go 生态事实标准；子命令/flags/补全/帮助文档一应俱全；社区活跃 |
| 配置管理 | spf13/viper | v1.20+ | 支持 YAML/ENV/flag 多源合并；与 cobra 深度集成；成熟稳定 |
| HTTP 服务 | net/http (stdlib) | - | 仪表板仅为本地服务，无需路由中间件生态；stdlib 零依赖、性能足够 |
| WebSocket | coder/websocket | v1.8+ | 零依赖、first-class context 支持、通过 autobahn 测试、并发写安全；比 gorilla/websocket 更现代 |
| 日志 | slog (stdlib) | Go 1.22+ | 结构化日志，stdlib 零依赖，性能优秀 |
| UUID | google/uuid | v1.6+ | 事件 ID 生成；RFC 4122 v4 实现，无需网络 |
| 前端 | 原生 HTML/CSS/JS + Chart.js | Chart.js v4 | 零构建链，go:embed 直接嵌入；Chart.js 通过内嵌 bundle 而非 CDN 加载，满足离线要求 |
| 前端模板 | html/template (stdlib) | - | 服务端渲染 HTML，避免引入模板引擎依赖 |
| 构建发布 | goreleaser/goreleaser | v2+ | 多平台交叉编译、单二进制打包、checksum 生成、Homebrew/GitHub Release 集成 |
| 交叉编译 | GOOS/GOARCH | - | 禁 CGO 后可自由交叉编译，无需 toolchain |

### 1.2 选型关键决策

**为什么不用 Gin/Echo 等 Web 框架？**
仪表板仅 4 个页面 + WebSocket 端点，路由数 < 15。引入框架增加依赖面但无实际收益。stdlib `http.ServeMux` (Go 1.22+ 支持 METHOD + pattern) 已足够。

**为什么选 coder/websocket 而非 gorilla/websocket？**
gorilla/websocket 已进入维护模式（2022 年归档后虽有人接手），coder/websocket API 更现代（first-class context、zero-alloc read/write），且零外部依赖。

**为什么不用 GORM 等 ORM？**
PRD 要求数据库层可替换（未来 Postgres/ClickHouse），ORM 会引入隐式方言耦合。显式 SQL + 接口抽象更利于后端切换。表结构简单（3 张表），手写 SQL 维护成本可接受。

**为什么前端不用 Preact/Svelte？**
PRD Q-01 决策：无 Node 构建链、go:embed 直接嵌入。原生 JS + Chart.js 内嵌 bundle 的方案：零构建步骤、无 npm 依赖、离线可用、单二进制发布。4 个页面的交互复杂度不需要组件化框架。

---

## 2. 架构设计

### 2.1 模块划分

```
cmd/agent-insight/main.go          -- 入口，组装依赖，注册 cobra 命令
  |
  +-- internal/collector/           -- 事件采集（F-01）
  |     +-- collector.go            -- Collector 接口 + 实现
  |     +-- stdin_reader.go         -- stdin JSON 读取与解析
  |     +-- enricher.go             -- 元数据补充（PID、hostname、时间戳）
  |     +-- sanitizer.go            -- 输入截断、敏感字段处理
  |
  +-- internal/storage/             -- 存储层（F-01, 可扩展）
  |     +-- storage.go              -- Storage 接口定义
  |     +-- sqlite.go               -- SQLite 实现
  |     +-- schema.go               -- 建表/迁移逻辑
  |     +-- retention.go            -- 数据保留策略执行
  |
  +-- internal/trace/               -- 调用链追踪（F-02）
  |     +-- tracer.go               -- Tracer 接口 + 实现
  |     +-- matcher.go              -- Pre/Post 配对匹配器
  |     +-- pending_spans.go        -- pending span 管理（FIFO 队列）
  |     +-- orphan_detector.go       -- orphan 检测与超时清理
  |
  +-- internal/stats/               -- 统计分析引擎（F-04）
  |     +-- engine.go               -- StatsEngine 接口 + 实现
  |     +-- aggregator.go           -- 滑动窗口聚合器
  |     +-- percentiles.go          -- 百分位数计算
  |     +-- flusher.go              -- 定时刷盘到 stats_hourly
  |
  +-- internal/session/             -- 会话聚合（F-06）
  |     +-- tracker.go              -- Session 生命周期追踪
  |     +-- aggregator.go           -- Session 指标聚合
  |     +-- scanner.go              -- 补全扫描器
  |
  +-- internal/alert/               -- 告警引擎（F-07）
  |     +-- engine.go               -- AlertEngine 接口 + 实现
  |     +-- rules.go                -- 内置规则实现（A-01 ~ A-05）
  |     +-- evaluator.go            -- 规则评估器
  |     +-- silencer.go             -- 静默期管理
  |     +-- channels/               -- 通知渠道
  |       +-- stderr.go
  |       +-- notify.go
  |       +-- webhook.go
  |       +-- file.go
  |
  +-- internal/dashboard/           -- Web 仪表板（F-05）
  |     +-- server.go               -- HTTP/WebSocket 服务
  |     +-- handler_api.go           -- REST API handlers
  |     +-- handler_ws.go            -- WebSocket handler + 事件广播
  |     +-- hub.go                  -- 连接管理 Hub
  |     +-- middleware.go           -- CORS、请求日志等中间件
  |
  +-- internal/config/              -- 配置管理
  |     +-- config.go               -- Config 结构体 + 加载逻辑
  |     +-- defaults.go             -- 默认值
  |
  +-- internal/export/              -- 数据导出（F-08）
  |     +-- exporter.go             -- Exporter 接口 + 实现
  |     +-- json.go                 -- JSON 导出
  |     +-- csv.go                  -- CSV 导出
  |     +-- html.go                 -- HTML 报告导出
  |
  +-- pkg/event/                    -- 公共事件模型（可被外部引用）
  |     +-- hook_event.go           -- HookEvent 结构体
  |     +-- hook_input.go           -- stdin JSON 输入结构体
  |     +-- span.go                 -- Span（配对后的工具调用）结构体
  |     +-- trace.go                -- Trace（完整调用链）结构体
  |
  +-- pkg/hookinput/                -- stdin 解析公共库
  |     +-- parser.go               -- ParseHookInput 从 io.Reader 解析
  |     +-- validator.go            -- 字段校验
  |
  +-- web/assets/                   -- 前端静态资源源码
  |     +-- index.html              -- SPA 入口
  |     +-- css/
  |     +-- js/
  |     +-- vendor/                 -- Chart.js 等 vendor bundle
  |
  +-- web/dist/                     -- go:embed 目标目录（构建产物）
```

### 2.2 数据流

**采集流（核心路径，P99 < 5ms）**

```
Claude Code 触发 hook
    |
    | stdin JSON
    v
cmd/agent-insight collect --event <type>
    |
    v
collector.StdinReader.ReadAll(os.Stdin)
    | 读取完整 stdin JSON (<0.5ms)
    v
hookinput.ParseHookInput(jsonBytes)
    | 解析为 HookInput 结构体 (<0.1ms)
    v
collector.Enricher.Enrich(hookInput)
    | 补充 event_id(UUID)、pid、hostname、created_at、collect_duration_ms (<0.2ms)
    v
collector.Sanitizer.Sanitize(enrichedEvent)
    | 截断 tool_input/tool_output 至 10KB (<0.1ms)
    v
storage.SQLite.InsertEvent(event)
    | WAL 模式写入 (<2ms)
    v
exit 0  (不阻塞 Claude Code)
    |
    |--- 异步路径（不影响 exit 时间）--->
    |
    v
trace.Matcher.Match(event)               -- Pre/Post 配对
    v
stats.Aggregator.Ingest(event)           -- 滑动窗口计数
    v
session.Tracker.Track(event)             -- Session 生命周期更新
    v
alert.Evaluator.Evaluate(event)          -- 告警规则检测
    v
dashboard.Hub.Broadcast(event)           -- WebSocket 推送（如仪表板运行中）
```

**查询流（CLI/Web）**

```
用户 CLI 命令 / 浏览器请求
    |
    v
cobra 命令 handler / HTTP handler
    |
    v
storage.SQLite.QueryXxx(...)
    | 读 SQLite（优先读聚合表）
    v
trace.Tracer.BuildTrace(sessionID)  -- 仅 trace 命令
    v
格式化输出（CLI: 文本表格 / Web: JSON）
```

**聚合流（后台定时）**

```
stats.Flusher (每 5min)
    | 读取内存滑动窗口聚合数据
    v
storage.SQLite.UpsertStatsHourly(...)

session.Scanner (每 10min)
    | 扫描已完成但未聚合的 session
    v
session.Aggregator.Aggregate(sessionID)
    v
storage.SQLite.UpsertSessionStats(...)

storage.Retention (启动时 + 每小时)
    | 删除超期数据
    v
DELETE FROM hook_events WHERE created_at < ?
DELETE FROM stats_hourly WHERE bucket_hour < ?
```

### 2.3 组件交互关系

**进程模型**

agent-insight 有两种运行模式，共享同一个二进制：

1. **collect 模式**：每次 Claude Code 触发 hook 时启动一个短生命周期进程。进程从 stdin 读取 JSON、写入 SQLite、exit 0。采集路径同步执行（保证 exit 前写入完成），其他路径在 `sync.WaitGroup` 守护的 goroutine 中异步执行（进程退出前最多等待 100ms）。

2. **dashboard 模式**：长生命周期进程，启动 HTTP/WebSocket 服务。通过定时轮询 SQLite（1s 间隔）检测新事件，而非依赖 collect 进程的直接通知（因为 collect 进程独立且短暂）。后台运行 stats flusher、session scanner、retention cleaner。

**关键交互**

- `collector` -> `storage`：同步写入，collect 进程的主路径。
- `collector` -> `trace` / `stats` / `session` / `alert`：异步 goroutine，collect 进程退出前最多等待 100ms，超时则丢弃（不影响主路径）。
- `dashboard` -> `storage`：读取查询，优先读聚合表（stats_hourly、session_stats）。
- `dashboard` -> `trace`：调用 Tracer 构建 trace 数据（瀑布图页面）。
- `stats` -> `storage`：定时 flush 聚合数据。
- `session` -> `storage`：定时聚合 session 数据。
- `alert` -> `channels`：触发通知。
- `CLI 命令` -> `storage` / `trace`：直接查询。

**并发安全**

- collect 进程之间通过 SQLite WAL 模式 + busy_timeout 实现并发写入安全。
- dashboard 进程内，多个 HTTP handler 通过 Go channel 与 WebSocket Hub 通信，Hub 串行化 WebSocket 写操作。
- 内存中的滑动窗口聚合器和 pending span 队列使用 `sync.Mutex` 保护。

---

## 3. 接口契约

### 3.1 核心接口定义

**Storage 接口（internal/storage/storage.go）**

```go
type Storage interface {
    // 初始化：建表/迁移
    Init(ctx context.Context) error

    // 事件写入
    InsertEvent(ctx context.Context, event *event.HookEvent) error

    // 事件查询
    QueryEvents(ctx context.Context, filter EventFilter) ([]*event.HookEvent, error)
    GetEvent(ctx context.Context, eventID string) (*event.HookEvent, error)

    // 聚合表写入
    UpsertStatsHourly(ctx context.Context, rows []StatsHourlyRow) error
    UpsertSessionStats(ctx context.Context, row *SessionStatsRow) error

    // 聚合表查询
    QueryStatsHourly(ctx context.Context, filter StatsFilter) ([]StatsHourlyRow, error)
    QuerySessionStats(ctx context.Context, filter SessionFilter) ([]SessionStatsRow, error)

    // 会话列表
    ListSessions(ctx context.Context, filter SessionFilter) ([]SessionSummary, error)

    // 数据保留
    DeleteBefore(ctx context.Context, before time.Time) (int64, error)

    // 关闭
    Close() error
}
```

**EventFilter（internal/storage/filter.go）**

```go
type EventFilter struct {
    SessionID  *string     // 精确匹配
    EventType  *string     // 精确匹配
    ToolName   *string     // 精确匹配
    Blocked    *bool       // 精确匹配
    Since      *time.Time  // created_at >=
    Until      *time.Time  // created_at <=
    Limit      int         // 默认 100，最大 10000
    Offset     int         // 分页偏移
    SortBy     string      // "created_at"(默认) | "hook_duration_ms"
    SortOrder  string      // "asc" | "desc"(默认)
}
```

**StatsFilter（internal/storage/filter.go）**

```go
type StatsFilter struct {
    Since      *time.Time  // bucket_hour >=
    Until      *time.Time  // bucket_hour <=
    EventType  *string     // 精确匹配，空=全部
    ToolName   *string     // 精确匹配，空=全部
}
```

**SessionFilter（internal/storage/filter.go）**

```go
type SessionFilter struct {
    ProjectPath *string    // 精确匹配
    Since       *time.Time // started_at >=
    Until       *time.Time // started_at <=
    SortBy      string     // "started_at"(默认) | "total_events" | "duration_secs"
    SortOrder   string     // "asc" | "desc"(默认)
    Limit       int        // 默认 100，最大 1000
    Offset      int        // 分页偏移
}
```

**SessionSummary（internal/storage/filter.go）**

```go
type SessionSummary struct {
    SessionID   string
    StartedAt   time.Time
    TotalEvents int
    DurationSec int64
    Blocked     int
}
```

**Tracer 接口（internal/trace/tracer.go）**

```go
type Tracer interface {
    // 处理新事件，尝试配对
    ProcessEvent(ctx context.Context, event *event.HookEvent) error

    // 构建完整调用链
    BuildTrace(ctx context.Context, sessionID string) (*event.Trace, error)

    // 获取 pending span 状态（调试用）
    PendingSpans(ctx context.Context, sessionID string) ([]*event.Span, error
}
```

**StatsEngine 接口（internal/stats/engine.go）**

```go
type StatsEngine interface {
    // 摄入新事件到滑动窗口
    Ingest(event *event.HookEvent)

    // 获取当前窗口统计快照
    Snapshot(window time.Duration) (*StatsSnapshot, error)

    // 刷盘到持久化聚合表
    Flush(ctx context.Context) error

    // 启动定时刷盘
    Start(ctx context.Context, interval time.Duration)

    // 停止
    Stop()
}
```

**AlertEngine 接口（internal/alert/engine.go）**

```go
type AlertEngine interface {
    // 评估单事件
    Evaluate(ctx context.Context, event *event.HookEvent) ([]*Alert, error)

    // 评估窗口聚合（用于 A-03 类规则）
    EvaluateWindow(ctx context.Context, snapshot *stats.StatsSnapshot) ([]*Alert, error)

    // 发送告警
    Send(ctx context.Context, alert *Alert) error
}
```

**Exporter 接口（internal/export/exporter.go）**

```go
type Exporter interface {
    Export(ctx context.Context, events []*event.HookEvent, w io.Writer) error
    Format() string // "json" | "csv" | "html"
}
```

### 3.2 CLI 命令接口

**collect 子命令**

```
agent-insight collect --event <event_type>

Flags:
  --event string    (required) hook 事件类型：SessionStart|UserPromptSubmit|PreToolUse|PostToolUse|Notification|Stop|SubagentStop|PreCompact

stdin: Claude Code 传入的 hook JSON payload
stdout: 空（不输出，透传 stdin）
stderr: 告警输出（如有）
exit code: 0（始终 exit 0，即使写入失败）
```

**init 子命令**

```
agent-insight init [--project] [--global]

Flags:
  --project    生成项目级 .claude/settings.json hook 配置（默认）
  --global     生成用户级 ~/.claude/settings.json hook 配置
  --force      覆盖已存在的配置

输出：写入 settings.json 或打印配置到 stdout（--dry-run）
```

**trace 子命令**

```
agent-insight trace <session_id> [flags]

Flags:
  --format string   输出格式："text"(默认) | "json"
  --no-color         禁用彩色输出

输出：
  text 格式：waterfall 风格文本
  json 格式：完整 Trace 对象 JSON
```

**sessions 子命令**

```
agent-insight sessions [flags]

Flags:
  --sort string      排序字段："started_at"(默认) | "events" | "duration" | "blocked"
  --order string     "desc"(默认) | "asc"
  --since duration   时间过滤："1h"/"24h"/"7d"/"30d"
  --limit int        最大条数，默认 50
  --detail string    显示特定 session 的详细聚合信息

输出：表格格式的 session 列表
```

**stats 子命令**

```
agent-insight stats [flags]

Flags:
  --since duration   时间范围："1h"/"6h"/"24h"(默认)/"7d"/"30d"
  --tool string      按工具过滤
  --event string     按事件类型过滤
  --format string    输出格式："text"(默认) | "json"

输出：
  text 格式：格式化统计摘要
  json 格式：StatsSnapshot 对象 JSON
```

**dashboard 子命令**

```
agent-insight dashboard [flags]

Flags:
  --host string      监听地址，默认 "127.0.0.1"
  --port int         监听端口，默认 8080
  --global           聚合所有项目数据（多项目模式）
  --open             自动打开浏览器

输出：启动日志到 stderr
```

**export 子命令**

```
agent-insight export [flags]

Flags:
  --format string    导出格式："json"(默认) | "csv" | "html"
  --since duration   时间范围
  --session string   按 session 过滤
  --output string    输出文件路径（默认 stdout）

输出：指定格式的导出数据
```

**bench 子命令**

```
agent-insight bench [flags]

Flags:
  --events int       事件数量，默认 10000
  --concurrency int  并发数，默认 4
  --db-path string   临时数据库路径（默认系统临时目录）

输出：基准测试结果文本
```

**config 子命令**

```
agent-insight config [key] [value]

子命令：
  get <key>          获取配置值
  set <key> <value>  设置配置值
  list               列出所有配置
  path               显示配置文件路径

配置键使用点分路径：storage.path, dashboard.port, alerts.enabled 等
```

**version 子命令**

```
agent-insight version

输出：agent-insight v0.1.0 (go1.22.0, darwin/arm64, commit: abc1234)
```

### 3.3 REST API 契约

仪表板 HTTP 服务的基础路径：`/api/v1`

**事件相关**

```
GET /api/v1/events
  Query 参数：
    session_id  string   精确匹配
    event_type  string   精确匹配
    tool_name   string   精确匹配
    blocked     bool     精确匹配
    since       string   RFC3339 时间戳
    until       string   RFC3339 时间戳
    limit       int      默认 100，最大 10000
    offset      int      分页偏移
    sort_by     string   "created_at" | "hook_duration_ms"
    sort_order  string   "asc" | "desc"

  Response 200:
    {
      "events": [
        {
          "id":                 1,
          "event_id":           "550e8400-e29b-41d4-a716-446655440000",
          "session_id":         "abc123",
          "event_type":         "PreToolUse",
          "tool_name":          "Bash",
          "tool_input":         "{\"command\":\"npm test\"}",
          "tool_output":        null,
          "cwd":                "/home/user/project",
          "transcript_path":    "/home/.claude/transcripts/abc.jsonl",
          "blocked":            false,
          "block_reason":       null,
          "hook_exit_code":     0,
          "hook_duration_ms":   2,
          "collect_duration_ms":1,
          "pid":                12345,
          "hostname":           "macbook-local",
          "created_at":         "2026-06-18T10:23:45Z"
        }
      ],
      "total":  1247,
      "limit":  100,
      "offset": 0
    }

GET /api/v1/events/{event_id}
  Response 200: 单个事件对象（同上结构）
  Response 404: {"error": "event not found"}
```

**统计相关**

```
GET /api/v1/stats
  Query 参数：
    since       string   RFC3339 时间戳
    until       string   RFC3339 时间戳
    event_type  string   精确匹配
    tool_name   string   精确匹配

  Response 200:
    {
      "total_events":    1247,
      "total_sessions":  12,
      "total_blocked":   104,
      "block_rate":      0.083,
      "avg_hook_duration_ms": 1.2,
      "p50_hook_duration_ms": 0.8,
      "p95_hook_duration_ms": 3.1,
      "p99_hook_duration_ms": 4.8,
      "tool_distribution": [
        {"tool_name": "Bash",  "count": 489, "blocked": 32, "avg_duration_ms": 3200, "p99_duration_ms": 12100},
        {"tool_name": "Write", "count": 312, "blocked": 45, "avg_duration_ms": 800,  "p99_duration_ms": 2100}
      ],
      "event_type_distribution": [
        {"event_type": "PreToolUse",  "count": 623},
        {"event_type": "PostToolUse", "count": 614},
        {"event_type": "SessionStart","count": 10}
      ],
      "hourly_trend": [
        {"hour": "2026-06-18T09:00:00Z", "count": 340},
        {"hour": "2026-06-18T10:00:00Z", "count": 512}
      ]
    }

GET /api/v1/stats/hourly
  Query 参数：同 /api/v1/stats
  Response 200:
    {
      "buckets": [
        {
          "bucket_hour":     "2026-06-18T10:00:00Z",
          "event_type":      "PreToolUse",
          "tool_name":       "Bash",
          "event_count":     120,
          "block_count":     8,
          "avg_duration_ms": 2.1,
          "p50_duration_ms": 1.5,
          "p95_duration_ms": 3.8,
          "p99_duration_ms": 4.6
        }
      ]
    }
```

**调用链相关**

```
GET /api/v1/traces/{session_id}
  Response 200:
    {
      "session_id":    "abc123",
      "started_at":    "2026-06-18T10:23:00Z",
      "ended_at":       "2026-06-18T10:25:34Z",
      "duration_secs":  154,
      "total_events":   42,
      "spans": [
        {
          "span_id":           "550e8400-...",
          "tool_name":         "Bash",
          "tool_input":        "{\"command\":\"npm test\"}",
          "tool_output":       "{\"exit_code\":0,\"stdout\":\"...\"}",
          "started_at":        "2026-06-18T10:23:02Z",
          "ended_at":          "2026-06-18T10:23:05Z",
          "duration_ms":       3000,
          "blocked":           false,
          "block_reason":      null,
          "orphan":            false,
          "pre_event_id":      "550e8400-...",
          "post_event_id":     "660e8400-..."
        },
        {
          "span_id":           "770e8400-...",
          "tool_name":         "Write",
          "tool_input":        "{\"file_path\":\"src/auth.ts\"}",
          "tool_output":       null,
          "started_at":        "2026-06-18T10:23:06Z",
          "ended_at":          null,
          "duration_ms":       0,
          "blocked":           true,
          "block_reason":      "Style check failed: missing semicolon",
          "orphan":            true,
          "pre_event_id":      "770e8400-...",
          "post_event_id":     null
        }
      ],
      "standalone_events": [
        {
          "event_id":    "880e8400-...",
          "event_type":  "SessionStart",
          "created_at":  "2026-06-18T10:23:00Z"
        },
        {
          "event_id":    "990e8400-...",
          "event_type":  "Stop",
          "created_at":  "2026-06-18T10:25:34Z"
        }
      ]
    }
  Response 404: {"error": "session not found"}
```

**Session 相关**

```
GET /api/v1/sessions
  Query 参数：
    since       string   RFC3339
    until       string   RFC3339
    sort_by     string   "started_at" | "total_events" | "duration_secs"
    sort_order  string   "asc" | "desc"
    limit       int      默认 100，最大 1000
    offset      int

  Response 200:
    {
      "sessions": [
        {
          "session_id":    "abc123",
          "started_at":    "2026-06-18T10:23:00Z",
          "ended_at":      "2026-06-18T10:25:34Z",
          "duration_secs": 154,
          "total_events":  42,
          "tool_calls":    35,
          "blocked_calls": 3,
          "block_rate":    0.086,
          "tools_used":    ["Bash","Write","Edit"],
          "avg_tool_duration_ms": 1800,
          "p99_tool_duration_ms": 5100,
          "project_path":  "/home/user/project"
        }
      ],
      "total":  12,
      "limit":  100,
      "offset": 0
    }
```

**WebSocket 端点**

```
WS /api/v1/ws/events

  服务端 -> 客户端消息（JSON）：
    {
      "type":     "event",          // 新事件推送
      "payload":  { ... HookEvent ... }
    }

    {
      "type":     "alert",          // 告警推送
      "payload":  {
        "id":        "alert-550e8400-...",
        "rule_id":   "A-01",
        "level":     "WARN",
        "message":   "Hook execution exceeded 1000ms",
        "event_id":  "550e8400-...",
        "tool_name": "Bash",
        "duration_ms": 1200,
        "timestamp": "2026-06-18T10:23:45Z"
      }
    }

    {
      "type":     "stats_update",   // 聚合数据更新
      "payload":  { ... StatsSnapshot ... }
    }

  客户端 -> 服务端消息（JSON）：
    {
      "type":     "subscribe",      // 订阅过滤
      "filters":  {
        "session_id": "abc123",     // 可选
        "event_type": "PreToolUse",// 可选
        "tool_name":  "Bash"       // 可选
      }
    }

    {
      "type":     "ping"
    }

  心跳：服务端每 30s 发 ping，客户端 10s 内需回 pong，否则断开
```

**静态资源**

```
GET /                    -- 事件流页面（默认首页）
GET /dashboard           -- 统计概览页面
GET /trace               -- 调用瀑布图页面
GET /sessions            -- Session 列表页面
GET /assets/{path}       -- 静态资源（JS/CSS/vendor）
```

---

## 4. 数据模型

### 4.1 核心表（hook_events）

```sql
CREATE TABLE IF NOT EXISTS hook_events (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id          TEXT    NOT NULL,                          -- UUID v4
    session_id        TEXT    NOT NULL,                          -- Claude Code session ID
    event_type        TEXT    NOT NULL,                          -- SessionStart|UserPromptSubmit|PreToolUse|PostToolUse|Notification|Stop|SubagentStop|PreCompact
    tool_name         TEXT    DEFAULT NULL,                     -- 仅 Pre/PostToolUse 有值
    tool_input        TEXT    DEFAULT NULL,                     -- JSON string, 截断至 10KB
    tool_output       TEXT    DEFAULT NULL,                     -- JSON string, 仅 PostToolUse, 截断至 10KB
    cwd               TEXT    NOT NULL,                         -- 工作目录
    transcript_path   TEXT    DEFAULT NULL,                     -- transcript 文件路径
    blocked           INTEGER NOT NULL DEFAULT 0,              -- 0=否, 1=是 (SQLite 无原生 BOOLEAN)
    block_reason      TEXT    DEFAULT NULL,                    -- stderr 内容 (exit=2 时)
    hook_exit_code    INTEGER NOT NULL DEFAULT 0,              -- hook 退出码
    hook_duration_ms  INTEGER NOT NULL DEFAULT 0,             -- hook 自身执行耗时(ms)
    collect_duration_ms INTEGER NOT NULL DEFAULT 0,           -- 采集耗时(ms)
    pid               INTEGER NOT NULL DEFAULT 0,             -- Claude Code 进程 PID
    hostname          TEXT    NOT NULL DEFAULT '',             -- 主机名
    created_at        TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')) -- ISO 8601 UTC, 毫秒精度
);

-- 索引（查询优化）
CREATE INDEX IF NOT EXISTS idx_events_session    ON hook_events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_type       ON hook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_tool       ON hook_events(tool_name);
CREATE INDEX IF NOT EXISTS idx_events_time       ON hook_events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_blocked    ON hook_events(blocked);
CREATE INDEX IF NOT EXISTS idx_events_session_time ON hook_events(session_id, created_at);
```

**设计决策说明**：
- `blocked` 用 INTEGER 而非 BOOLEAN：SQLite 无原生布尔类型，INTEGER 0/1 是惯用做法。
- `created_at` 用 TEXT 而非 TIMESTAMP：SQLite 无时间类型原生支持，TEXT 存储 ISO 8601 字符串，便于跨语言解析，通过 `strftime` 实现毫秒精度。
- `idx_events_session_time`：复合索引，加速 trace 查询中最常见的 `WHERE session_id = ? ORDER BY created_at` 模式。
- `idx_events_blocked`：加速拦截率统计查询 `WHERE blocked = 1`。
- 不设外键：单表无关联关系，避免外键开销。

### 4.2 统计聚合表（stats_hourly）

```sql
CREATE TABLE IF NOT EXISTS stats_hourly (
    bucket_hour       TEXT    NOT NULL,                          -- 整点时间, 如 "2026-06-18T10:00:00Z"
    event_type        TEXT    NOT NULL,                          -- PreToolUse|PostToolUse|...
    tool_name         TEXT    DEFAULT NULL,                     -- 空=该 event_type 的汇总行
    event_count       INTEGER NOT NULL DEFAULT 0,
    block_count       INTEGER NOT NULL DEFAULT 0,
    avg_duration_ms   REAL    DEFAULT NULL,                     -- NULL 当无数据时
    p50_duration_ms   REAL    DEFAULT NULL,
    p95_duration_ms   REAL    DEFAULT NULL,
    p99_duration_ms   REAL    DEFAULT NULL,
    PRIMARY KEY (bucket_hour, event_type, tool_name)
);

CREATE INDEX IF NOT EXISTS idx_stats_hourly_time ON stats_hourly(bucket_hour);
```

**设计决策说明**：
- PRIMARY KEY 使用 (bucket_hour, event_type, tool_name)：天然唯一，支持 UPSERT。
- `tool_name` 允许 NULL：当为 NULL 时表示该 event_type 的汇总行（不含工具维度的聚合）。
- 百分位数用 REAL：p50/p95/p99 需要小数精度。

### 4.3 会话聚合表（session_stats）

```sql
CREATE TABLE IF NOT EXISTS session_stats (
    session_id          TEXT    PRIMARY KEY,
    started_at          TEXT    NOT NULL,                       -- SessionStart 的 created_at
    ended_at            TEXT    DEFAULT NULL,                   -- Stop 的 created_at 或推算值
    duration_secs       INTEGER DEFAULT 0,
    total_events        INTEGER NOT NULL DEFAULT 0,
    tool_calls          INTEGER NOT NULL DEFAULT 0,            -- PreToolUse 事件数
    blocked_calls       INTEGER NOT NULL DEFAULT 0,            -- blocked=1 的 PreToolUse 数
    block_rate          REAL    NOT NULL DEFAULT 0.0,           -- blocked_calls / tool_calls
    tools_used          TEXT    DEFAULT '[]',                   -- JSON array: ["Bash","Write","Edit"]
    avg_tool_duration_ms REAL   DEFAULT NULL,
    p99_tool_duration_ms REAL   DEFAULT NULL,
    project_path        TEXT    DEFAULT NULL                   -- cwd 字段值
);
```

**设计决策说明**：
- `tools_used` 用 JSON TEXT 而非关联表：工具列表通常 < 20 个，JSON 数组足够，避免额外表和 JOIN。
- `block_rate` 预计算：避免每次查询时做除法，且可处理 tool_calls=0 的边界情况。
- `ended_at` 允许 NULL：session 仍在进行中时为 NULL。

### 4.4 Go 结构体映射（pkg/event/）

```go
// HookEvent 对应 hook_events 表的一行记录
type HookEvent struct {
    ID               int64  `json:"id"               db:"id"`
    EventID          string `json:"event_id"         db:"event_id"`
    SessionID        string `json:"session_id"       db:"session_id"`
    EventType        string `json:"event_type"       db:"event_type"`
    ToolName         string `json:"tool_name"        db:"tool_name"`
    ToolInput        string `json:"tool_input"       db:"tool_input"`
    ToolOutput       string `json:"tool_output"      db:"tool_output"`
    Cwd              string `json:"cwd"              db:"cwd"`
    TranscriptPath   string `json:"transcript_path"  db:"transcript_path"`
    Blocked          bool   `json:"blocked"          db:"blocked"`
    BlockReason      string `json:"block_reason"     db:"block_reason"`
    HookExitCode     int    `json:"hook_exit_code"   db:"hook_exit_code"`
    HookDurationMs   int    `json:"hook_duration_ms" db:"hook_duration_ms"`
    CollectDurationMs int   `json:"collect_duration_ms" db:"collect_duration_ms"`
    Pid              int    `json:"pid"              db:"pid"`
    Hostname         string `json:"hostname"         db:"hostname"`
    CreatedAt        string `json:"created_at"       db:"created_at"` // ISO 8601 UTC
}

// HookInput 对应 Claude Code 通过 stdin 传入的 JSON
type HookInput struct {
    SessionID      string `json:"session_id"`
    Cwd            string `json:"cwd"`
    HookEventName  string `json:"hook_event_name"`
    TranscriptPath string `json:"transcript_path,omitempty"`
    ToolName       string `json:"tool_name,omitempty"`
    ToolInput      any    `json:"tool_input,omitempty"`  // object, 保留原始结构
    ToolResponse   any    `json:"tool_response,omitempty"` // object, 仅 PostToolUse
}

// Span 表示配对后的完整工具调用
type Span struct {
    SpanID       string  `json:"span_id"`
    ToolName     string  `json:"tool_name"`
    ToolInput    string  `json:"tool_input"`
    ToolOutput   string  `json:"tool_output,omitempty"`
    StartedAt    string  `json:"started_at"`
    EndedAt      string  `json:"ended_at,omitempty"`
    DurationMs   int64   `json:"duration_ms"`
    Blocked      bool    `json:"blocked"`
    BlockReason  string  `json:"block_reason,omitempty"`
    Orphan       bool    `json:"orphan"`
    PreEventID   string  `json:"pre_event_id"`
    PostEventID  string  `json:"post_event_id,omitempty"`
}

// Trace 表示一次会话的完整调用链
type Trace struct {
    SessionID        string           `json:"session_id"`
    StartedAt        string           `json:"started_at"`
    EndedAt          string           `json:"ended_at,omitempty"`
    DurationSecs     int64            `json:"duration_secs"`
    TotalEvents      int              `json:"total_events"`
    Spans            []*Span          `json:"spans"`
    StandaloneEvents []*StandaloneEvent `json:"standalone_events"`
}

// StandaloneEvent 非工具调用的事件（SessionStart/Stop 等）
type StandaloneEvent struct {
    EventID   string `json:"event_id"`
    EventType string `json:"event_type"`
    CreatedAt string `json:"created_at"`
}
```

### 4.5 数据库初始化与迁移

```go
// internal/storage/schema.go
// 所有建表语句放在一个 schema 切片中，按版本号递增
// 启动时通过 schema_version 表判断当前版本，逐版本执行迁移
var schemaMigrations = []string{
    // v1: 初始建表
    `
    CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY);
    INSERT INTO schema_version VALUES (1);
    -- ... 上述 CREATE TABLE/INDEX 语句 ...
    `,
    // v2+: 后续迁移
}

func (s *SQLite) migrate(ctx context.Context) error {
    // 读取当前 version，执行 version+1 到 len(migrations) 的所有迁移
    // 每个迁移在事务内执行
}
```

---

## 5. 非功能方案

### 5.1 性能设计

**采集路径（P99 < 5ms）**

| 步骤 | 预算 | 优化手段 |
|------|------|---------|
| stdin 读取 | < 0.5ms | `io.ReadAll` 一次性读取，大小限制 1MB |
| JSON 解析 | < 0.1ms | `encoding/json` 标准库，避免反射开销大的库 |
| UUID 生成 | < 0.05ms | `google/uuid` v4，无系统调用 |
| 元数据补充 | < 0.2ms | 缓存 hostname（进程内变量），PID 通过 `os.Getpid()` |
| 输入截断 | < 0.1ms | 字节级截断，字符串切片 |
| SQLite 写入 | < 2ms | WAL 模式 + `PRAGMA synchronous=NORMAL` + `PRAGMA busy_timeout=5000` + 预编译语句 |
| 总计 | < 3ms | 留 2ms 余量 |

**写入吞吐量（>= 1000 events/s）**

- WAL 模式允许并发读写，写不阻塞读。
- busy_timeout=5000ms 遇到锁时等待而非立即失败。
- `PRAGMA journal_size_limit=67108864` (64MB) 防止 WAL 文件无限增长。
- `PRAGMA cache_size=-32000` (32MB) 增加页缓存。
- collect 模式下 batch_size 默认为 1（单条插入），避免事务持有时间过长。但 INSERT 使用预编译语句 + 参数绑定，避免每次解析 SQL。

**内存占用（< 50MB）**

- collect 模式：短生命周期进程，内存主要用于 JSON 解析和 SQLite 写入缓冲，预估 < 10MB。
- dashboard 模式：滑动窗口聚合器内存占用取决于窗口大小。1h 窗口、1000 events/s，存储 event_type+tool_name 计数器，预估 < 5MB。pending span 队列：30s 超时 × 1000/s = 30000 条，每条 ~200B = 6MB。HTTP/WebSocket 连接缓冲：每连接 ~4KB，100 连接 = 0.4MB。总计 < 20MB。

**Web 仪表板首屏（< 500ms）**

- 服务端渲染 HTML 模板，API 返回 JSON，前端 Chart.js 渲染图表。
- 统计数据优先读 stats_hourly 聚合表（行数少），避免全表扫描。
- 首屏 API 并行请求（events、stats、sessions），前端 `Promise.all` 并行获取。

### 5.2 安全设计

| 安全要求 | 实现方案 |
|---------|---------|
| 数据本地化 | SQLite 文件仅写入本地磁盘，无任何远程写入逻辑 |
| 无网络外传 | 默认不发起任何外部 HTTP 请求；webhook 为显式配置项，需用户手动启用并白名单 URL |
| tool_input 截断 | `Sanitizer.Sanitize()` 在写入前检查 `len(tool_input) > 10240`，超出则截断并追加 `...[truncated]` 后缀 |
| tool_output 截断 | 同 tool_input，阈值相同 |
| SQLite 文件权限 | 创建数据库文件后立即 `os.Chmod(path, 0600)`；创建目录 `os.MkdirAll` 后 `os.Chmod(dir, 0700)` |
| HTTP 仅本地监听 | 默认绑定 `127.0.0.1`，不支持 `0.0.0.0` 除非显式 `--host 0.0.0.0` |
| WebSocket 来源校验 | 检查 `Origin` header 为 `http://localhost:PORT` 或 `http://127.0.0.1:PORT` |
| 输入大小限制 | stdin 读取限制 1MB，超限丢弃并 exit 0 |

### 5.3 可靠性设计

**采集失败容错**

```go
func (c *Collector) Run(ctx context.Context) {
    // 主路径：同步写入 SQLite
    event, err := c.collect(ctx)   // stdin 读取 + 解析 + 补充元数据
    if err != nil {
        // 记录日志但 exit 0
        slog.Warn("collection failed", "error", err)
        os.Exit(0)
    }
    if err := c.storage.InsertEvent(ctx, event); err != nil {
        slog.Warn("storage write failed", "error", err)
        // exit 0，不阻断 Claude Code
        os.Exit(0)
    }

    // 异步路径：trace/stats/session/alert/dashboard
    wg := sync.WaitGroup{}
    wg.Add(1)
    go func() {
        defer wg.Done()
        c.tracer.ProcessEvent(ctx, event)
        c.statsEngine.Ingest(event)
        c.sessionTracker.Track(ctx, event)
        c.alertEngine.Evaluate(ctx, event)
        c.dashboardHub.Broadcast(event)
    }()

    // 等待异步完成，最多 100ms
    waitCh := make(chan struct{})
    go func() { wg.Wait(); close(waitCh) }()
    select {
    case <-waitCh:
    case <-time.After(100 * time.Millisecond):
        slog.Warn("async processing timed out")
    }
    os.Exit(0)
}
```

**数据库损坏恢复**

```go
func (s *SQLite) Init(ctx context.Context) error {
    // 尝试 PRAGMA integrity_check
    var ok string
    err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok)
    if err != nil || ok != "ok" {
        // 备份损坏文件
        backupPath := s.path + ".corrupt." + time.Now().Format("20060102-150405")
        os.Rename(s.path, backupPath)
        slog.Warn("corrupt database backed up", "backup", backupPath)
        // 重建数据库
        return s.migrate(ctx)
    }
    return nil
}
```

**并发安全**

- SQLite WAL 模式：多个 collect 进程同时写入不阻塞。
- busy_timeout=5000ms：遇到写锁时等待最多 5 秒。
- 每个 collect 进程持有独立 `*sql.DB` 连接池（短生命周期，连接数 1）。
- dashboard 进程的 `*sql.DB` 连接池 `SetMaxOpenConns(4)`，`SetMaxIdleConns(2)`。
- 内存数据结构（滑动窗口、pending spans）通过 `sync.Mutex` 保护。

**优雅关闭**

```go
// dashboard 模式
func (srv *Server) Shutdown() {
    // 1. 停止接受新连接
    srv.httpServer.Shutdown(ctx)

    // 2. 关闭 WebSocket 连接
    srv.hub.CloseAll()

    // 3. 停止后台任务
    srv.statsEngine.Stop()
    srv.sessionScanner.Stop()
    srv.retention.Stop()

    // 4. 关闭数据库
    srv.storage.Close()
}

// collect 模式：sync.WaitGroup + 100ms timeout（见上方）
```

### 5.4 可扩展性设计

**存储后端抽象**

```go
// internal/storage/storage.go
type Storage interface {
    InsertEvent(ctx context.Context, event *event.HookEvent) error
    QueryEvents(ctx context.Context, filter EventFilter) ([]*event.HookEvent, error)
    // ... 其他方法
}

// 当前实现
type SQLite struct { db *sql.DB }

// 未来可扩展
// type Postgres struct { db *sql.DB }
// type ClickHouse struct { conn clickhouse.Conn }
```

所有存储操作通过 `Storage` 接口访问，具体实现由 `config.Storage.Type` 决定。工厂函数：

```go
func NewStorage(ctx context.Context, cfg config.StorageConfig) (Storage, error) {
    switch cfg.Type {
    case "sqlite":
        return NewSQLite(ctx, cfg)
    default:
        return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
    }
}
```

**采集器插件化**

```go
// internal/collector/collector.go
type EventProcessor interface {
    Process(ctx context.Context, event *event.HookEvent) error
    Name() string
}

type Collector struct {
    processors []EventProcessor
}

// 内置处理器通过 init 注册
func init() {
    RegisterProcessor(&TraceProcessor{})
    RegisterProcessor(&StatsProcessor{})
    RegisterProcessor(&SessionProcessor{})
    RegisterProcessor(&AlertProcessor{})
    RegisterProcessor(&DashboardProcessor{})
}
```

**统计引擎可扩展**

```go
// internal/stats/engine.go
type Aggregator interface {
    Name() string
    Ingest(event *event.HookEvent)
    Snapshot(window time.Duration) any
    Reset()
}

// 内置聚合器
type CountAggregator struct { ... }    // 事件计数
type DurationAggregator struct { ... } // 耗时百分位
type BlockRateAggregator struct { ... } // 拦截率

// 未来可扩展
// type CustomMetricAggregator struct { ... }
```

---

## 6. 技术风险

| 编号 | 风险 | 影响 | 概率 | 缓解措施 | 负责人 |
|------|------|------|------|---------|--------|
| R-01 | Claude Code stdin JSON 格式变更导致解析失败 | 采集器无法工作，事件丢失 | 中 | 1. `hookinput.ParseHookInput` 做宽松解析，未知字段忽略而非报错；2. 必需字段缺失时记录 warn 日志但 exit 0；3. 版本检测：init 命令检查 Claude Code 版本兼容性 | 开发 |
| R-02 | Pre/Post 配对算法在并发场景下误匹配 | 调用链数据不准确 | 中 | 1. 同一 session 内严格 FIFO 配对；2. 对连续同名 tool 调用使用栈式匹配（后进先出的考虑，最终选择 FIFO 因更符合顺序执行语义）；3. 配对超时 30s 后自动标记 orphan；4. 提供 `trace --debug` 查看未配对事件辅助排查 | 开发 |
| R-03 | SQLite WAL 文件在频繁写入时无限膨胀 | 磁盘空间耗尽 | 低 | 1. `PRAGMA journal_size_limit=67108864`（64MB）；2. 每次 checkpoint 后 WAL 自动回收；3. retention cleaner 定期执行 `PRAGMA wal_checkpoint(TRUNCATE)` | 开发 |
| R-04 | modernc.org/sqlite 性能不如 CGO 版本 | 达不到 1000 events/s 吞吐 | 低 | 1. 基准测试（bench 子命令）提前验证；2. 如不达标考虑 batch insert（需调整事务策略）；3. modernc.org/sqlite 近年性能已大幅改善，实测差距 < 20% | 开发 |
| R-05 | tool_input 包含 API Key 等敏感信息泄露 | 安全合规风险 | 高 | 1. M1 版本在 Sanitizer 中实现基础正则脱敏（匹配常见 key pattern 如 `sk-`、`ghp_`、`AKIA` 前缀）；2. 文档明确告知用户数据保留在本地；3. 后续版本考虑可配置脱敏规则 | 开发+安全 |
| R-06 | Subagent hook 事件与主 session 事件混淆 | 调用链数据混乱 | 中 | 1. SubagentStop 事件中的 `session_id` 与主 session 不同，通过 session_id 天然隔离；2. Subagent 内部如果也有 Pre/PostToolUse，其 session_id 也与主 session 不同，不会交叉匹配；3. 未来可通过 parent_session_id 字段建立关联（P2） | 架构 |
| R-07 | 大量短生命周期 collect 进程频繁开关数据库 | 连接开销影响延迟 | 中 | 1. `*sql.DB` 配置 `SetMaxOpenConns(1)` + `SetMaxIdleConns(1)` 保持单连接热备；2. WAL 模式下连接打开开销 < 1ms；3. 如仍不达标考虑 Unix domain socket + 常驻守护进程方案（P2） | 架构 |
| R-08 | Web 前端无构建链导致代码可维护性差 | 后续功能迭代困难 | 低 | 1. 页面数固定为 4，交互复杂度有限；2. JS 代码按页面拆分为独立模块；3. 如后续功能膨胀再引入轻量框架（P2 预留） | 开发 |

---

## 附录 A: 配置结构体

```go
// internal/config/config.go
type Config struct {
    Storage   StorageConfig   `yaml:"storage"`
    Collector CollectorConfig `yaml:"collector"`
    Dashboard DashboardConfig `yaml:"dashboard"`
    Stats     StatsConfig     `yaml:"stats"`
    Alerts    AlertsConfig    `yaml:"alerts"`
    Export    ExportConfig    `yaml:"export"`
    Logging   LoggingConfig   `yaml:"logging"`
}

type StorageConfig struct {
    Type           string `yaml:"type"`              // "sqlite"(默认)
    Path           string `yaml:"path"`              // 空=项目目录下 .agent-insight/insight.db
    RetentionDays  int    `yaml:"retention_days"`    // 默认 30
    MaxInputSize   int    `yaml:"max_input_size"`    // 默认 10240 (10KB)
    MaxOutputSize  int    `yaml:"max_output_size"`   // 默认 10240 (10KB)
}

type CollectorConfig struct {
    TimeoutMs  int  `yaml:"timeout_ms"`   // 默认 5000
    BatchSize  int  `yaml:"batch_size"`   // 默认 1
    AsyncWrite bool `yaml:"async_write"`  // 默认 true
}

type DashboardConfig struct {
    Host             string `yaml:"host"`               // 默认 "127.0.0.1"
    Port             int    `yaml:"port"`                // 默认 8080
    RefreshIntervalMs int   `yaml:"refresh_interval_ms"` // 默认 1000
}

type StatsConfig struct {
    AggregationInterval string `yaml:"aggregation_interval"` // 默认 "5m"
}

type AlertsConfig struct {
    Enabled  bool         `yaml:"enabled"`   // 默认 false
    Rules    []RuleConfig `yaml:"rules"`
    Channels []ChannelConfig `yaml:"channels"`
}

type RuleConfig struct {
    ID        string `yaml:"id"`         // "A-01" ~ "A-05"
    Threshold int    `yaml:"threshold"`
    Window    string `yaml:"window,omitempty"` // 仅 A-03 需要
    Enabled   bool   `yaml:"enabled"`
}

type ChannelConfig struct {
    Type     string `yaml:"type"`      // "stderr"|"notify"|"webhook"|"file"
    MinLevel string `yaml:"min_level"` // "WARN"|"ERROR"|"CRITICAL"
    URL      string `yaml:"url,omitempty"`     // webhook URL
    Path     string `yaml:"path,omitempty"`    // file path
}

type ExportConfig struct {
    DefaultFormat string `yaml:"default_format"` // "json"
}

type LoggingConfig struct {
    Level string `yaml:"level"` // "debug"|"info"|"warn"(默认)|"error"
    Path  string `yaml:"path"`  // 空=stderr
}
```

## 附录 B: 环境变量覆盖规则

环境变量采用 `AGENT_INSIGHT_` 前缀 + 下划线分隔的大写路径：

| 环境变量 | 对应配置键 | 示例值 |
|---------|----------|--------|
| `AGENT_INSIGHT_DB_PATH` | storage.path | `/data/insight.db` |
| `AGENT_INSIGHT_RETENTION_DAYS` | storage.retention_days | `30` |
| `AGENT_INSIGHT_DASHBOARD_PORT` | dashboard.port | `9090` |
| `AGENT_INSIGHT_DASHBOARD_HOST` | dashboard.host | `0.0.0.0` |
| `AGENT_INSIGHT_ALERTS_ENABLED` | alerts.enabled | `true` |
| `AGENT_INSIGHT_LOG_LEVEL` | logging.level | `debug` |

优先级：CLI flag > 环境变量 > 配置文件 > 默认值

## 附录 C: 数据保留与清理策略

```
启动时：
  1. 执行 retention 清理（删除超期 hook_events 和 stats_hourly）
  2. 执行 PRAGMA wal_checkpoint(TRUNCATE) 回收 WAL 空间
  3. 执行 PRAGMA optimize 更新统计信息

定时任务（dashboard 模式）：
  - retention cleaner: 每小时执行一次
  - stats flusher: 每 5 分钟执行一次
  - session scanner: 每 10 分钟执行一次
  - WAL checkpoint: 每 30 分钟执行一次

清理 SQL：
  DELETE FROM hook_events WHERE created_at < strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-{retention_days} days');
  DELETE FROM stats_hourly WHERE bucket_hour < strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-{retention_days} days');
  DELETE FROM session_stats WHERE ended_at < strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '-{retention_days} days');
```
