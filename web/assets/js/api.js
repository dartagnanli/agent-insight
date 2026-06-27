// API 封装
const API = {
    base: '',

    async fetchJSON(url, params = {}) {
        const qs = new URLSearchParams(params).toString();
        const fullURL = qs ? `${API.base}${url}?${qs}` : `${API.base}${url}`;
        const resp = await fetch(fullURL);
        if (!resp.ok) {
            const err = await resp.json().catch(() => ({ error: resp.statusText }));
            throw new Error(err.error || resp.statusText);
        }
        return resp.json();
    },

    // 事件列表
    async listEvents(params = {}) {
        return API.fetchJSON('/api/v1/events', params);
    },

    // 单条事件
    async getEvent(eventID) {
        return API.fetchJSON(`/api/v1/events/${eventID}`);
    },

    // 统计概览
    async getStats(params = {}) {
        return API.fetchJSON('/api/v1/stats', params);
    },

    // 小时级统计
    async getStatsHourly(params = {}) {
        return API.fetchJSON('/api/v1/stats/hourly', params);
    },

    // 调用链追踪
    async getTrace(sessionID) {
        return API.fetchJSON(`/api/v1/traces/${sessionID}`);
    },

    // 会话列表
    async listSessions(params = {}) {
        return API.fetchJSON('/api/v1/sessions', params);
    },

    // 版本信息
    async getVersion() {
        return API.fetchJSON('/api/v1/version');
    },
};

// 工具函数
function formatDuration(secs) {
    if (secs < 60) return `${secs}s`;
    const mins = Math.floor(secs / 60);
    const rem = secs % 60;
    if (mins < 60) return `${mins}m ${rem}s`;
    const hrs = Math.floor(mins / 60);
    const remMins = mins % 60;
    return `${hrs}h ${remMins}m`;
}

function formatTime(ts) {
    if (!ts) return '-';
    const d = new Date(ts);
    return d.toLocaleString('zh-CN', { hour12: false });
}

function formatMs(ms) {
    if (ms == null) return '-';
    if (ms < 1000) return `${ms.toFixed(0)}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    return formatDuration(Math.floor(ms / 1000));
}

function escapeHTML(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}
