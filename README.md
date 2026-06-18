# agent-insight

Claude Code Hooks 可观测性平台 — 调用链追踪、统计分析、实时仪表板。

## 功能

- **Hook 事件采集**：注册为 Claude Code hook handler，自动采集所有工具调用事件
- **调用链追踪**：Pre/Post 自动配对，拦截识别，orphan 检测
- **统计分析**：工具使用分布、拦截率、P50/P95/P99 延迟
- **实时仪表板**：WebSocket 推送，瀑布图，事件流（M2）
- **告警通知**：超时/异常/拦截率飙升告警（M3）

## 安装

```bash
# 从源码构建
git clone ssh://ezone.ksyun.com:23/ezone/libin18/agent-insight.git
cd agent-insight && make install

# 或使用 go install
go install github.com/libin18/agent-insight/cmd/agent-insight@latest
```

## 快速开始

```bash
# 1. 初始化（注册 hook 到 Claude Code）
agent-insight init

# 2. 正常使用 Claude Code — 事件自动采集

# 3. 查看数据
agent-insight stats
agent-insight stats --project .
agent-insight sessions
agent-insight sessions --project .
agent-insight trace <session_id>
```

## 命令一览

| 命令 | 说明 |
|------|------|
| `agent-insight init` | 初始化 hook 配置 |
| `agent-insight collect --event <type>` | 采集事件（自动调用） |
| `agent-insight stats` | 统计摘要（支持 `--project`） |
| `agent-insight sessions` | 列出会话（支持 `--project`） |
| `agent-insight trace <id>` | 查看调用链 |
| `agent-insight config` | 管理配置 |
| `agent-insight version` | 版本信息 |

## 文档

- [使用文档](docs/usage.md) — 完整的命令参考、配置说明、工作原理、常见问题
- [PRD](docs/prd.md) — 产品需求文档
- [架构设计](docs/architecture.md) — 技术选型、接口契约、数据模型

## 构建

```bash
make build    # 编译
make test     # 运行测试
make lint     # 静态检查
make dist     # 多平台发布包
```

## 许可证

MIT
