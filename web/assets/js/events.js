// 事件流页面
const EventsPage = {
    events: [],
    total: 0,
    limit: 50,
    offset: 0,
    filters: {
        session_id: '',
        event_type: '',
        tool_name: '',
        blocked: '',
    },

    mount(container) {
        container.innerHTML = `
            <h2 style="margin-bottom:16px">
                <span class="live-dot"></span>事件流
                <span class="ws-status disconnected" style="margin-left:12px">未连接</span>
            </h2>
            <div class="filter-bar">
                <select id="evt-filter-type">
                    <option value="">全部类型</option>
                    <option value="PreToolUse">PreToolUse</option>
                    <option value="PostToolUse">PostToolUse</option>
                    <option value="SessionStart">SessionStart</option>
                    <option value="Stop">Stop</option>
                    <option value="SubagentStop">SubagentStop</option>
                </select>
                <select id="evt-filter-tool">
                    <option value="">全部工具</option>
                    <option value="Bash">Bash</option>
                    <option value="Write">Write</option>
                    <option value="Edit">Edit</option>
                    <option value="Read">Read</option>
                    <option value="Glob">Glob</option>
                    <option value="Grep">Grep</option>
                    <option value="WebFetch">WebFetch</option>
                </select>
                <select id="evt-filter-blocked">
                    <option value="">全部状态</option>
                    <option value="true">已拦截</option>
                    <option value="false">未拦截</option>
                </select>
                <input id="evt-filter-session" type="text" placeholder="会话ID" style="width:160px">
                <button id="evt-apply-filter" style="background:var(--accent);color:#fff;border:none;border-radius:var(--radius);padding:6px 16px;cursor:pointer">筛选</button>
            </div>
            <div id="evt-list"></div>
            <div class="pagination" id="evt-pagination"></div>
        `;

        // 绑定事件
        document.getElementById('evt-apply-filter').addEventListener('click', () => {
            EventsPage.filters.event_type = document.getElementById('evt-filter-type').value;
            EventsPage.filters.tool_name = document.getElementById('evt-filter-tool').value;
            EventsPage.filters.blocked = document.getElementById('evt-filter-blocked').value;
            EventsPage.filters.session_id = document.getElementById('evt-filter-session').value;
            EventsPage.offset = 0;
            EventsPage.loadEvents();
        });

        // WebSocket 实时事件
        WS.on('onEvent', (evt) => {
            if (EventsPage.offset === 0) {
                EventsPage.events.unshift(evt);
                if (EventsPage.events.length > EventsPage.limit) {
                    EventsPage.events = EventsPage.events.slice(0, EventsPage.limit);
                }
                EventsPage.renderEvents();
            }
        });

        EventsPage.loadEvents();
    },

    unmount() {
        WS.on('onEvent', null);
    },

    async loadEvents() {
        try {
            const params = { limit: EventsPage.limit, offset: EventsPage.offset };
            if (EventsPage.filters.event_type) params.event_type = EventsPage.filters.event_type;
            if (EventsPage.filters.tool_name) params.tool_name = EventsPage.filters.tool_name;
            if (EventsPage.filters.blocked) params.blocked = EventsPage.filters.blocked;
            if (EventsPage.filters.session_id) params.session_id = EventsPage.filters.session_id;

            const data = await API.listEvents(params);
            EventsPage.events = data.events || [];
            EventsPage.total = data.total || 0;
            EventsPage.renderEvents();
            EventsPage.renderPagination();
        } catch (err) {
            document.getElementById('evt-list').innerHTML = `<div class="empty-state"><div class="empty-state-icon">&#9888;</div><div class="empty-state-text">加载失败: ${escapeHTML(err.message)}</div></div>`;
        }
    },

    renderEvents() {
        const listEl = document.getElementById('evt-list');
        if (!EventsPage.events.length) {
            listEl.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128466;</div><div class="empty-state-text">暂无事件数据</div></div>';
            return;
        }

        listEl.innerHTML = EventsPage.events.map(evt => {
            const typeClass = evt.event_type === 'PreToolUse' ? 'pre' :
                              evt.event_type === 'PostToolUse' ? 'post' :
                              evt.event_type === 'SessionStart' ? 'start' :
                              evt.event_type === 'Stop' || evt.event_type === 'SubagentStop' ? 'stop' : '';
            const blocked = evt.blocked ? '<span style="color:var(--danger);font-weight:600;margin-left:8px">[已拦截]</span>' : '';
            const toolName = evt.tool_name ? `<span style="color:var(--accent);margin-left:8px">${escapeHTML(evt.tool_name)}</span>` : '';
            const duration = evt.hook_duration_ms ? `<span style="color:var(--text-muted);margin-left:8px">${evt.hook_duration_ms}ms</span>` : '';

            return `<div class="event-row" data-event-id="${escapeHTML(evt.event_id)}">
                <div class="event-header">
                    <div>
                        <span class="event-type ${typeClass}">${escapeHTML(evt.event_type)}</span>
                        ${toolName}${blocked}${duration}
                    </div>
                    <span style="color:var(--text-muted);font-size:12px">${formatTime(evt.created_at)}</span>
                </div>
                <div class="event-json">${escapeHTML(JSON.stringify(evt, null, 2))}</div>
            </div>`;
        }).join('');

        // 绑定展开事件
        listEl.querySelectorAll('.event-row').forEach(row => {
            row.addEventListener('click', () => row.classList.toggle('expanded'));
        });
    },

    renderPagination() {
        const pagEl = document.getElementById('evt-pagination');
        const totalPages = Math.ceil(EventsPage.total / EventsPage.limit);
        if (totalPages <= 1) { pagEl.innerHTML = ''; return; }

        const currentPage = Math.floor(EventsPage.offset / EventsPage.limit) + 1;
        let html = '';
        html += `<button ${currentPage <= 1 ? 'disabled' : ''} data-offset="0">首页</button>`;
        html += `<button ${currentPage <= 1 ? 'disabled' : ''} data-offset="${(currentPage - 2) * EventsPage.limit}">上一页</button>`;
        for (let p = Math.max(1, currentPage - 2); p <= Math.min(totalPages, currentPage + 2); p++) {
            html += `<button class="${p === currentPage ? 'current' : ''}" data-offset="${(p - 1) * EventsPage.limit}">${p}</button>`;
        }
        html += `<button ${currentPage >= totalPages ? 'disabled' : ''} data-offset="${currentPage * EventsPage.limit}">下一页</button>`;
        html += `<button ${currentPage >= totalPages ? 'disabled' : ''} data-offset="${(totalPages - 1) * EventsPage.limit}">末页</button>`;
        pagEl.innerHTML = html;

        pagEl.querySelectorAll('button[data-offset]').forEach(btn => {
            btn.addEventListener('click', () => {
                EventsPage.offset = parseInt(btn.dataset.offset);
                EventsPage.loadEvents();
            });
        });
    },
};
