package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/dartagnanli/agent-insight/internal/config"
	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/require"
)

// newWSTestDB 创建内存 SQLite 数据库用于 WebSocket 测试
func newWSTestDB(t *testing.T) storage.Storage {
	t.Helper()
	dir := t.TempDir()
	dbPath := dir + "/insight.db"
	cfg := config.StorageConfig{Type: "sqlite", Path: dbPath}
	s, err := storage.NewSQLite(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// startTestWSServer 创建一个 httptest.Server 提供 WebSocket 处理
// 由于 Server 依赖 embed 导致无法直接构建，这里直接构建路由
func startTestWSServer(t *testing.T, srv *Server) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ws", srv.HandleWebSocket)
	httpSrv := httptest.NewServer(mux)
	t.Cleanup(func() { httpSrv.Close() })
	return httpSrv
}

// newTestDashboardServer 创建用于 WebSocket 测试的 Server 实例
// 注意：此函数依赖 NewServer，而 NewServer 使用了 embed 指令
// 如果 web/assets 不存在会导致编译错误，Dev 实现后此函数可用
func newTestDashboardServer(t *testing.T) *Server {
	t.Helper()
	db := newWSTestDB(t)
	cfg := config.DashboardConfig{Host: "127.0.0.1", Port: 0}
	return NewServer(cfg, db)
}

// --- TestWebSocketConnect_成功连接 ---
func TestWebSocketConnect_成功连接(t *testing.T) {
	db := newWSTestDB(t)
	hub := NewHub()
	srv := &Server{storage: db, hub: hub, cfg: config.DashboardConfig{Host: "127.0.0.1", Port: 8080}}

	httpSrv := startTestWSServer(t, srv)
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/ws"

	conn, _, err := websocket.Dial(t.Context(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "test done")
}

// --- TestWebSocketEventPush_事件推送 ---
func TestWebSocketEventPush_事件推送(t *testing.T) {
	db := newWSTestDB(t)
	hub := NewHub()
	srv := &Server{storage: db, hub: hub, cfg: config.DashboardConfig{Host: "127.0.0.1", Port: 8080}}

	httpSrv := startTestWSServer(t, srv)
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/ws"

	conn, _, err := websocket.Dial(t.Context(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	// 等待注册完成
	time.Sleep(100 * time.Millisecond)

	// 广播一条事件
	hub.Broadcast(&WSMessage{
		Type: "event",
		Payload: map[string]any{
			"event_id":   "evt-ws-push-1",
			"session_id": "sess-ws-push",
			"event_type": "PreToolUse",
			"tool_name":  "Bash",
			"created_at": event.Now(),
		},
	})

	// 客户端应该收到推送的消息
	ctxTimeout, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	_, msg, err := conn.Read(ctxTimeout)
	require.NoError(t, err)
	t.Logf("received message: %s", string(msg))
}

// --- TestWebSocketSubscribe_过滤订阅 ---
func TestWebSocketSubscribe_过滤订阅(t *testing.T) {
	db := newWSTestDB(t)
	hub := NewHub()
	srv := &Server{storage: db, hub: hub, cfg: config.DashboardConfig{Host: "127.0.0.1", Port: 8080}}

	httpSrv := startTestWSServer(t, srv)
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/ws"

	conn, _, err := websocket.Dial(t.Context(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	// 等待注册完成
	time.Sleep(100 * time.Millisecond)

	// 注册一个订阅 Bash 的客户端
	bashClient := &Client{
		send:    make(chan *WSMessage, 64),
		filters: &SubscribeFilters{ToolName: "Bash"},
	}
	hub.Register(bashClient)

	// 广播 Bash 事件
	hub.Broadcast(&WSMessage{
		Type: "event",
		Payload: map[string]any{
			"event_id":   "evt-sub-bash",
			"session_id": "sess-sub",
			"event_type": "PreToolUse",
			"tool_name":  "Bash",
			"created_at": event.Now(),
		},
	})

	// bashClient 应该收到
	select {
	case msg := <-bashClient.send:
		payload, _ := msg.Payload.(map[string]any)
		require.Equal(t, "Bash", payload["tool_name"])
	case <-time.After(2 * time.Second):
		t.Fatal("bashClient 应该收到 Bash 事件")
	}
}

// --- TestWebSocketOriginCheck_非localhost拒绝 ---
func TestWebSocketOriginCheck_非localhost拒绝(t *testing.T) {
	db := newWSTestDB(t)
	hub := NewHub()
	srv := &Server{storage: db, hub: hub, cfg: config.DashboardConfig{Host: "127.0.0.1", Port: 8080}}

	httpSrv := startTestWSServer(t, srv)
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/ws"

	_, _, err := websocket.Dial(t.Context(), wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Origin": {"http://evil.example.com"},
		},
	})

	// 非 localhost 来源的连接应被拒绝
	require.Error(t, err)
}
