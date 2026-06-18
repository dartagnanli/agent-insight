package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/libin18/agent-insight/internal/collector"
	"github.com/libin18/agent-insight/internal/config"
	"github.com/libin18/agent-insight/internal/session"
	"github.com/libin18/agent-insight/internal/stats"
	"github.com/libin18/agent-insight/internal/storage"
	"github.com/libin18/agent-insight/internal/trace"
	"github.com/libin18/agent-insight/pkg/event"
	"github.com/spf13/cobra"
)

var (
	buildVersion = config.Version
	buildCommit  = "unknown"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Warn("failed to load config, using defaults", "error", err)
		cfg = config.DefaultConfig()
	}
	initLogger(cfg)

	rootCmd := &cobra.Command{
		Use:   "agent-insight",
		Short: "Claude Code Hooks 可观测性平台",
		Long:  "agent-insight 为 Claude Code hooks 生态提供生产级可观测性能力",
	}

	// --- collect ---
	collectCmd := &cobra.Command{
		Use:   "collect --event <event_type>",
		Short: "采集 hook 事件（被 Claude Code 调用）",
		Run: func(cmd *cobra.Command, args []string) {
			eventType, _ := cmd.Flags().GetString("event")
			if eventType == "" {
				slog.Warn("--event flag is required")
				os.Exit(0)
			}
			runCollect(cfg, eventType)
		},
	}
	collectCmd.Flags().String("event", "", "hook 事件类型")
	rootCmd.AddCommand(collectCmd)

	// --- init ---
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "初始化配置，生成 settings.json hook 注册",
		Run: func(cmd *cobra.Command, args []string) {
			global, _ := cmd.Flags().GetBool("global")
			force, _ := cmd.Flags().GetBool("force")
			runInit(global, force)
		},
	}
	initCmd.Flags().Bool("global", false, "生成用户级 ~/.claude/settings.json 配置")
	initCmd.Flags().Bool("force", false, "覆盖已存在的配置")
	rootCmd.AddCommand(initCmd)

	// --- trace ---
	traceCmd := &cobra.Command{
		Use:   "trace <session_id>",
		Short: "查看指定 session 的调用链",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			format, _ := cmd.Flags().GetString("format")
			runTrace(cfg, args[0], format)
		},
	}
	traceCmd.Flags().String("format", "text", "输出格式: text|json")
	rootCmd.AddCommand(traceCmd)

	// --- sessions ---
	sessionsCmd := &cobra.Command{
		Use:   "sessions",
		Short: "列出所有已知 session",
		Run: func(cmd *cobra.Command, args []string) {
			sortField, _ := cmd.Flags().GetString("sort")
			order, _ := cmd.Flags().GetString("order")
			since, _ := cmd.Flags().GetString("since")
			limit, _ := cmd.Flags().GetInt("limit")
			detail, _ := cmd.Flags().GetString("detail")
			project, _ := cmd.Flags().GetString("project")
			runSessions(cfg, sortField, order, since, limit, detail, project)
		},
	}
	sessionsCmd.Flags().String("sort", "started_at", "排序字段: started_at|events|duration|blocked")
	sessionsCmd.Flags().String("order", "desc", "排序方向: asc|desc")
	sessionsCmd.Flags().String("since", "24h", "时间过滤: 1h|6h|24h|7d|30d")
	sessionsCmd.Flags().Int("limit", 50, "最大条数")
	sessionsCmd.Flags().String("detail", "", "显示特定 session 的详细聚合信息")
	sessionsCmd.Flags().String("project", "", "按项目路径过滤（支持相对路径）")
	rootCmd.AddCommand(sessionsCmd)

	// --- stats ---
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "显示统计摘要",
		Run: func(cmd *cobra.Command, args []string) {
			since, _ := cmd.Flags().GetString("since")
			tool, _ := cmd.Flags().GetString("tool")
			evt, _ := cmd.Flags().GetString("event")
			format, _ := cmd.Flags().GetString("format")
			project, _ := cmd.Flags().GetString("project")
			runStats(cfg, since, tool, evt, format, project)
		},
	}
	statsCmd.Flags().String("since", "24h", "时间范围: 1h|6h|24h|7d|30d")
	statsCmd.Flags().String("tool", "", "按工具过滤")
	statsCmd.Flags().String("event", "", "按事件类型过滤")
	statsCmd.Flags().String("format", "text", "输出格式: text|json")
	statsCmd.Flags().String("project", "", "按项目路径过滤（支持相对路径）")
	rootCmd.AddCommand(statsCmd)

	// --- config ---
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "查看和修改配置",
	}
	configGetCmd := &cobra.Command{
		Use:  "get <key>",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(getConfigValue(cfg, args[0]))
		},
	}
	configSetCmd := &cobra.Command{
		Use:  "set <key> <value>",
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := setConfigValue(args[0], args[1]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("OK")
		},
	}
	configListCmd := &cobra.Command{
		Use: "list",
		Run: func(cmd *cobra.Command, args []string) {
			data, _ := json.MarshalIndent(cfg, "", "  ")
			fmt.Println(string(data))
		},
	}
	configPathCmd := &cobra.Command{
		Use: "path",
		Run: func(cmd *cobra.Command, args []string) {
			p, _ := config.ConfigPath()
			if p == "" {
				p = "(not found)"
			}
			fmt.Println(p)
		},
	}
	configCmd.AddCommand(configGetCmd, configSetCmd, configListCmd, configPathCmd)
	rootCmd.AddCommand(configCmd)

	// --- version ---
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "版本信息",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("agent-insight %s (go%s, %s/%s, commit: %s)\n",
				buildVersion, runtime.Version()[2:], runtime.GOOS, runtime.GOARCH, buildCommit)
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initLogger(cfg *config.Config) {
	var level slog.Level
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelWarn
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

// ======== collect ========

func runCollect(cfg *config.Config, eventType string) {
	ctx := context.Background()
	s, err := openStorage(ctx, cfg)
	if err != nil {
		slog.Warn("open storage failed", "error", err)
		os.Exit(0)
	}
	defer s.Close()

	c := collector.NewCollector(s, eventType, cfg.Storage.MaxInputSize)
	c.Run(ctx, eventType)
}

// ======== init ========

func runInit(global bool, force bool) {
	type hookEntry struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type hookGroup struct {
		Matcher string      `json:"matcher"`
		Hooks   []hookEntry `json:"hooks"`
	}
	type settingsJSON struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}

	events := []string{"PreToolUse", "PostToolUse", "SessionStart", "Stop"}
	hooks := make(map[string][]hookGroup)
	for _, ev := range events {
		hooks[ev] = []hookGroup{
			{
				Matcher: "",
				Hooks: []hookEntry{
					{Type: "command", Command: fmt.Sprintf("agent-insight collect --event %s", ev)},
				},
			},
		}
	}

	data := settingsJSON{Hooks: hooks}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal settings: %v\n", err)
		os.Exit(1)
	}

	var path string
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get home dir: %v\n", err)
			os.Exit(1)
		}
		dir := home + "/.claude"
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create dir: %v\n", err)
			os.Exit(1)
		}
		path = dir + "/settings.json"
	} else {
		dir := ".claude"
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create dir: %v\n", err)
			os.Exit(1)
		}
		path = dir + "/settings.json"
	}

	if !force {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(os.Stderr, "file already exists: %s (use --force to overwrite)\n", path)
			os.Exit(1)
		}
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Settings written to %s\n", path)
}

// ======== trace ========

func runTrace(cfg *config.Config, sessionID string, format string) {
	ctx := context.Background()
	s, err := openStorage(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	sid := sessionID
	filter := storage.EventFilter{SessionID: &sid, Limit: 10000, SortOrder: "asc"}
	events, err := s.QueryEvents(ctx, filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(events) == 0 {
		fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionID)
		os.Exit(1)
	}

	tracer := trace.NewTracer(30)
	t, err := tracer.BuildTrace(ctx, events)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building trace: %v\n", err)
		os.Exit(1)
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(t, "", "  ")
		fmt.Println(string(data))
	default:
		printTraceText(t)
	}
}

func printTraceText(t *event.Trace) {
	dur := event.FormatDuration(t.DurationSecs)
	fmt.Printf("\nSession: %s  Duration: %s  Events: %d\n\n", t.SessionID, dur, t.TotalEvents)

	startTime, _ := time.Parse("2006-01-02T15:04:05.000Z", t.StartedAt)

	for _, se := range t.StandaloneEvents {
		evtTime, _ := time.Parse("2006-01-02T15:04:05.000Z", se.CreatedAt)
		offset := event.FormatDuration(int64(evtTime.Sub(startTime).Seconds()))
		fmt.Printf("  %5s  %s\n", offset, se.EventType)
	}

	for _, sp := range t.Spans {
		spanTime, _ := time.Parse("2006-01-02T15:04:05.000Z", sp.StartedAt)
		offset := event.FormatDuration(int64(spanTime.Sub(startTime).Seconds()))
		input := truncateDisplay(sp.ToolInput, 30)
		status := ""
		if sp.Blocked {
			status = "[BLOCKED]"
		} else if sp.Orphan {
			status = "[ORPHAN]"
		}
		durStr := fmt.Sprintf("%dms", sp.DurationMs)
		reason := ""
		if sp.BlockReason != "" {
			reason = truncateDisplay(sp.BlockReason, 40)
		}
		fmt.Printf("  %5s  %-12s %-10s %-30s %s %s\n",
			offset, sp.ToolName, status, input, durStr, reason)
	}
}

// ======== sessions ========

func runSessions(cfg *config.Config, sortField, order, since string, limit int, detail string, project string) {
	ctx := context.Background()
	s, err := openStorage(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	if detail != "" {
		runSessionDetail(ctx, s, detail)
		return
	}

	sinceDur, err := storage.ParseDuration(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid --since: %v\n", err)
		os.Exit(1)
	}
	sinceTime := time.Now().UTC().Add(-sinceDur)

	sortBy := "started_at"
	switch sortField {
	case "events":
		sortBy = "total_events"
	case "duration":
		sortBy = "duration_secs"
	case "blocked":
		sortBy = "blocked"
	}

	filter := storage.SessionFilter{
		Since:       &sinceTime,
		SortBy:      sortBy,
		SortOrder:   order,
		Limit:       limit,
	}
	if project != "" {
		abs, err := filepath.Abs(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --project path: %v\n", err)
			os.Exit(1)
		}
		filter.ProjectPath = &abs
	}

	summaries, err := s.ListSessions(ctx, filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(summaries) == 0 {
		fmt.Println("No sessions found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tStarted\tEvents\tDuration\tBlocked")
	for _, sm := range summaries {
		dur := event.FormatDuration(sm.DurationSec)
		started := sm.StartedAt.Format("2006-01-02 15:04")
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%d\n", sm.SessionID, started, sm.TotalEvents, dur, sm.Blocked)
	}
	w.Flush()
}

func runSessionDetail(ctx context.Context, s *storage.SQLite, sessionID string) {
	sid := sessionID
	evtFilter := storage.EventFilter{SessionID: &sid, Limit: 10000, SortOrder: "asc"}
	events, err := s.QueryEvents(ctx, evtFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(events) == 0 {
		fmt.Fprintf(os.Stderr, "Session not found: %s\n", sessionID)
		os.Exit(1)
	}

	snap := stats.ComputeFromEvents(events)
	ts := stats.ComputeToolStats(events)

	fmt.Printf("\nSession: %s\n", sessionID)
	fmt.Printf("  Started:       %s\n", events[0].CreatedAt)
	fmt.Printf("  Total Events:  %d\n", snap.TotalEvents)
	preToolCount := snap.EventTypeDist["PreToolUse"]
	fmt.Printf("  Tool Calls:    %d\n", preToolCount)
	fmt.Printf("  Blocked:       %d\n", snap.TotalBlocked)
	if preToolCount > 0 {
		fmt.Printf("  Block Rate:    %.1f%%\n", float64(snap.TotalBlocked)/float64(preToolCount)*100)
	}

	fmt.Println("\n  Tools Used:")
	for name, cnt := range snap.ToolDist {
		bl := snap.BlockDist[name]
		fmt.Printf("    %-12s %d calls, %d blocked\n", name, cnt, bl)
	}

	if len(ts) > 0 {
		fmt.Println("\n  Tool Duration:")
		for name, t := range ts {
			fmt.Printf("    %-12s avg=%.1fms, p99=%.1fms\n", name, t.AvgMs, t.P99Ms)
		}
	}

	agg := session.NewAggregator(s)
	if err := agg.AggregateFromEvents(ctx, events); err != nil {
		slog.Warn("failed to aggregate session stats", "error", err)
	}
}

// ======== stats ========

func runStats(cfg *config.Config, since, tool, evt, format, project string) {
	ctx := context.Background()
	s, err := openStorage(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	sinceDur, err := storage.ParseDuration(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid --since: %v\n", err)
		os.Exit(1)
	}
	sinceTime := time.Now().UTC().Add(-sinceDur)

	filter := storage.EventFilter{Since: &sinceTime, Limit: 10000}
	if tool != "" {
		filter.ToolName = &tool
	}
	if evt != "" {
		filter.EventType = &evt
	}
	if project != "" {
		abs, err := filepath.Abs(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --project path: %v\n", err)
			os.Exit(1)
		}
		filter.Cwd = &abs
	}

	events, err := s.QueryEvents(ctx, filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(events) == 0 {
		fmt.Println("No data available.")
		return
	}

	snap := stats.ComputeFromEvents(events)
	ts := stats.ComputeToolStats(events)

	switch format {
	case "json":
		type jsonOut struct {
			*stats.StatsSnapshot
			ToolDetails map[string]*stats.ToolStats `json:"tool_details"`
		}
		out := jsonOut{StatsSnapshot: snap, ToolDetails: ts}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	default:
		printStatsText(snap, ts, since)
	}
}

func printStatsText(snap *stats.StatsSnapshot, ts map[string]*stats.ToolStats, since string) {
	fmt.Printf("\n=== agent-insight Stats (Last %s) ===\n\n", since)

	fmt.Printf("Total Events:     %d\n", snap.TotalEvents)
	fmt.Printf("Sessions:         %d\n", snap.TotalSessions)
	fmt.Printf("Total Blocked:    %d\n", snap.TotalBlocked)
	fmt.Printf("Block Rate:       %.1f%%\n", snap.BlockRate*100)

	fmt.Printf("\nAvg Hook Latency:  %.1fms\n", snap.AvgHookMs)
	fmt.Printf("P50 Hook Latency: %.1fms\n", snap.P50HookMs)
	fmt.Printf("P95 Hook Latency: %.1fms\n", snap.P95HookMs)
	fmt.Printf("P99 Hook Latency: %.1fms\n", snap.P99HookMs)

	fmt.Println("\nTools Used:")
	for name, cnt := range snap.ToolDist {
		bl := snap.BlockDist[name]
		fmt.Printf("  %-12s %d", name, cnt)
		if bl > 0 {
			fmt.Printf(" (%d blocked)", bl)
		}
		fmt.Println()
	}

	fmt.Println("\nEvent Types:")
	for name, cnt := range snap.EventTypeDist {
		fmt.Printf("  %-18s %d\n", name, cnt)
	}

	if len(snap.BlockDist) > 0 {
		fmt.Println("\nTop Blocked:")
		type kv struct {
			k string
			v int
		}
		var sorted []kv
		for k, v := range snap.BlockDist {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
		for i, item := range sorted {
			fmt.Printf("  %-12s %d\n", item.k, item.v)
			if i >= 9 {
				break
			}
		}
	}

	if len(ts) > 0 {
		fmt.Println("\nTool Duration:")
		for name, t := range ts {
			fmt.Printf("  %-12s avg=%.1fms, p99=%.1fms\n", name, t.AvgMs, t.P99Ms)
		}
	}
}

// ======== config ========

func getConfigValue(cfg *config.Config, key string) string {
	parts := strings.Split(key, ".")
	if len(parts) < 2 {
		return ""
	}
	switch parts[0] {
	case "storage":
		switch parts[1] {
		case "type":
			return cfg.Storage.Type
		case "path":
			p, _ := config.DBPath(cfg)
			return p
		case "retention_days":
			return strconv.Itoa(cfg.Storage.RetentionDays)
		case "max_input_size":
			return strconv.Itoa(cfg.Storage.MaxInputSize)
		case "max_output_size":
			return strconv.Itoa(cfg.Storage.MaxOutputSize)
		}
	case "collector":
		switch parts[1] {
		case "timeout_ms":
			return strconv.Itoa(cfg.Collector.TimeoutMs)
		case "batch_size":
			return strconv.Itoa(cfg.Collector.BatchSize)
		case "async_write":
			return strconv.FormatBool(cfg.Collector.AsyncWrite)
		}
	case "dashboard":
		switch parts[1] {
		case "host":
			return cfg.Dashboard.Host
		case "port":
			return strconv.Itoa(cfg.Dashboard.Port)
		}
	case "logging":
		switch parts[1] {
		case "level":
			return cfg.Logging.Level
		case "path":
			return cfg.Logging.Path
		}
	case "alerts":
		if parts[1] == "enabled" {
			return strconv.FormatBool(cfg.Alerts.Enabled)
		}
	}
	return ""
}

func setConfigValue(key, value string) error {
	configDir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	configPath, _ := config.ConfigPath()
	v := config.NewViper()
	_ = v.ReadInConfig()
	v.Set(key, value)
	return v.WriteConfigAs(configPath)
}

// ======== 辅助函数 ========

func openStorage(ctx context.Context, cfg *config.Config) (*storage.SQLite, error) {
	dbPath, err := config.DBPath(cfg)
	if err != nil {
		return nil, err
	}
	return storage.NewSQLite(ctx, config.StorageConfig{
		Type:          "sqlite",
		Path:          dbPath,
		MaxInputSize:  cfg.Storage.MaxInputSize,
		MaxOutputSize: cfg.Storage.MaxOutputSize,
	})
}

func truncateDisplay(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
