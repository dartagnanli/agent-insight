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
            if (TracePage.sessionID) TracePage.loadTrace();
        });

        if (TracePage.sessionID) TracePage.loadTrace();
    },

    unmount() {},

    async loadTrace() {
        const contentEl = document.getElementById('trace-content');
        contentEl.innerHTML = '<div style="color:var(--text-muted);padding:20px">加载中...</div>';
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

        const totalSec = trace.duration_secs || 0;
        const totalMs = totalSec * 1000;
        const startTs = trace.started_at ? new Date(trace.started_at).getTime() : 0;

        // 合并所有事件为统一时间线
        const items = [];

        // 独立事件
        (trace.standalone_events || []).forEach(se => {
            items.push({ type: 'event', eventType: se.event_type, at: se.created_at, ts: new Date(se.created_at).getTime() });
        });

        // Spans
        (trace.spans || []).forEach(span => {
            items.push({
                type: 'span',
                tool: span.tool_name,
                at: span.started_at,
                endAt: span.ended_at,
                ts: new Date(span.started_at).getTime(),
                endTs: new Date(span.ended_at || span.started_at).getTime(),
                durMs: span.duration_ms || 0,
                blocked: span.blocked,
                orphan: span.orphan,
            });
        });

        // 按时间排序
        items.sort((a, b) => a.ts - b.ts);

        // 求最大单次耗时，用于 bar 宽度映射
        let maxDurMs = 0;
        items.forEach(it => { if (it.durMs > maxDurMs) maxDurMs = it.durMs; });
        if (maxDurMs < 1) maxDurMs = 1;

        let html = '';

        // 会话信息卡片
        html += `<div class="card" style="margin-bottom:16px">
            <div class="trace-header">
                <div>
                    <div class="card-title" style="margin-bottom:4px">会话信息</div>
                    <div class="mono-cell" style="font-size:11px;color:var(--text-muted)">${escapeHTML(trace.session_id)}</div>
                </div>
                <div class="trace-meta">
                    <span>时长 <strong>${formatDuration(totalSec)}</strong></span>
                    <span>事件 <strong>${trace.total_events}</strong></span>
                    <span>调用 <strong>${(trace.spans || []).length}</strong></span>
                </div>
            </div>
        </div>`;

        // 瀑布图区域
        html += '<div class="card" style="padding:0;overflow:hidden">';

        // 时间刻度条
        if (totalMs > 0) {
            html += '<div class="trace-ruler-row">';
            html += '<div class="trace-label-col"></div>';
            html += '<div class="trace-bar-col"><div class="trace-ruler">';
            const tickCount = Math.min(8, Math.max(4, Math.ceil(totalSec / 300)));
            for (let i = 0; i <= tickCount; i++) {
                const sec = Math.round(totalSec * i / tickCount);
                const left = (i / tickCount * 100);
                html += `<span style="left:${left}%">${formatDuration(sec)}</span>`;
            }
            html += '</div></div></div>';
        }

        // 逐行渲染
        items.forEach(it => {
            const offsetMs = it.ts - startTs;
            const leftPct = totalMs > 0 ? (offsetMs / totalMs * 100) : 0;

            if (it.type === 'event') {
                const cls = it.eventType === 'SessionStart' ? 'start' : it.eventType === 'Stop' || it.eventType === 'SubagentStop' ? 'stop' : '';
                html += `<div class="trace-row">
                    <div class="trace-label-col">
                        <span class="event-type ${cls}" style="font-size:11px">${escapeHTML(it.eventType)}</span>
                    </div>
                    <div class="trace-bar-col">
                        <div class="trace-dot" style="left:${leftPct}%"></div>
                    </div>
                </div>`;
            } else {
                const widthPct = totalMs > 0 ? Math.max(it.durMs / totalMs * 100, 0.3) : 1;
                const barClass = it.blocked ? 'blocked' : it.orphan ? 'orphan' : 'tool';
                const suffix = it.blocked ? ' [拦截]' : it.orphan ? ' [孤立]' : '';
                html += `<div class="trace-row">
                    <div class="trace-label-col">
                        <span class="trace-tool-name">${escapeHTML(it.tool)}</span>
                        <span class="trace-tool-dur">${formatMs(it.durMs)}${suffix}</span>
                    </div>
                    <div class="trace-bar-col">
                        <div class="trace-bar ${barClass}" style="left:${leftPct}%;width:${widthPct}%"></div>
                    </div>
                </div>`;
            }
        });

        if (!items.length) {
            html += '<div class="empty-state" style="padding:40px"><div class="empty-state-text">该会话无调用数据</div></div>';
        }

        html += '</div>';
        container.innerHTML = html;
    },
};
