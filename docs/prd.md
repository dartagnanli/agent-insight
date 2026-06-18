# agent-insight PRD

> Claude Code Hooks 可观测性平台 -- 产品需求文档
>
> 版本: v0.1.0-draft
> 日期: 2026-06-18
> 作者: 产品经理
> 状态: Draft

---

## 1. 项目概述

### 1.1 背景

Claude Code 是 Anthropic 推出的终端 AI 编程助手，其 hooks 机制允许用户在 8 个生命周期节点（SessionStart、UserPromptSubmit、PreToolUse、PostToolUse、Notification、Stop、SubagentStop、PreCompact）插入自定义脚本，实现对工具调用的拦截、审计和扩展。

然而，当前的 hooks 机制缺乏可观测性：

- 用户无法知道 hooks 被触发了多少次、执行耗时如何
- 无法追踪一次会话中完整的工具调用链路
- 无法量化 hook 拦截率和对整体延迟的影响
- 无法在发生异常时快速定位问题 hook
- 多 hook 协作场景下缺少时序和因果关系视图

### 1.2 项目愿景

**agent-insight** 为 Claude Code hooks 生态提供生产级可观测性能力，让开发者和团队对 hooks 的行为"看得见、查得到、管得住"。

### 1.3 目标用户

| 角色 | 场景 |
|------|------|
| Claude Code 个人用户 | 了解自己配置的 hooks 运行情况，排查 hook 异常 |
| 团队技术负责人 | 审计团队 hooks 配置的效果，优化工具使用策略 |
| Hook 开发者 | 调试自研 hook 脚本，验证拦截逻辑和耗时 |
| DevOps / SRE | 监控 hooks 对 CI/CD 流水线的影响，设置告警 |

### 1.4 核心价值主张

1. **零侵入接入**：作为 Claude Code 的一个 hook 脚本注册即可，无需修改 Claude Code 本身
2. **完整调用链**：从 session 维度串联所有 hook 事件，还原真实执行时序
3. **低延迟采集**：hook 脚本本身耗时 < 5ms，不拖慢 Claude Code 响应
4. **开箱即用**：单二进制文件，一条命令启动仪表板

---

## 2. 需求拆解与优先级

### 2.1 优先级定义

- **P0**：MVP 必须，无此功能产品不可用
- **P1**：核心体验，首版发布应包含
- **P2**：增强体验，后续版本迭代

### 2.2 功能点矩阵

| 编号 | 功能点 | 优先级 | 依赖 | 预估复杂度 |
|------|--------|--------|------|-----------|
| F-01 | Hook 事件采集与存储 | P0 | 无 | 高 |
| F-02 | 调用链时序追踪 | P0 | F-01 | 高 |
| F-03 | CLI 查询命令 | P0 | F-01, F-02 | 中 |
| F-04 | 统计分析引擎 | P1 | F-01 | 中 |
| F-05 | Web 实时仪表板 | P1 | F-01, F-04 | 高 |
| F-06 | 会话关联聚合 | P1 | F-02 | 中 |
| F-07 | 告警通知 | P2 | F-04 | 中 |
| F-08 | 数据导出 | P2 | F-01 | 低 |
| F-09 | 多项目过滤 | P2 | F-01 | 中 |
| F-10 | Hook 性能基准测试 | P2 | F-01 | 低 |

---

## 3. 功能设计

### 3.1 F-01: Hook 事件采集与存储

#### 3.1.1 交互流程

```
Claude Code
    |
    | (触发 hook 事件，通过 stdin 传入 JSON)
    v
agent-insight collector (注册为 hook handler)
    |
    | 1. 从 stdin 读取 JSON payload
    | 2. 补充采集元数据（时间戳、进程 PID、host）
    | 3. 本地持久化写入
    |
    v
本地存储 (SQLite / JSONL 文件)
```

#### 3.1.2 核心逻辑

**采集层**：agent-insight 的 collector 子命令注册为 Claude Code 的 hook handler。当 Claude Code 触发 hook 事件时：

1. 从 stdin 读取完整 JSON，解析为 HookEvent 结构体
2. 为事件分配全局递增 ID
3. 记录接收到事件的精确时间戳（monotonic clock）
4. 计算自身采集耗时，写入事件元数据
5. 异步写入本地存储（不阻塞 stdout 返回）
6. 对 PreToolUse 类型事件：透传原始 stdin 内容，不干扰 hook 链

**存储层**：

- 主存储：SQLite（单文件，嵌入式，支持复杂查询）
- WAL 模式启用，支持并发读写
- 表设计：

```sql
CREATE TABLE hook_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id    TEXT NOT NULL,           -- 全局唯一事件 ID (UUID)
    session_id  TEXT NOT NULL,           -- Claude Code session ID
    event_type  TEXT NOT NULL,           -- PreToolUse/PostToolUse/...
    tool_name   TEXT,                    -- 工具名称 (Bash/Write/Edit/...)
    tool_input  TEXT,                    -- 工具输入 JSON
    tool_output TEXT,                    -- 工具输出 JSON (仅 PostToolUse)
    cwd         TEXT,                    -- 工作目录
    transcript_path TEXT,               -- transcript 文件路径
    blocked     BOOLEAN DEFAULT FALSE,  -- 是否被拦截 (exit 2)
    block_reason TEXT,                  -- 拦截原因 (stderr 内容)
    hook_exit_code INTEGER,            -- hook 脚本退出码
    hook_duration_ms INTEGER,          -- hook 自身执行耗时
    collect_duration_ms INTEGER,       -- 采集耗时
    pid         INTEGER,               -- Claude Code 进程 PID
    hostname    TEXT,                   -- 主机名
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_events_session ON hook_events(session_id);
CREATE INDEX idx_events_type ON hook_events(event_type);
CREATE INDEX idx_events_tool ON hook_events(tool_name);
CREATE INDEX idx_events_time ON hook_events(created_at);
```

**数据保留策略**：

- 默认保留 30 天数据
- 超期数据自动清理（可配置）
- 支持按 session 手动清理

#### 3.1.3 Claude Code 集成配置

用户需在 `.claude/settings.json` 中注册 agent-insight：

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "agent-insight collect --event PreToolUse"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "agent-insight collect --event PostToolUse"
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "agent-insight collect --event SessionStart"
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "agent-insight collect --event Stop"
          }
        ]
      }
    ]
  }
}
```

提供 `agent-insight init` 命令自动生成上述配置。

#### 3.1.4 验收标准

```gherkin
Scenario: 采集 PreToolUse 事件
  Given agent-insight 已注册为 Claude Code 的 PreToolUse hook
  When Claude Code 即将执行 Bash 工具，通过 stdin 传入 JSON 事件
  Then agent-insight 在 5ms 内完成数据采集并 exit 0
  And 数据库 hook_events 表新增一条记录，event_type 为 "PreToolUse"
  And tool_name 为 "Bash"
  And collect_duration_ms <= 5

Scenario: 采集 PostToolUse 事件并记录工具输出
  Given agent-insight 已注册为 Claude Code 的 PostToolUse hook
  When Claude Code 执行 Write 工具完成，通过 stdin 传入包含 tool_response 的 JSON
  Then 数据库记录中 tool_output 字段非空
  And tool_output 包含 Write 工具的执行结果

Scenario: 采集过程不干扰 hook 链
  Given agent-insight 注册在 PreToolUse 事件的其他 hook 之前
  When Claude Code 触发 PreToolUse 事件
  Then agent-insight exit 0，不阻断后续 hook 执行
  And 不修改 stdin 内容

Scenario: 存储路径可配置
  Given 用户设置环境变量 AGENT_INSIGHT_DB_PATH=/custom/path/insight.db
  When agent-insight collect 执行
  Then 数据写入 /custom/path/insight.db
```

---

### 3.2 F-02: 调用链时序追踪

#### 3.2.1 交互流程

```
SessionStart ─────────────────────────────────────────────── Stop
    |                                                            |
    +-- UserPromptSubmit ────────────────────────── Stop         |
    |       |                                        |          |
    |       +-- PreToolUse(Bash) ── PostToolUse(Bash)           |
    |       +-- PreToolUse(Write) ── PostToolUse(Write)         |
    |       +-- PreToolUse(Bash) ── PostToolUse(Bash)           |
    |                                                          |
    +-- UserPromptSubmit ────────────────────────── Stop         |
            |                                                  |
            +-- PreToolUse(Edit)── PostToolUse(Edit)            |
            +-- PreToolUse(Bash)── PostToolUse(Bash)            |
```

#### 3.2.2 核心逻辑

**调用链模型**：

每条调用链以 `session_id` 为根节点，按时间戳排序串联所有事件。关键概念：

- **Trace**：一次会话的完整事件序列，以 session_id 标识
- **Span**：单个 hook 事件，包含起止时间、工具名称、执行结果
- **Link**：PreToolUse 与对应 PostToolUse 的关联关系

**Pre/PostToolUse 自动关联**：

通过 `session_id + tool_name + 时间窗口` 自动配对 PreToolUse 和 PostToolUse 事件：

1. PreToolUse 事件到达时，记录为 "pending span"
2. PostToolUse 事件到达时，查找同一 session 内最近的同名 tool 的 pending span
3. 配对成功则计算工具实际执行耗时（Post 时间 - Pre 时间）
4. 若 30s 内未配对，标记为 "orphan"（可能被拦截或超时）

**拦截识别**：

- exit code = 2 的 PreToolUse 标记为 `blocked=true`
- 被 PreToolUse 拦截的工具调用不会产生 PostToolUse，通过 orphan 检测补全

**时序重建**：

- 所有事件按 `created_at` 排序
- 支持按 wall clock 和 monotonic clock 双时序
- 相同 session 的事件强制有序，跨 session 不保证

#### 3.2.3 验收标准

```gherkin
Scenario: 自动关联 Pre/PostToolUse
  Given 同一 session 内依次发生 PreToolUse(Bash) 和 PostToolUse(Bash)
  When 两个事件的时间差 < 30s
  Then 系统自动将二者关联为一次完整的工具调用
  And 计算出的工具执行耗时 = PostToolUse.timestamp - PreToolUse.timestamp

Scenario: 识别被拦截的工具调用
  Given PreToolUse(Bash) 事件中 hook_exit_code = 2
  When 30s 内未收到对应的 PostToolUse(Bash)
  Then 该 PreToolUse 标记为 blocked=true
  And 该 pending span 标记为 orphan，不再等待配对
  And block_reason 记录 stderr 输出内容

Scenario: 完整会话调用链查询
  Given session abc123 产生 20 条 hook 事件
  When 用户执行 "agent-insight trace abc123"
  Then 输出按时间排序的完整事件序列
  And PreToolUse/PostToolUse 已配对显示
  And 拦截事件用 [BLOCKED] 标记
  And 每个工具调用显示执行耗时

Scenario: 处理同一工具连续调用
  Given 同一 session 内连续两次 PreToolUse(Bash)
  When 第二次 PreToolUse(Bash) 到达
  Then 系统按 FIFO 原则配对第一次的 pending span
  And 第二次 PreToolUse 进入新的 pending span
```

---

### 3.3 F-03: CLI 查询命令

#### 3.3.1 命令结构

```
agent-insight
  |-- collect          # 采集 hook 事件（被 Claude Code 调用）
  |-- init             # 初始化配置，生成 settings.json hook 注册
  |-- trace <session>  # 查看指定 session 的调用链
  |-- sessions         # 列出所有已知 session
  |-- stats            # 显示统计摘要
  |-- dashboard        # 启动 Web 仪表板
  |-- config           # 查看和修改配置
  |-- version          # 版本信息
```

#### 3.3.2 核心命令设计

**`agent-insight trace <session_id>`**：

输出 waterfall 风格的调用链：

```
Session: abc123  Duration: 2m 34s  Events: 42

  0:00:00  SessionStart
  0:00:01  UserPromptSubmit
  0:00:02  PreToolUse  Bash   "npm test"              2ms
  0:00:05  PostToolUse Bash   exit=0                   3s
  0:00:06  PreToolUse  Write  "src/auth.ts"            1ms
  0:00:06  [BLOCKED]   Write  "Style check failed"     0ms
  0:00:08  PreToolUse  Write  "src/auth.ts"            1ms
  0:00:09  PostToolUse Write  success=true             1s
  ...
  0:02:34  Stop
```

**`agent-insight stats`**：

输出统计摘要：

```
=== agent-insight Stats (Last 24h) ===

Total Events:     1,247
Sessions:         12
Tools Used:       Bash(489) Write(312) Edit(198) Read(148) ...

Block Rate:       8.3% (104/1,247)
Avg Hook Latency: 1.2ms
P99 Hook Latency: 4.8ms
Avg Tool Duration: 1.8s (Bash: 3.2s, Write: 0.8s, Edit: 1.1s)

Top Blocked:      Write(45) Bash(32) Edit(27)
Slowest Tools:    Bash(avg 3.2s, p99 12.1s)
```

**`agent-insight sessions`**：

```
ID              Started              Events  Duration  Blocked
abc123          2026-06-18 10:23     42      2m 34s    3
def456          2026-06-18 09:15     128     15m 12s   12
ghi789          2026-06-17 17:42     67      5m 45s    0
```

#### 3.3.3 验收标准

```gherkin
Scenario: 查询不存在的 session
  Given 数据库中不存在 session_id "nonexist"
  When 用户执行 "agent-insight trace nonexist"
  Then 输出 "Session not found: nonexist"
  And 退出码为 1

Scenario: stats 时间范围过滤
  Given 用户执行 "agent-insight stats --since 1h"
  Then 仅统计最近 1 小时内的事件数据

Scenario: sessions 排序
  Given 用户执行 "agent-insight sessions --sort events"
  Then session 列表按事件数降序排列

Scenario: trace 输出包含完整信息
  Given session abc123 存在
  When 用户执行 "agent-insight trace abc123"
  Then 每行包含：序号、时间偏移、事件类型、工具名、简要信息、耗时
  And 拦截事件显示 [BLOCKED] 标记
```

---

### 3.4 F-04: 统计分析引擎

#### 3.4.1 核心逻辑

**统计维度**：

| 维度 | 指标 | 计算方式 |
|------|------|---------|
| 事件类型 | 各类型事件数 | COUNT GROUP BY event_type |
| 工具使用 | 各工具调用次数 | COUNT GROUP BY tool_name |
| 拦截率 | 总拦截率、各工具拦截率 | blocked=true / total |
| 耗时分析 | 平均耗时、P50/P95/P99 | percentile 聚合 |
| 时段分布 | 每小时事件数 | 按小时分桶 |
| Session 聚合 | 每 session 事件数、时长、拦截数 | GROUP BY session_id |

**聚合策略**：

- 实时聚合：内存中维护滑动窗口计数器（1h / 6h / 24h）
- 持久聚合：每 5 分钟将内存聚合快照写入 SQLite
- 查询优先读聚合表，回退读原始表

**聚合表设计**：

```sql
CREATE TABLE stats_hourly (
    bucket_hour  TIMESTAMP NOT NULL,
    event_type   TEXT NOT NULL,
    tool_name    TEXT,
    event_count  INTEGER DEFAULT 0,
    block_count  INTEGER DEFAULT 0,
    avg_duration_ms  REAL,
    p50_duration_ms  REAL,
    p95_duration_ms  REAL,
    p99_duration_ms  REAL,
    PRIMARY KEY (bucket_hour, event_type, tool_name)
);
```

#### 3.4.2 验收标准

```gherkin
Scenario: 计算工具拦截率
  Given 最近 24h 内 Write 工具调用 100 次，其中 8 次被拦截
  When 用户执行 "agent-insight stats --tool Write"
  Then 显示 Write 工具拦截率为 8.0%

Scenario: P99 延迟计算
  Given 最近 1h 内 Bash 工具调用 200 次，其中 198 次 < 5s，2 次 > 10s
  When 计算耗时 P99
  Then P99 值 > 5s（覆盖了最慢的 2% 请求）

Scenario: 统计数据自动刷新
  Given 新事件持续写入
  When 5 分钟过去
  Then stats_hourly 表自动更新最新聚合数据

Scenario: 空数据查询
  Given 数据库中无任何事件
  When 用户执行 "agent-insight stats"
  Then 输出 "No data available" 而非报错
```

---

### 3.5 F-05: Web 实时仪表板

#### 3.5.1 交互流程

```
用户浏览器 <--- WebSocket / SSE ---> agent-insight dashboard (Go HTTP server)
                                          |
                                          | 读取 SQLite + 内存聚合
                                          v
                                       本地存储
```

#### 3.5.2 页面设计

**页面一：事件流 (Event Stream)**

- 实时滚动的事件列表，新事件自动推送到顶部
- 每行显示：时间戳、事件类型、工具名、简要输入、耗时、拦截状态
- 支持按事件类型、工具名、session 过滤
- 点击事件展开完整 JSON 详情

**页面二：统计概览 (Dashboard)**

- 四个核心指标卡片：总事件数、拦截率、平均 Hook 延迟、P99 延迟
- 工具使用分布饼图
- 事件类型分布柱状图
- 24h 事件趋势折线图
- Top 10 拦截工具排行

**页面三：调用瀑布图 (Trace Waterfall)**

- 左侧选择 session，右侧显示瀑布图
- 每个工具调用显示为一个横条，长度代表耗时
- PreToolUse 到 PostToolUse 之间用连线标注
- 被拦截的事件用红色标记
- 支持缩放和拖拽

**页面四：Session 列表 (Sessions)**

- Session 表格：ID、开始时间、事件数、总时长、拦截数
- 支持搜索和排序
- 点击行跳转到该 session 的 Trace Waterfall

#### 3.5.3 技术选型

- 服务端：Go net/http + gorilla/websocket (或类似)
- 前端：嵌入式 SPA，编译时打包进 Go 二进制 (go:embed)
- 图表库：Chart.js 或 ECharts (通过 CDN 或内嵌)
- 实时推送：WebSocket 优先，SSE 降级

#### 3.5.4 验收标准

```gherkin
Scenario: 实时事件推送
  Given 仪表板已打开并连接 WebSocket
  When 新的 hook 事件被采集
  Then 事件在 1s 内出现在事件流页面
  And 新事件高亮显示 2s 后恢复正常

Scenario: 瀑布图展示调用链
  Given session abc123 包含 10 次工具调用
  When 用户在瀑布图页面选择 session abc123
  Then 显示 10 个横条，按时间排列
  And 每个横条长度与实际耗时成正比
  And 被拦截的事件显示为红色

Scenario: 仪表板端口配置
  Given 用户执行 "agent-insight dashboard --port 9090"
  Then Web 服务监听 0.0.0.0:9090
  And 浏览器访问 http://localhost:9090 可打开仪表板

Scenario: 仪表板无外部依赖
  Given 用户在内网环境无法访问外部 CDN
  When 打开仪表板页面
  Then 所有静态资源从 Go 嵌入的文件系统加载
  And 页面功能正常渲染

Scenario: 多浏览器标签页同时查看
  Given 用户打开 3 个浏览器标签页访问同一仪表板
  Then 3 个标签页均能正常接收实时事件推送
  And 无数据丢失或重复
```

---

### 3.6 F-06: 会话关联聚合

#### 3.6.1 核心逻辑

**Session 生命周期追踪**：

- SessionStart 事件标记 session 开始
- Stop/SubagentStop 事件标记 session 结束
- 计算 session 总时长 = Stop.timestamp - SessionStart.timestamp
- 无 Stop 事件时，以最后一条事件时间 + 5min 超时标记结束

**Session 聚合指标**：

```sql
CREATE TABLE session_stats (
    session_id       TEXT PRIMARY KEY,
    started_at       TIMESTAMP,
    ended_at         TIMESTAMP,
    duration_secs    INTEGER,
    total_events     INTEGER,
    tool_calls       INTEGER,
    blocked_calls    INTEGER,
    block_rate       REAL,
    tools_used       TEXT,           -- JSON array of unique tool names
    avg_tool_duration_ms REAL,
    p99_tool_duration_ms REAL,
    project_path     TEXT
);
```

**自动聚合触发**：

- 每收到 Stop 事件时立即聚合该 session
- 每 10 分钟扫描未聚合的已完成 session
- 启动时扫描数据库补全缺失的聚合记录

#### 3.6.2 验收标准

```gherkin
Scenario: Session 自动聚合
  Given session abc123 收到 Stop 事件
  When 聚合引擎触发
  Then session_stats 表新增一条记录
  And total_events 等于该 session 的实际事件数
  And duration_secs 等于 Stop.timestamp - SessionStart.timestamp

Scenario: 无 Stop 事件的 session 超时
  Given session xyz789 的最后一条事件距今超过 5 分钟
  And 没有 Stop 事件
  When 聚合引擎扫描
  Then session 标记为 ended，ended_at 为最后事件时间 + 5min

Scenario: 查询 session 聚合数据
  When 用户执行 "agent-insight sessions --detail abc123"
  Then 输出 session 的完整聚合信息
  And 包含：事件数、工具调用数、拦截数、拦截率、工具列表、平均耗时
```

---

### 3.7 F-07: 告警通知

#### 3.7.1 告警规则

| 规则 ID | 条件 | 级别 | 默认阈值 |
|---------|------|------|---------|
| A-01 | Hook 执行耗时超过阈值 | WARN | > 1000ms |
| A-02 | Hook 执行返回非零退出码 | ERROR | exit != 0 且 != 2 |
| A-03 | 连续拦截率飙升 | WARN | 5min 内拦截率 > 50% |
| A-04 | Session 内工具调用数异常 | WARN | 单 session > 500 次 |
| A-05 | Hook 采集进程崩溃 | CRITICAL | collector 异常退出 |

#### 3.7.2 通知渠道

| 渠道 | 优先级 | 说明 |
|------|--------|------|
| 终端 stderr | P0 | 默认启用，输出到 Claude Code 的 stderr |
| 系统通知 | P1 | macOS/osascript, Linux/notify-send |
| Webhook | P2 | 用户配置 URL，POST JSON 告警体 |
| 文件日志 | P1 | 写入 `~/.agent-insight/alerts.log` |

#### 3.7.3 告警配置

```yaml
# ~/.agent-insight/config.yaml
alerts:
  enabled: true
  rules:
    - id: A-01
      threshold: 1000    # ms
      enabled: true
    - id: A-03
      threshold: 50      # %
      window: 5m
      enabled: true
  channels:
    - type: stderr
      min_level: WARN
    - type: webhook
      url: https://hooks.example.com/agent-insight
      min_level: ERROR
```

#### 3.7.4 验收标准

```gherkin
Scenario: Hook 超时告警
  Given 告警规则 A-01 已启用，阈值为 1000ms
  When 一个 PreToolUse hook 执行耗时 1200ms
  Then 输出 WARN 级别告警到 stderr
  And 告警内容包含事件 ID、工具名、实际耗时

Scenario: 拦截率飙升告警
  Given 告警规则 A-03 已启用，阈值 50%，窗口 5min
  When 最近 5 分钟内 60% 的 PreToolUse 调用被拦截
  Then 输出 WARN 级别告警
  And 包含当前拦截率、窗口时间、影响工具列表

Scenario: 告警静默期
  Given 同一规则 5 分钟内已触发过告警
  When 再次满足告警条件
  Then 不重复发送告警（静默期 5 分钟）

Scenario: 告警禁用
  Given 配置 alerts.enabled = false
  When 任何告警条件满足
  Then 不发送任何告警通知
```

---

### 3.8 F-08: 数据导出

#### 3.8.1 支持格式

| 格式 | 用途 | 说明 |
|------|------|------|
| JSON | 数据交换 | 完整事件数据，可导入其他工具 |
| CSV | 数据分析 | 扁平化字段，适合 Excel / Pandas |
| HTML | 报告分享 | 单文件快照，包含图表和统计 |

#### 3.8.2 导出命令

```bash
agent-insight export --format json --since 24h --output report.json
agent-insight export --format csv --session abc123 --output trace.csv
agent-insight export --format html --since 7d --output weekly-report.html
```

#### 3.8.3 验收标准

```gherkin
Scenario: 导出 JSON 格式
  When 用户执行 "agent-insight export --format json --since 1h"
  Then 生成包含最近 1h 所有事件的 JSON 文件
  And JSON 结构与 stdin 接收的格式一致，附加采集元数据

Scenario: 导出 CSV 格式
  When 用户执行 "agent-insight export --format csv --session abc123"
  Then 生成 CSV 文件，每行一条事件
  And 列包含：id, event_type, tool_name, blocked, duration_ms, created_at

Scenario: 导出 HTML 报告
  When 用户执行 "agent-insight export --format html --since 7d"
  Then 生成独立 HTML 文件，包含统计图表
  And 文件可离线浏览，不依赖外部资源
```

---

### 3.9 F-09: 多项目过滤

#### 3.9.1 核心逻辑

- 所有项目数据集中存储在全局数据库 `~/.agent-insight/insight.db`
- 通过 `cwd` 字段（Claude Code 传入）区分项目来源
- 查询命令支持 `--project` flag 按项目路径精确过滤（支持相对路径）

#### 3.9.2 验收标准

```gherkin
Scenario: 全局数据集中存储
  Given 用户在 /project-a 和 /project-b 分别使用 Claude Code
  When 两个项目的 hook 事件被采集
  Then 全局数据库 ~/.agent-insight/insight.db 包含两个项目的事件
  And 每条记录的 cwd 字段正确标识所属项目

Scenario: 按项目过滤查询
  When 用户执行 "agent-insight stats --project /project-a"
  Then 仅显示 /project-a 的事件数据
```

---

### 3.10 F-10: Hook 性能基准测试

#### 3.10.1 核心逻辑

提供基准测试模式，模拟 hook 事件注入，测量采集器的吞吐量和延迟：

```bash
agent-insight bench --events 10000 --concurrency 4
```

输出：

```
Events:     10,000
Duration:   1.2s
Throughput: 8,333 events/s
Avg Latency: 0.12ms
P99 Latency: 0.48ms
DB Size:    12.4 MB
```

#### 3.10.2 验收标准

```gherkin
Scenario: 基准测试执行
  When 用户执行 "agent-insight bench --events 1000"
  Then 输出吞吐量、延迟分布、数据库大小指标
  And 使用临时数据库，不污染生产数据
```

---

## 4. 技术约束

### 4.1 性能约束

| 约束项 | 要求 | 理由 |
|--------|------|------|
| 采集延迟 | 单事件采集耗时 < 5ms (P99) | hook 执行在 Claude Code 主线程，高延迟影响用户体验 |
| 内存占用 | 常驻内存 < 50MB | 终端工具不应成为资源瓶颈 |
| 数据库大小 | 1000 events < 2MB | 避免磁盘空间快速膨胀 |
| 写入吞吐 | >= 1000 events/s | 覆盖高频使用场景 |
| Web 仪表板首屏 | < 500ms | 保证交互体验 |
| WebSocket 推送延迟 | < 1s | 实时感 |

### 4.2 安全约束

| 约束项 | 要求 | 理由 |
|--------|------|------|
| 数据本地化 | 所有数据仅存储在用户本地磁盘 | 事件数据可能包含代码和敏感信息 |
| 无网络外传 | 默认禁止任何外部网络请求 | 防止代码和工具输入意外泄露 |
| tool_input 截断 | 超过 10KB 的 tool_input 截断存储 | 防止超大输入撑爆数据库 |
| tool_output 截断 | 超过 10KB 的 tool_output 截断存储 | 同上 |
| Webhook 白名单 | 仅允许用户显式配置的 URL | 告警 webhook 可能泄露事件元数据 |
| SQLite 文件权限 | 数据库文件权限 0600 | 防止其他用户读取事件数据 |

### 4.3 兼容性约束

| 约束项 | 要求 |
|--------|------|
| Go 版本 | >= 1.22 |
| 操作系统 | macOS (arm64/amd64), Linux (amd64/arm64), Windows (amd64) |
| Claude Code | >= 1.0 (hooks 功能可用版本) |
| CGO | 禁用 (纯 Go SQLite 驱动，如 modernc.org/sqlite) |
| 二进制发布 | 单文件，无外部依赖 |
| 配置文件 | 兼容 Claude Code settings.json 格式 |

### 4.4 可靠性约束

| 约束项 | 要求 | 理由 |
|--------|------|------|
| 采集失败容错 | 采集器写入失败时 exit 0，不阻断 Claude Code | 可观测性工具不应成为故障源 |
| 数据库损坏恢复 | 启动时检测并自动重建损坏的数据库 | 终端用户不应手动修复数据库 |
| 并发安全 | 多进程同时写入 SQLite 不丢失数据 | Claude Code 可能启动多个并行 hook |
| 优雅关闭 | SIGTERM 时完成当前写入后退出 | 不丢失正在处理的事件 |

### 4.5 可扩展性约束

| 约束项 | 要求 |
|--------|------|
| 采集器插件化 | 支持自定义采集器注册（未来可接入 OpenTelemetry） |
| 存储后端抽象 | 当前 SQLite，接口设计支持未来替换为 Postgres / ClickHouse |
| 统计引擎可扩展 | 支持注册自定义聚合函数 |
| Webhook 模板 | 支持自定义告警模板 |

---

## 5. 项目目录结构

遵循 golang-standards/project-layout：

```
agent-insight/
|-- cmd/
|   |-- agent-insight/
|   |   |-- main.go              # 入口
|   |-- agent-insight-collector/ # 独立采集器二进制（可选，轻量化部署）
|
|-- internal/
|   |-- collector/               # 事件采集逻辑
|   |-- storage/                 # 存储层（SQLite 抽象）
|   |-- trace/                   # 调用链追踪逻辑
|   |-- stats/                   # 统计分析引擎
|   |-- alert/                   # 告警引擎
|   |-- dashboard/               # Web 仪表板服务
|   |-- config/                  # 配置管理
|   |-- export/                  # 数据导出
|   |-- session/                 # 会话聚合
|
|-- pkg/
|   |-- event/                   # HookEvent 公共模型（可被外部引用）
|   |-- hookinput/               # stdin JSON 解析公共库
|
|-- web/
|   |-- assets/                  # 前端静态资源
|   |-- dist/                    # 构建产物 (go:embed)
|
|-- api/
|   |-- openapi.yaml             # 仪表板 REST API 定义
|
|-- configs/
|   |-- config.example.yaml      # 配置模板
|
|-- scripts/
|   |-- install.sh               # 安装脚本
|   |-- build.sh                 # 构建脚本
|
|-- deploy/
|   |-- Dockerfile
|
|-- docs/
|   |-- prd.md                   # 本文档
|
|-- test/
|   |-- integration/             # 集成测试
|   |-- testdata/                # 测试固件数据
|
|-- go.mod
|-- go.sum
|-- Makefile
|-- LICENSE
|-- .goreleaser.yaml             # 多平台发布配置
```

---

## 6. 里程碑规划

### M1: MVP (P0 功能) -- 预计 2 周

| 功能 | 交付物 |
|------|--------|
| F-01 Hook 事件采集与存储 | collector 子命令 + SQLite schema + init 命令 |
| F-02 调用链时序追踪 | Pre/Post 自动关联 + orphan 检测 |
| F-03 CLI 查询命令 | trace / sessions / stats 命令 |

**M1 验收标准**：用户通过 `agent-insight init` 注册 hook 后，能在终端用 CLI 查询完整的调用链和统计数据。

### M2: 可视化 (P1 功能) -- 预计 3 周

| 功能 | 交付物 |
|------|--------|
| F-04 统计分析引擎 | 聚合表 + 滑动窗口 |
| F-05 Web 实时仪表板 | 4 个页面 + WebSocket 推送 |
| F-06 会话关联聚合 | session_stats 表 + 自动聚合 |

**M2 验收标准**：用户启动仪表板后，可在浏览器中实时查看事件流、统计图表、调用瀑布图。

### M3: 增强 (P2 功能) -- 预计 2 周

| 功能 | 交付物 |
|------|--------|
| F-07 告警通知 | 5 条默认规则 + 4 种通知渠道 |
| F-08 数据导出 | JSON / CSV / HTML 三种格式 |
| F-09 多项目过滤 | 全局数据库 + --project 按项目过滤 |
| F-10 性能基准测试 | bench 子命令 |

**M3 验收标准**：告警功能上线，支持数据导出和跨项目管理。

---

## 7. 风险与缓解

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| Claude Code hooks 格式变更 | 采集器失效 | 中 | 版本协商机制 + 字段兼容性检测 + 社区跟进 |
| SQLite 并发写入锁竞争 | 事件丢失 | 低 | WAL 模式 + busy timeout + 批量写入 |
| tool_input/tool_output 过大 | 数据库膨胀 | 高 | 10KB 截断 + 可配置阈值 + 定期清理 |
| Web 前端构建复杂度 | 发布周期延长 | 中 | 使用 go:embed + 最小前端依赖 + 无 Node 构建链 |
| Hook 执行超时影响 Claude Code | 用户体验恶化 | 中 | 严格 P99 < 5ms 目标 + 异步写入 + 基准测试门禁 |
| CGO 依赖导致交叉编译困难 | 多平台发布受阻 | 低 | 使用 modernc.org/sqlite 纯 Go 实现 |

---

## 8. 开放问题

| 编号 | 问题 | 负责人 | 截止日期 |
|------|------|--------|---------|
| Q-01 | Web 仪表板前端技术选型：纯 HTML/JS 还是引入轻量框架（Preact/Svelte）？ | 架构师 | M2 启动前 |
| Q-02 | 是否需要支持远程存储后端（如 Postgres）作为 P2 扩展？ | 产品 + 架构 | M3 规划时 |
| Q-03 | tool_input 中可能包含 API Key 等敏感信息，是否需要内置脱敏？ | 安全 + 产品 | M1 启动前 |
| Q-04 | Claude Code Subagent 的 hook 事件是否需要特殊处理（嵌套 trace）？ | 架构 | M1 启动前 |
| Q-05 | Windows 平台的系统通知兼容方案？ | 开发 | M3 启动前 |

---

## 附录 A: Claude Code Hook 事件 JSON Schema

### 通用字段

| 字段 | 类型 | 说明 | 所有事件 |
|------|------|------|---------|
| session_id | string | 会话唯一标识 | 是 |
| cwd | string | 当前工作目录 | 是 |
| hook_event_name | string | 事件类型名称 | 是 |
| transcript_path | string | transcript 文件路径 | 部分 |

### PreToolUse / PostToolUse 附加字段

| 字段 | 类型 | 说明 |
|------|------|------|
| tool_name | string | 工具名称 (Bash/Write/Edit/Read/...) |
| tool_input | object | 工具输入参数 |
| tool_response | object | 工具执行结果 (仅 PostToolUse) |

### Hook 退出码语义

| 退出码 | 含义 | 对 Claude Code 的影响 |
|--------|------|---------------------|
| 0 | 正常完成 | 继续执行（stdout JSON 可选） |
| 2 | 阻断 | 阻止当前操作，stderr 内容作为反馈返回给 Claude |
| 其他 | 错误 | hook 执行失败，不阻断操作 |

### 环境变量

| 变量名 | 说明 |
|--------|------|
| CLAUDE_EVENT | 等同于 hook_event_name |
| CLAUDE_TOOL_NAME | 工具名（Pre/PostToolUse） |
| CLAUDE_TOOL_INPUT | 工具输入 JSON（Pre/PostToolUse） |
| CLAUDE_TOOL_OUTPUT | 工具输出 JSON（仅 PostToolUse） |
| CLAUDE_FILE_PATH | 涉及的文件路径（部分工具） |

---

## 附录 B: 配置文件参考

### 默认配置 (~/.agent-insight/config.yaml)

```yaml
# agent-insight 配置文件
storage:
  type: sqlite
  path: ""                          # 空=使用全局路径 ~/.agent-insight/insight.db
  retention_days: 30
  max_input_size: 10240             # 10KB, tool_input 截断阈值
  max_output_size: 10240            # 10KB, tool_output 截断阈值

collector:
  timeout_ms: 5000                  # 采集超时
  batch_size: 1                     # 写入批量大小
  async_write: true                 # 异步写入

dashboard:
  host: "127.0.0.1"
  port: 8080
  refresh_interval_ms: 1000         # 数据刷新间隔

stats:
  aggregation_interval: 5m         # 聚合刷新间隔

alerts:
  enabled: false
  rules: []
  channels: []

export:
  default_format: json

logging:
  level: "warn"                     # debug/info/warn/error
  path: ""                          # 空=stderr
```
