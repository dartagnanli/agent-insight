// 会话列表页面
const SessionsPage = {
    sessions: [],
    total: 0,
    limit: 20,
    offset: 0,
    sortKey: 'started_at',
    sortOrder: 'desc',

    mount(container) {
        container.innerHTML = `
            <h2 style="margin-bottom:16px">会话列表</h2>
            <div class="filter-bar">
                <input id="sess-filter-project" type="text" placeholder="项目路径" style="width:200px">
                <select id="sess-sort-key">
                    <option value="started_at">按开始时间</option>
                    <option value="total_events">按事件数</option>
                    <option value="duration_secs">按持续时长</option>
                </select>
                <select id="sess-sort-order">
                    <option value="desc">降序</option>
                    <option value="asc">升序</option>
                </select>
                <button id="sess-apply-filter" class="btn-accent">刷新</button>
            </div>
            <div class="table-wrapper card" id="sess-table-wrapper"></div>
            <div class="pagination" id="sess-pagination"></div>
        `;

        document.getElementById('sess-apply-filter').addEventListener('click', () => {
            SessionsPage.sortKey = document.getElementById('sess-sort-key').value;
            SessionsPage.sortOrder = document.getElementById('sess-sort-order').value;
            SessionsPage.offset = 0;
            SessionsPage.loadSessions();
        });

        SessionsPage.loadSessions();
    },

    unmount() {},

    async loadSessions() {
        try {
            const params = {
                limit: SessionsPage.limit,
                offset: SessionsPage.offset,
                sort_by: SessionsPage.sortKey,
                sort_order: SessionsPage.sortOrder,
            };
            const projectPath = document.getElementById('sess-filter-project')?.value.trim();
            if (projectPath) params.project = projectPath;

            const data = await API.listSessions(params);
            SessionsPage.sessions = data.sessions || [];
            SessionsPage.total = data.total || 0;
            SessionsPage.renderTable();
            SessionsPage.renderPagination();
        } catch (err) {
            document.getElementById('sess-table-wrapper').innerHTML = `<div class="empty-state"><div class="empty-state-icon">&#9888;</div><div class="empty-state-text">加载失败: ${escapeHTML(err.message)}</div></div>`;
        }
    },

    renderTable() {
        const wrapper = document.getElementById('sess-table-wrapper');
        if (!SessionsPage.sessions.length) {
            wrapper.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128209;</div><div class="empty-state-text">暂无会话数据</div></div>';
            return;
        }

        wrapper.innerHTML = `<table>
            <thead><tr>
                <th>会话ID</th>
                <th>开始时间</th>
                <th>持续时长</th>
                <th>事件数</th>
                <th>工具调用</th>
                <th>拦截</th>
                <th>拦截率</th>
                <th>使用工具</th>
                <th>操作</th>
            </tr></thead>
            <tbody>${SessionsPage.sessions.map(s => {
                const blockRate = s.block_rate != null ? (s.block_rate * 100).toFixed(1) + '%' : '-';
                const dur = s.duration_secs ? formatDuration(s.duration_secs) : '-';
                let tools = '-';
                try { tools = s.tools_used ? JSON.parse(s.tools_used).map(t => `<span class="tool-tag">${escapeHTML(t)}</span>`).join('') : '-'; } catch(_) {}
                const statusBadge = s.ended_at ? '' : '<span class="badge badge-live">活跃</span>';
                return `<tr>
                    <td class="mono-cell">${escapeHTML(s.session_id.slice(0, 8))}<span style="color:var(--text-muted)">...</span></td>
                    <td>${formatTime(s.started_at)} ${statusBadge}</td>
                    <td>${dur}</td>
                    <td>${s.total_events}</td>
                    <td>${s.tool_calls || 0}</td>
                    <td style="color:${s.blocked_calls > 0 ? 'var(--danger)' : 'var(--text)'}">${s.blocked_calls || 0}</td>
                    <td>${blockRate}</td>
                    <td style="font-size:12px">${tools}</td>
                    <td class="action-cell">
                        <button class="btn-icon" title="查看调用链" onclick="location.hash='#/trace?sid=${encodeURIComponent(s.session_id)}'">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/></svg>
                        </button>
                        <button class="btn-icon" title="复制ID" onclick="navigator.clipboard.writeText('${escapeHTML(s.session_id)}')">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                        </button>
                    </td>
                </tr>`;
            }).join('')}</tbody>
        </table>`;
    },

    renderPagination() {
        const pagEl = document.getElementById('sess-pagination');
        const totalPages = Math.ceil(SessionsPage.total / SessionsPage.limit);
        if (totalPages <= 1) { pagEl.innerHTML = ''; return; }

        const currentPage = Math.floor(SessionsPage.offset / SessionsPage.limit) + 1;
        let html = '';
        html += `<button ${currentPage <= 1 ? 'disabled' : ''} data-offset="0">首页</button>`;
        html += `<button ${currentPage <= 1 ? 'disabled' : ''} data-offset="${(currentPage - 2) * SessionsPage.limit}">上一页</button>`;
        for (let p = Math.max(1, currentPage - 2); p <= Math.min(totalPages, currentPage + 2); p++) {
            html += `<button class="${p === currentPage ? 'current' : ''}" data-offset="${(p - 1) * SessionsPage.limit}">${p}</button>`;
        }
        html += `<button ${currentPage >= totalPages ? 'disabled' : ''} data-offset="${currentPage * SessionsPage.limit}">下一页</button>`;
        html += `<button ${currentPage >= totalPages ? 'disabled' : ''} data-offset="${(totalPages - 1) * SessionsPage.limit}">末页</button>`;
        pagEl.innerHTML = html;

        pagEl.querySelectorAll('button[data-offset]').forEach(btn => {
            btn.addEventListener('click', () => {
                SessionsPage.offset = parseInt(btn.dataset.offset);
                SessionsPage.loadSessions();
            });
        });
    },
};
