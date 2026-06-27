package dashboard

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dartagnanli/agent-insight/internal/session"
	"github.com/dartagnanli/agent-insight/internal/stats"
	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/internal/trace"
)

// APIHandler 提供 REST API 端点
type APIHandler struct {
	storage storage.Storage
	tracer  *trace.Tracer
}

// NewAPIHandler 创建 API 处理器
func NewAPIHandler(s storage.Storage) *APIHandler {
	return &APIHandler{
		storage: s,
		tracer:  trace.NewTracer(30),
	}
}

// HandleListEvents GET /api/v1/events
func (h *APIHandler) HandleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := parseEventFilter(q)

	events, err := h.storage.QueryEvents(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := h.storage.CountEvents(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// HandleGetEvent GET /api/v1/events/{eventID}
func (h *APIHandler) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "event_id is required")
		return
	}

	ev, err := h.storage.GetEvent(r.Context(), eventID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ev == nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// HandleStats GET /api/v1/stats
func (h *APIHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := parseEventFilter(q)
	// stats 需要全量事件，不受分页 limit 限制
	f.Limit = 10000

	events, err := h.storage.QueryEvents(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	snap := stats.ComputeFromEvents(events)
	ts := stats.ComputeToolStats(events)

	// 查询小时级统计用于趋势
	sf := parseStatsFilter(q)
	buckets, err := h.storage.QueryStatsHourly(r.Context(), sf)
	if err != nil {
		slog.Warn("failed to query hourly stats", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total_events":            snap.TotalEvents,
		"total_sessions":          snap.TotalSessions,
		"total_blocked":           snap.TotalBlocked,
		"block_rate":              snap.BlockRate,
		"avg_hook_duration_ms":    snap.AvgHookMs,
		"p50_hook_duration_ms":   snap.P50HookMs,
		"p95_hook_duration_ms":   snap.P95HookMs,
		"p99_hook_duration_ms":   snap.P99HookMs,
		"tool_distribution":      snap.ToolDist,
		"event_type_distribution": snap.EventTypeDist,
		"tool_details":            ts,
		"hourly_trend":            buckets,
	})
}

// HandleStatsHourly GET /api/v1/stats/hourly
func (h *APIHandler) HandleStatsHourly(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := parseStatsFilter(q)

	buckets, err := h.storage.QueryStatsHourly(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"buckets": buckets,
	})
}

// HandleTrace GET /api/v1/traces/{sessionID}
func (h *APIHandler) HandleTrace(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	sid := sessionID
	f := storage.EventFilter{SessionID: &sid, Limit: 10000, SortOrder: "asc"}
	events, err := h.storage.QueryEvents(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(events) == 0 {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	t, err := h.tracer.BuildTrace(r.Context(), events)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, t)
}

// HandleListSessions GET /api/v1/sessions
func (h *APIHandler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := parseSessionFilter(q)

	sessions, err := h.storage.ListSessions(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := h.storage.CountSessions(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// session_stats 为空时从 hook_events 实时聚合
	if len(sessions) == 0 {
		sessionIDs, err := h.storage.DistinctSessions(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agg := session.NewAggregator(h.storage)
		for _, sid := range sessionIDs {
			sid := sid
			evts, err := h.storage.QueryEvents(r.Context(), storage.EventFilter{SessionID: &sid, Limit: 10000, SortOrder: "asc"})
			if err != nil || len(evts) == 0 {
				continue
			}
			_ = agg.AggregateFromEvents(r.Context(), evts)
		}
		sessions, err = h.storage.ListSessions(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total, err = h.storage.CountSessions(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"total":    total,
		"limit":    f.Limit,
		"offset":   f.Offset,
	})
}

// parseEventFilter 从 query 参数构建 EventFilter
func parseEventFilter(q url.Values) storage.EventFilter {
	f := storage.EventFilter{}
	if v := q.Get("session_id"); v != "" {
		f.SessionID = &v
	}
	if v := q.Get("event_type"); v != "" {
		f.EventType = &v
	}
	if v := q.Get("tool_name"); v != "" {
		f.ToolName = &v
	}
	if v := q.Get("blocked"); v != "" {
		b := v == "true"
		f.Blocked = &b
	}
	if v := q.Get("cwd"); v != "" {
		f.Cwd = &v
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = &t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = &t
		}
	}
	f.Limit = 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 10000 {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			f.Offset = n
		}
	}
	if v := q.Get("sort_by"); v != "" {
		f.SortBy = v
	}
	if v := q.Get("sort_order"); v != "" {
		f.SortOrder = v
	}
	return f
}

// parseStatsFilter 从 query 参数构建 StatsFilter
func parseStatsFilter(q url.Values) storage.StatsFilter {
	f := storage.StatsFilter{}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = &t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = &t
		}
	}
	if v := q.Get("event_type"); v != "" {
		f.EventType = &v
	}
	if v := q.Get("tool_name"); v != "" {
		f.ToolName = &v
	}
	return f
}

// parseSessionFilter 从 query 参数构建 SessionFilter
func parseSessionFilter(q url.Values) storage.SessionFilter {
	f := storage.SessionFilter{}
	if v := q.Get("project_path"); v != "" {
		f.ProjectPath = &v
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = &t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = &t
		}
	}
	f.Limit = 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			f.Offset = n
		}
	}
	if v := q.Get("sort_by"); v != "" {
		f.SortBy = v
	}
	if v := q.Get("sort_order"); v != "" {
		f.SortOrder = v
	}
	return f
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// HandleVersion GET /api/v1/version
func (h *APIHandler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "agent-insight",
		"version": Version,
	})
}
