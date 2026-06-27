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
                <button id="trace-load-btn" class="btn-accent">加载</button>
            </div>
            <div id="trace-content"></div>
        `;

        document.getElementById('trace-load-btn').addEventListener('click', () => {
            TracePage.sessionID = document.getElementById('trace-session-input').value.trim();
            if (TracePage.sessionID) {
                TracePage.loadTrace();
            }
        });

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
        const totalDurationMs = totalDuration * 1000;
        const startedAt = trace.started_at ? new Date(trace.started_at).getTime() : 0;
        const endedAt = trace.ended_at ? new Date(trace.ended_at).getTime() : startedAt + totalDurationMs;

        let html = '';

        // 会话信息卡片
        html += `<div class="card" style="margin-bottom:16px">
            <div class="trace-header">
                <div>
                    <div class="card-title" style="margin-bottom:4px">会话信息</div>
                    <div class="mono-cell" style="font-size:11px;color:var(--text-muted)">${escapeHTML(trace.session_id)}</div>
                </div>
                <div class="trace-meta">
                    <span>时长 <strong>${formatDuration(totalDuration)}</strong></span>
                    <span>事件 <strong>${trace.total_events}</strong></span>
                    <span>调用 <strong>${trace.spans ? trace.spans.length : 0}</strong></span>
                </div>
            </div>
        </div>`;

        // 时间轴区域
        html += '<div class="card">';

        // 时间刻度
        if (totalDurationMs > 0) {
            html += '<div class="trace-ruler">';
            const tickCount = Math.min(6, Math.ceil(totalDuration));
            for (let i = 0; i <= tickCount; i++) {
                const sec = Math.round(totalDuration * i / tickCount);
                const left = (i / tickCount * 100);
                html += `<span style="left:${left}%">${formatDuration(sec)}</span>`;
            }
            html += '</div>';
        }

        // 独立事件（渲染为标记点）
        if (trace.standalone_events && trace.standalone_events.length) {
            html += '<div style="margin:8px 0 4px">';
            trace.standalone_events.forEach(se => {
                const offsetMs = new Date(se.created_at).getTime() - startedAt;
                const leftPercent = totalDurationMs > 0 ? (offsetMs / totalDurationMs * 100) : 0;
                const typeClass = se.event_type === 'SessionStart' ? 'start' : se.event_type === 'Stop' ? 'stop' : '';
                html += `<div class="trace-marker" style="left:${leftPercent}%">
                    <span class="event-type ${typeClass}">${escapeHTML(se.event_type)}</span>
                </div>`;
            });
            html += '</div>';
        }

        // Spans（瀑布条）
        if (trace.spans && trace.spans.length) {
            html += '<div class="trace-timeline">';
            trace.spans.forEach(span => {
                const startMs = new Date(span.started_at).getTime() - startedAt;
                const leftPercent = totalDurationMs > 0 ? (startMs / totalDurationMs * 100) : 0;
                const widthPercent = totalDurationMs > 0 ? Math.max(span.duration_ms / totalDurationMs * 100, 0.5) : 2;
                const barClass = span.blocked ? 'blocked' : span.orphan ? 'orphan' : 'tool';

                const durStr = formatMs(span.duration_ms);
                const label = `${escapeHTML(span.tool_name)} ${durStr}${span.blocked ? ' [拦截]' : ''}${span.orphan ? ' [孤立]' : ''}`;

                html += `<div class="trace-bar ${barClass}" style="left:${leftPercent}%;width:${widthPercent}%">
                    <span class="trace-bar-label">${label}</span>
                </div>`;
            });
            html += '</div>';
        } else if (!trace.standalone_events || !trace.standalone_events.length) {
            html += '<div class="empty-state" style="padding:24px"><div class="empty-state-text">该会话无调用数据</div></div>';
        }

        html += '</div>';
        container.innerHTML = html;
    },
};
