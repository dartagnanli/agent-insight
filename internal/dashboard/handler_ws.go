package dashboard

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// HandleWebSocket 处理 WebSocket 连接升级
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 校验 Origin
	origin := r.Header.Get("Origin")
	if origin != "" && !isAllowedOrigin(origin, s.cfg.Host) {
		slog.Warn("websocket origin rejected", "origin", origin)
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 本地开发允许非加密连接
	})
	if err != nil {
		slog.Warn("websocket accept failed", "error", err)
		return
	}

	client := &Client{
		conn:   conn,
		send:   make(chan *WSMessage, 64),
		filters: &SubscribeFilters{},
	}
	s.hub.Register(client)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// 写 goroutine：消费 client.send 并写入 WebSocket
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-client.send:
				if !ok {
					// channel 已关闭
					conn.Close(websocket.StatusNormalClosure, "")
					return
				}
				if err := wsjson.Write(ctx, conn, msg); err != nil {
					slog.Debug("websocket write error", "error", err)
					return
				}
			}
		}
	}()

	// 心跳：30s 发 ping，10s 无 pong 则关闭
	go func() {
		defer cancel()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		pongDeadline := time.Now().Add(40 * time.Second)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Now().After(pongDeadline) {
					conn.Close(websocket.StatusPolicyViolation, "pong timeout")
					return
				}
				if err := conn.Ping(ctx); err != nil {
					return
				}
				pongDeadline = time.Now().Add(10 * time.Second)
			}
		}
	}()

	// 读 goroutine：读客户端消息（subscribe/pong）
	for {
		var msg struct {
			Type       string `json:"type"`
			SessionID  string `json:"session_id,omitempty"`
			EventType  string `json:"event_type,omitempty"`
			ToolName   string `json:"tool_name,omitempty"`
		}
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			break
		}
		switch msg.Type {
		case "subscribe":
			client.filters = &SubscribeFilters{
				SessionID: msg.SessionID,
				EventType: msg.EventType,
				ToolName:  msg.ToolName,
			}
		case "pong":
			// pong 已由底层处理，这里仅刷新
		}
	}

	s.hub.Unregister(client)
	conn.Close(websocket.StatusNormalClosure, "")
}

// isAllowedOrigin 检查 Origin 是否允许
func isAllowedOrigin(origin, host string) bool {
	// 本地开发允许 localhost 和 127.0.0.1
	allowed := []string{
		"http://localhost",
		"http://127.0.0.1",
	}
	for _, a := range allowed {
		if strings.HasPrefix(origin, a) {
			return true
		}
	}
	// 同源也允许
	return origin == "" || strings.Contains(origin, host)
}
