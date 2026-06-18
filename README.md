# agent-insight

Claude Code Hooks 可观测性平台 — 调用链追踪、统计分析、实时仪表板。

## 功能

- **Hook 事件采集**：注册为 Claude Code hook handler，自动采集所有事件
- **调用链追踪**：Pre/Post 自动配对，拦截识别，orphan 检测
- **统计分析**：工具使用分布、拦截率、P99 延迟
- **实时仪表板**：WebSocket 推送，瀑布图，事件流（M2）
- **告警通知**：超时/异常/拦截率飙升告警（M2）

## 快速开始

```bash
# 安装
go install github.com/libin18/agent-insight/cmd/agent-insight@latest

# 初始化（注册 hook 到 Claude Code）
agent-insight init

# 使用 Claude Code — 事件自动采集

# 查看统计
agent-insight stats

# 查看调用链
agent-insight trace <session_id>

# 列出会话
agent-insight sessions
```

## 配置

配置文件位于 `~/.agent-insight/config.yaml`，支持环境变量覆盖（`AGENT_INSIGHT_` 前缀）。

详见 [configs/config.example.yaml](configs/config.example.yaml)。

## 构建

```bash
make build
make test
make lint
```

## 许可证

MIT
