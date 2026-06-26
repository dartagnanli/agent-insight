package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dartagnanli/agent-insight/pkg/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestHubRegisterUnregister_注册和注销客户端 ---
func TestHubRegisterUnregister_注册和注销客户端(t *testing.T) {
	hub := NewHub()

	client := &Client{send: make(chan *WSMessage, 64)}
	hub.Register(client)

	hub.mu.Lock()
	count := len(hub.clients)
	hub.mu.Unlock()
	assert.Equal(t, 1, count)

	hub.Unregister(client)

	hub.mu.Lock()
	count = len(hub.clients)
	hub.mu.Unlock()
	assert.Equal(t, 0, count)
}

// --- TestHubBroadcast_广播消息到所有客户端 ---
func TestHubBroadcast_广播消息到所有客户端(t *testing.T) {
	hub := NewHub()

	client1 := &Client{send: make(chan *WSMessage, 64), filters: nil}
	client2 := &Client{send: make(chan *WSMessage, 64), filters: nil}

	hub.Register(client1)
	hub.Register(client2)

	msg := &WSMessage{
		Type: "event",
		Payload: map[string]any{
			"event_id":   "evt-bcast",
			"session_id": "sess-bcast",
			"event_type": "PreToolUse",
			"tool_name":  "Bash",
			"created_at": event.Now(),
		},
	}
	hub.Broadcast(msg)

	msg1 := <-client1.send
	msg2 := <-client2.send

	assert.Equal(t, "event", msg1.Type)
	assert.Equal(t, "event", msg2.Type)
}

// --- TestHubBroadcastFiltered_只广播到匹配filters的客户端 ---
func TestHubBroadcastFiltered_只广播到匹配filters的客户端(t *testing.T) {
	hub := NewHub()

	// client1 订阅 tool_name=Bash
	client1 := &Client{
		send:    make(chan *WSMessage, 64),
		filters: &SubscribeFilters{ToolName: "Bash"},
	}
	// client2 无过滤
	client2 := &Client{
		send:    make(chan *WSMessage, 64),
		filters: nil,
	}

	hub.Register(client1)
	hub.Register(client2)

	// 广播一条 Write 事件
	writeMsg := &WSMessage{
		Type: "event",
		Payload: map[string]any{
			"event_id":   "evt-write-1",
			"session_id": "sess-filter",
			"event_type": "PreToolUse",
			"tool_name":  "Write",
			"created_at": event.Now(),
		},
	}
	hub.Broadcast(writeMsg)

	// client1 (订阅 Bash) 不应该收到 Write 事件
	select {
	case <-client1.send:
		t.Fatal("client1 不应该收到 Write 事件")
	default:
		// 预期行为：没有消息
	}

	// client2 (无过滤) 应该收到
	select {
	case msg := <-client2.send:
		payload, _ := msg.Payload.(map[string]any)
		assert.Equal(t, "Write", payload["tool_name"])
	default:
		t.Fatal("client2 应该收到消息")
	}
}

// --- TestHubSlowClient_发送满时关闭客户端 ---
func TestHubSlowClient_发送满时关闭客户端(t *testing.T) {
	hub := NewHub()

	client := &Client{
		send: make(chan *WSMessage, 1),
	}
	hub.Register(client)

	// 填满 send channel
	client.send <- &WSMessage{Type: "filler"}

	// 再广播多条消息，send channel 已满应触发客户端关闭
	for i := 0; i < 3; i++ {
		hub.Broadcast(&WSMessage{
			Type: "event",
			Payload: map[string]any{
				"event_id":   "evt-slow",
				"session_id": "sess-slow",
				"event_type": "PreToolUse",
				"tool_name":  "Bash",
				"created_at": event.Now(),
			},
		})
	}

	// 等待 hub 处理
	time.Sleep(200 * time.Millisecond)

	hub.mu.Lock()
	_, exists := hub.clients[client.id]
	hub.mu.Unlock()
	assert.False(t, exists, "慢客户端应该被从 hub 移除")
}

// --- TestHubCloseAll_关闭所有客户端 ---
func TestHubCloseAll_关闭所有客户端(t *testing.T) {
	hub := NewHub()

	client1 := &Client{send: make(chan *WSMessage, 64)}
	client2 := &Client{send: make(chan *WSMessage, 64)}

	hub.Register(client1)
	hub.Register(client2)

	hub.CloseAll()

	hub.mu.Lock()
	count := len(hub.clients)
	hub.mu.Unlock()
	assert.Equal(t, 0, count)
}

// --- TestMatchFilters_过滤逻辑 ---
func TestMatchFilters_过滤逻辑(t *testing.T) {
	tests := []struct {
		name    string
		msg     *WSMessage
		filters *SubscribeFilters
		want    bool
	}{
		{
			name: "nil过滤器匹配所有",
			msg: &WSMessage{Type: "event", Payload: map[string]any{"session_id": "s1"}},
			filters: nil,
			want: true,
		},
		{
			name: "session_id匹配",
			msg: &WSMessage{Type: "event", Payload: map[string]any{"session_id": "s1"}},
			filters: &SubscribeFilters{SessionID: "s1"},
			want: true,
		},
		{
			name: "session_id不匹配",
			msg: &WSMessage{Type: "event", Payload: map[string]any{"session_id": "s1"}},
			filters: &SubscribeFilters{SessionID: "s2"},
			want: false,
		},
		{
			name: "非event类型总匹配",
			msg: &WSMessage{Type: "ping", Payload: nil},
			filters: &SubscribeFilters{SessionID: "s1"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchFilters(tt.msg, tt.filters)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- TestEventToMap_转换正确 ---
func TestEventToMap_转换正确(t *testing.T) {
	ev := &event.HookEvent{
		ID:        1,
		EventID:   "evt-1",
		SessionID: "sess-1",
		EventType: "PreToolUse",
		ToolName:  "Bash",
		Cwd:       "/home",
		Hostname:  "h",
		CreatedAt: "2026-06-18T10:00:00.000Z",
	}
	m := eventToMap(ev)
	assert.Equal(t, int64(1), m["id"])
	assert.Equal(t, "evt-1", m["event_id"])
	assert.Equal(t, "PreToolUse", m["event_type"])
	assert.Equal(t, "Bash", m["tool_name"])

	// 验证可序列化
	data, err := json.Marshal(m)
	require.NoError(t, err)
	assert.Contains(t, string(data), "evt-1")
}
