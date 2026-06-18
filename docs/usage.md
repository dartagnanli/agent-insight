# agent-insight 使用文档

## 概述

agent-insight 是 Claude Code Hooks 的可观测性平台。它注册为 Claude Code 的 hook handler，自动采集所有工具调用事件，提供调用链追踪、统计分析、实时可视化能力。

## 安装

### 从源码构建

```bash
git clone ssh://ezone.ksyun.com:23/ezone/libin18/agent-insight.git
cd agent-insight
make install
```

### 使用 go install

```bash
go install github.com/libin18/agent-insight/cmd/agent-insight@latest
```

### 验证安装

```bash
agent-insight version
# 输出: agent-insight v0.1.0 (go1.22.0, darwin/arm64, commit: abc1234)
```

## 快速开始

### 1. 初始化 Hook 注册

将 agent-insight 注册到 Claude Code 的 hook 配置中：

```bash
# 项目级配置（推荐，仅影响当前项目）
agent-insight init

# 全局配置（影响所有项目）
agent-insight init --global

# 覆盖已有配置
agent-insight init --force
```

`init` 命令会在 `.claude/settings.json`（项目级）或 `~/.claude/settings.json`（全局级）中生成以下 hook 配置：

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "agent-insight collect --event PreToolUse" }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "agent-insight collect --event PostToolUse" }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "agent-insight collect --event SessionStart" }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "agent-insight collect --event Stop" }
        ]
      }
    ]
  }
}
```

### 2. 正常使用 Claude Code

初始化后，正常使用 Claude Code 即可。每次工具调用都会自动触发 agent-insight 采集事件。

### 3. 查看数据

```bash
# 查看统计摘要
agent-insight stats

# 列出会话
agent-insight sessions

# 查看某次会话的调用链
agent-insight trace <session_id>
```

## 命令参考

### `agent-insight collect`

采集 hook 事件，由 Claude Code 自动调用，不建议手动执行。

```bash
agent-insight collect --event <event_type>
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `--event` | 是 | hook 事件类型：PreToolUse / PostToolUse / SessionStart / Stop |

**工作原理**：
1. 从 stdin 读取 Claude Code 传入的 JSON 事件
2. 解析并补充元数据（UUID、PID、hostname、时间戳）
3. 截断超长 tool_input/tool_output（>10KB）
4. 同步写入本地 SQLite 数据库
5. exit 0（任何失败都不阻断 Claude Code）

### `agent-insight init`

初始化 hook 配置。

```bash
agent-insight init [flags]
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `--global` | false | 写入全局配置 `~/.claude/settings.json` |
| `--force` | false | 覆盖已存在的配置文件 |

**注意**：
- 项目级和全局级配置可以共存，Claude Code 会合并两者
- 如果已有其他 hook 配置，建议手动合并而非使用 `--force`
- 初始化后需重启 Claude Code 才能生效

### `agent-insight trace`

查看指定 session 的完整调用链。

```bash
agent-insight trace <session_id> [flags]
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `--format` | text | 输出格式：text / json |

**text 格式输出示例**：

```
Session: abc123  Duration: 2m 34s  Events: 42

   0:00  SessionStart
   0:01  UserPromptSubmit
   0:02  PreToolUse  Bash   "npm test"              2ms
   0:05  PostToolUse Bash   exit=0                   3s
   0:06  PreToolUse  Write  "src/auth.ts"            1ms
   0:06  [BLOCKED]   Write  "Style check failed"     0ms
   0:08  PreToolUse  Write  "src/auth.ts"            1ms
   0:09  PostToolUse Write  success=true             1s
   ...
   2:34  Stop
```

**json 格式**输出完整的 Trace 对象，包含 spans 和 standalone_events。

**调用链模型**：
- **Trace**：一次会话的完整事件序列，以 session_id 标识
- **Span**：PreToolUse 和 PostToolUse 配对后的完整工具调用，包含执行耗时
- **Standalone Event**：非工具调用事件（SessionStart / Stop 等）
- **[BLOCKED]**：被 PreToolUse hook 拦截的工具调用（exit code=2）
- **[ORPHAN]**：PreToolUse 后 30s 内未收到对应 PostToolUse 的调用（可能超时或被拦截）

### `agent-insight sessions`

列出所有已知 session。

```bash
agent-insight sessions [flags]
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `--sort` | started_at | 排序字段：started_at / events / duration / blocked |
| `--order` | desc | 排序方向：asc / desc |
| `--since` | 24h | 时间过滤：1h / 6h / 24h / 7d / 30d |
| `--limit` | 50 | 最大条数 |
| `--detail` | | 指定 session_id 显示详细聚合信息 |
| `--project` | | 按项目路径过滤（支持相对路径） |

**默认输出**：

```
ID              Started              Events  Duration  Blocked
abc123          2026-06-18 10:23     42      2m 34s    3
def456          2026-06-18 09:15     128     15m 12s   12
```

**详细模式**（`--detail abc123`）：

```
Session: abc123
  Started:       2026-06-18T10:23:00Z
  Total Events:  42
  Tool Calls:    35
  Blocked:       3
  Block Rate:    8.6%

  Tools Used:
    Bash         20 calls, 2 blocked
    Write        10 calls, 1 blocked
    Edit         5 calls, 0 blocked

  Tool Duration:
    Bash         avg=3200.0ms, p99=12100.0ms
    Write        avg=800.0ms, p99=2100.0ms
```

### `agent-insight stats`

显示统计摘要。

```bash
agent-insight stats [flags]
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `--since` | 24h | 时间范围：1h / 6h / 24h / 7d / 30d |
| `--tool` | | 按工具名过滤 |
| `--event` | | 按事件类型过滤 |
| `--format` | text | 输出格式：text / json |
| `--project` | | 按项目路径过滤（支持相对路径） |

**text 格式输出示例**：

```
=== agent-insight Stats (Last 24h) ===

Total Events:     1,247
Sessions:         12
Total Blocked:    104
Block Rate:       8.3%

Avg Hook Latency:  1.2ms
P50 Hook Latency: 0.8ms
P95 Hook Latency: 3.1ms
P99 Hook Latency: 4.8ms

Tools Used:
  Bash         489 (32 blocked)
  Write        312 (45 blocked)
  Edit         198 (27 blocked)
  Read         148

Event Types:
  PreToolUse         623
  PostToolUse        614
  SessionStart       10

Top Blocked:
  Write        45
  Bash         32
  Edit         27

Tool Duration:
  Bash         avg=3200.0ms, p99=12100.0ms
  Write        avg=800.0ms, p99=2100.0ms
  Edit         avg=1100.0ms, p99=3100.0ms
```

### `agent-insight config`

查看和修改配置。

```bash
# 查看配置文件路径
agent-insight config path

# 列出所有配置
agent-insight config list

# 获取单个配置值
agent-insight config get storage.path

# 设置配置值
agent-insight config set dashboard.port 9090
agent-insight config set logging.level debug
```

**配置键使用点分路径**：
- `storage.type` / `storage.path` / `storage.retention_days` / `storage.max_input_size` / `storage.max_output_size`
- `collector.timeout_ms` / `collector.batch_size` / `collector.async_write`
- `dashboard.host` / `dashboard.port`
- `logging.level` / `logging.path`
- `alerts.enabled`

### `agent-insight version`

显示版本信息。

```bash
agent-insight version
# 输出: agent-insight v0.1.0 (go1.22.0, darwin/arm64, commit: abc1234)
```

## 配置详解

### 配置文件位置

```
~/.agent-insight/config.yaml
```

首次运行时自动创建。可通过环境变量和 CLI flag 覆盖。

### 配置项说明

```yaml
storage:
  type: sqlite                    # 存储类型，目前仅支持 sqlite
  path: ""                        # 数据库路径，空=全局 ~/.agent-insight/insight.db
  retention_days: 30              # 数据保留天数
  max_input_size: 10240           # tool_input 截断阈值（字节）
  max_output_size: 10240          # tool_output 截断阈值（字节）

collector:
  timeout_ms: 5000                # 采集超时（毫秒）
  batch_size: 1                   # 写入批量大小
  async_write: true               # 异步写入

dashboard:
  host: "127.0.0.1"              # Web 仪表板监听地址
  port: 8080                     # Web 仪表板监听端口
  refresh_interval_ms: 1000       # 数据刷新间隔

stats:
  aggregation_interval: 5m       # 统计聚合刷新间隔

alerts:
  enabled: false                 # 是否启用告警
  rules: []                      # 告警规则列表
  channels: []                   # 通知渠道列表

export:
  default_format: json           # 默认导出格式

logging:
  level: warn                    # 日志级别: debug / info / warn / error
  path: ""                       # 日志文件路径，空=输出到 stderr
```

### 环境变量覆盖

环境变量使用 `AGENT_INSIGHT_` 前缀 + 大写下划线路径：

| 环境变量 | 对应配置键 |
|---------|----------|
| `AGENT_INSIGHT_DB_PATH` | storage.path |
| `AGENT_INSIGHT_RETENTION_DAYS` | storage.retention_days |
| `AGENT_INSIGHT_DASHBOARD_PORT` | dashboard.port |
| `AGENT_INSIGHT_DASHBOARD_HOST` | dashboard.host |
| `AGENT_INSIGHT_ALERTS_ENABLED` | alerts.enabled |
| `AGENT_INSIGHT_LOG_LEVEL` | logging.level |

**优先级**：CLI flag > 环境变量 > 配置文件 > 默认值

### 数据存储位置

所有项目数据集中存储在全局数据库：

```
~/.agent-insight/insight.db
```

每条记录的 `cwd` 字段标识所属项目。查询命令（stats/sessions）在任意目录执行都能看到全局数据，可通过 `--project <路径>` 按项目过滤（支持相对路径，精确匹配 cwd 字段）。可通过 `storage.path` 或 `AGENT_INSIGHT_DB_PATH` 自定义路径。目录权限 0700，数据库文件权限 0600。

## 工作原理

### 事件采集流程

```
Claude Code 触发 hook
    |
    | stdin JSON
    v
agent-insight collect --event <type>
    |
    | 1. 读取 stdin JSON (<0.5ms)
    | 2. 解析 HookInput (<0.1ms)
    | 3. 补充元数据: UUID/PID/hostname/timestamp (<0.2ms)
    | 4. 截断超长输入至 10KB (<0.1ms)
    | 5. 同步写入 SQLite WAL (<2ms)
    | 6. exit 0
    v
Claude Code 继续（不受影响）
```

**关键设计**：采集始终 exit 0，任何内部失败都不阻断 Claude Code 正常运行。

### Pre/Post 配对机制

agent-insight 自动将 PreToolUse 和 PostToolUse 事件配对为一次完整的工具调用（Span）：

1. PreToolUse 到达时，创建 pending span 存入队列
2. PostToolUse 到达时，查找同 session 内最早的同名 tool pending span
3. 配对成功：`Span.duration = Post.created_at - Pre.created_at`
4. 30s 内未收到 PostToolUse：标记为 orphan（可能被拦截或超时）
5. PreToolUse exit code=2：标记为 blocked，不再等待 Post

### 拦截识别

当 PreToolUse hook 返回 exit code=2 时，Claude Code 会取消当前工具调用。agent-insight 识别这类事件：

- `blocked=true` + exit code=2
- stderr 内容记录为 `block_reason`
- 不会收到对应的 PostToolUse（标记为 orphan）

## 数据安全

| 安全措施 | 说明 |
|---------|------|
| 数据本地化 | 所有数据仅存储在本地 SQLite 文件中 |
| 无网络外传 | 默认不发起任何外部 HTTP 请求 |
| 输入截断 | tool_input/tool_output 超过 10KB 自动截断 |
| 文件权限 | 数据库文件权限 0600，目录权限 0700 |
| Webhook 白名单 | 告警 webhook 需要用户显式配置 URL |

**注意**：tool_input 可能包含代码片段、文件内容或命令行参数。如果你不希望某些数据被采集，可以在 Claude Code 的 hook matcher 中排除特定工具：

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash|Write|Edit",
        "hooks": [
          { "type": "command", "command": "agent-insight collect --event PreToolUse" }
        ]
      }
    ]
  }
}
```

## 常见问题

### Q: agent-insight 会拖慢 Claude Code 吗？

不会。采集路径 P99 延迟 < 5ms，对用户不可感知。即使采集写入失败，也会 exit 0 不阻断 Claude Code。

### Q: 数据库文件会无限增长吗？

不会。默认保留 30 天数据，超期自动清理。可通过 `storage.retention_days` 调整。

### Q: 多个项目怎么查看数据？

所有项目数据集中存储在 `~/.agent-insight/insight.db`，查询命令在任意目录执行都能看到全局数据。可通过 `--project <路径>` 过滤特定项目，例如：

```bash
agent-insight stats --project /Users/x/project-a
agent-insight sessions --project .
```

### Q: 如何清除所有采集数据？

```bash
rm -rf ~/.agent-insight/
```

此命令将删除全局数据库和配置文件。

### Q: 如何调试采集问题？

```bash
# 开启 debug 日志
agent-insight config set logging.level debug

# 查看 agent-insight 的 stderr 输出
# Claude Code 会将 hook 的 stderr 输出显示在终端
```

### Q: init 后 Claude Code 没有触发 hook？

1. 确认配置文件路径正确：`agent-insight config path`
2. 重启 Claude Code（hook 配置在启动时加载）
3. 检查配置文件中是否有其他 hook 配置冲突
4. 确认 `agent-insight` 在 PATH 中可用

## 路线图

| 里程碑 | 功能 | 状态 |
|--------|------|------|
| M1 | 事件采集、调用链追踪、CLI 查询 | 已完成 |
| M2 | 统计聚合引擎、Web 实时仪表板、会话聚合 | 规划中 |
| M3 | 告警通知、数据导出、多项目隔离、基准测试 | 规划中 |
