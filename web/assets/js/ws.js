// WebSocket 管理
const WS = {
    url: '',
    conn: null,
    reconnectTimer: null,
    callbacks: {},
    statusEl: null,

    connect() {
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        WS.url = `${proto}//${location.host}/api/v1/ws/events`;

        if (WS.conn) {
            WS.conn.close();
        }

        try {
            WS.conn = new WebSocket(WS.url);
        } catch (e) {
            WS.scheduleReconnect();
            return;
        }

        WS.conn.onopen = () => {
            WS.updateStatus('connected');
            WS.schedulePing();
        };

        WS.conn.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            if (msg.type === 'event' && WS.callbacks.onEvent) {
                WS.callbacks.onEvent(msg.payload);
            } else if (msg.type === 'pong') {
                // 心跳响应，忽略
            }
        };

        WS.conn.onclose = () => {
            WS.updateStatus('disconnected');
            WS.scheduleReconnect();
        };

        WS.conn.onerror = () => {
            WS.updateStatus('disconnected');
        };
    },

    send(msg) {
        if (WS.conn && WS.conn.readyState === WebSocket.OPEN) {
            WS.conn.send(JSON.stringify(msg));
        }
    },

    // 订阅过滤条件
    subscribe(filters = {}) {
        WS.send({ type: 'subscribe', ...filters });
    },

    // 心跳：30s 发一次 ping
    schedulePing() {
        if (WS._pingTimer) clearInterval(WS._pingTimer);
        WS._pingTimer = setInterval(() => {
            WS.send({ type: 'ping' });
        }, 30000);
    },

    // 断线重连：3s 后重试
    scheduleReconnect() {
        if (WS.reconnectTimer) clearTimeout(WS.reconnectTimer);
        WS.reconnectTimer = setTimeout(() => {
            WS.connect();
        }, 3000);
    },

    // 更新连接状态指示
    updateStatus(status) {
        if (!WS.statusEl) {
            WS.statusEl = document.querySelector('.ws-status');
        }
        if (WS.statusEl) {
            WS.statusEl.className = `ws-status ${status}`;
            WS.statusEl.textContent = status === 'connected' ? '已连接' : '未连接';
        }
    },

    on(event, callback) {
        WS.callbacks[event] = callback;
    },
};
