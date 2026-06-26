// 调用瀑布图页面
const TracePage = {
    sessionID: '',
    trace: null,

    mount(container, sessionID) {
        TracePage.sessionID = sessionID || '';
        container.innerHTML = `
            <h2 style="margin-bottom:16px">调用链追踪</h2>
            <div class="filter-bar">
                <input id="trace-session-input" type="text" placeholder="输入会话ID" value="${escapeHTML(TracePage.sessionID)}" style="width:300px">
                <button id="trace-load-btn" style="background:var(--accent);color:#fff;border:none;border-radius:var(--radius);padding:6px 16px;cursor:pointer">加载</button>
            </div>
            <div id="trace-content"></div>
        `;

        document.getElementById('trace-load-btn').addEventListener('click', () => {
            TracePage.sessionID = document.getElementById('trace-session-input').value.trim();
            if (TracePage.sessionID) {
                TracePage.loadTrace();
            }
        });

        // 如果有 sessionID 就自动加载
        if (TracePage.sessionID) {
            TracePage.loadTrace();
        }
    },

    unmount() {},

    async loadTrace() {
        const contentEl = document.getElementById('trace-content');
        contentEl.innerHTML = '<div style="color:var(--text-muted)">加载中...</div>';

        try {
            const trace = await API.getTrace(TracePage.sessionID);
            TracePage.trace = trace;
            TracePage.renderTrace(contentEl, trace);
        } catch (err) {
            contentEl.innerHTML = `<div class="empty-state"><div class="empty-state-icon">&#9888;</div><div class="empty-state-text">加载失败: ${escapeHTML(err.message)}</div></div>`;
        }
    },

    renderTrace(container, trace) {
        if (!trace) {
            container.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128269;</div><div class="empty-state-text">请输入会话ID查看调用链</div></div>';
            return;
        }

        const totalDuration = trace.duration_secs || 0;
        const startedAt = new Date(trace.started_at).getTime();

        // 计算 time range
        let minTime = startedAt;
        let maxTime = startedAt + totalDuration * 1000;

        let html = `<div class="card" style="margin-bottom:16px">
            <div class="trace-header">
                <div><strong>会话ID:</strong> ${escapeHTML(trace.session_id)}</div>
                <div><strong>总时长:</strong> ${formatDuration(trace.duration_secs)} &nbsp; <strong>事件数:</strong> ${trace.total_events}</div>
            </div>
        </div>`;

        // 独立事件
        if (trace.standalone_events && trace.standalone_events.length) {
            html += '<div style="margin-bottom:12px">';
            html += '<div class="card-title" style="margin-bottom:8px">独立事件</div>';
            trace.standalone_events.forEach(se => {
                const offsetMs = new Date(se.created_at).getTime() - minTime;
                const leftPercent = totalDuration > 0 ? (offsetMs / (totalDuration * 1000) * 100) : 0;
                html += `<div class="trace-standalone" style="margin-left:${leftPercent}%">
                    <span class="event-type ${se.event_type === 'SessionStart' ? 'start' : se.event_type === 'Stop' ? 'stop' : ''}">${escapeHTML(se.event_type)}</span>
                    <span style="color:var(--text-muted);margin-left:8px">${formatTime(se.created_at)}</span>
                </div>`;
            });
            html += '</div>';
        }

        // 时间轴
        html += '<div class="card"><div class="trace-timeline">';

        // Spans
        if (trace.spans && trace.spans.length) {
            trace.spans.forEach(span => {
                const startMs = new Date(span.started_at).getTime() - minTime;
                const leftPercent = totalDuration > 0 ? (startMs / (totalDuration * 1000) * 100) : 0;
                const widthPercent = totalDuration > 0 ? Math.max(span.duration_ms / (totalDuration * 1000) * 100, 0.5) : 2;

                const barClass = span.blocked ? 'blocked' : span.orphan ? 'orphan' : 'tool';
                const label = `${escapeHTML(span.tool_name)} ${span.duration_ms}ms${span.blocked ? ' [拦截]' : ''}${span.orphan ? ' [孤立]' : ''}`;

                html += `<div class="trace-bar ${barClass}" style="left:${leftPercent}%;width:${widthPercent}%">
                    <span class="trace-bar-label">${label}</span>
                </div>`;
            });
        }

        html += '</div></div>';
        container.innerHTML = html;
    },
};
