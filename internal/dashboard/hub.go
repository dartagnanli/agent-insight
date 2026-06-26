package dashboard

import (
	"sync"

	"github.com/coder/websocket"
)

// Hub 管理所有 WebSocket 客户端的广播
type Hub struct {
	mu      sync.Mutex
	clients map[int64]*Client
	nextID  int64
}

// Client 表示一个 WebSocket 连接
type Client struct {
	id     int64
	conn   *websocket.Conn
	send   chan *WSMessage
	filters *SubscribeFilters
}

// SubscribeFilters 客户端订阅的过滤条件
type SubscribeFilters struct {
	SessionID string
	EventType string
	ToolName  string
}

// WSMessage WebSocket 消息格式
type WSMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// NewHub 创建 Hub 实例
func NewHub() *Hub {
	return &Hub{
		clients: make(map[int64]*Client),
	}
}

// Register 注册新客户端
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c.id = h.nextID
	h.nextID++
	h.clients[c.id] = c
}

// Unregister 移除客户端并关闭连接
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c.id]; ok {
		delete(h.clients, c.id)
		close(c.send)
	}
}

// Broadcast 向所有匹配的客户端广播消息
func (h *Hub) Broadcast(msg *WSMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.clients {
		if !matchFilters(msg, c.filters) {
			continue
		}
		select {
		case c.send <- msg:
		default:
			// channel 满时关闭客户端
			delete(h.clients, c.id)
			close(c.send)
		}
	}
}

// CloseAll 关闭所有客户端连接
func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.clients {
		close(c.send)
	}
	h.clients = make(map[int64]*Client)
}

// ClientCount 返回当前客户端数量
func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// matchFilters 检查消息是否匹配客户端过滤条件
func matchFilters(msg *WSMessage, filters *SubscribeFilters) bool {
	if filters == nil {
		return true
	}
	// 只对事件消息做过滤
	if msg.Type != "event" {
		return true
	}
	payload, ok := msg.Payload.(map[string]any)
	if !ok {
		return true
	}
	if filters.SessionID != "" {
		if sid, _ := payload["session_id"].(string); sid != filters.SessionID {
			return false
		}
	}
	if filters.EventType != "" {
		if et, _ := payload["event_type"].(string); et != filters.EventType {
			return false
		}
	}
	if filters.ToolName != "" {
		if tn, _ := payload["tool_name"].(string); tn != filters.ToolName {
			return false
		}
	}
	return true
}
