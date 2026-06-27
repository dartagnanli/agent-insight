// WebSocket 管理
const WS = {
    url: '',
    conn: null,
    reconnectTimer: null,
    callbacks: {},

    connect() {
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        WS.url = `${proto}//${location.host}/api/v1/ws/events`;

        if (WS.conn) WS.conn.close();

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

    subscribe(filters = {}) {
        WS.send({ type: 'subscribe', ...filters });
    },

    schedulePing() {
        if (WS._pingTimer) clearInterval(WS._pingTimer);
        WS._pingTimer = setInterval(() => WS.send({ type: 'ping' }), 30000);
    },

    scheduleReconnect() {
        if (WS.reconnectTimer) clearTimeout(WS.reconnectTimer);
        WS.reconnectTimer = setTimeout(() => WS.connect(), 3000);
    },

    updateStatus(status) {
        const el = document.getElementById('ws-indicator');
        if (!el) return;
        el.className = `ws-status ${status}`;
        const dot = el.querySelector('.ws-dot');
        const text = el.querySelector('.ws-text');
        if (dot) dot.style.background = status === 'connected' ? 'var(--success)' : 'var(--danger)';
        if (text) text.textContent = status === 'connected' ? '实时' : '离线';
    },

    on(event, callback) {
        WS.callbacks[event] = callback;
    },
};
