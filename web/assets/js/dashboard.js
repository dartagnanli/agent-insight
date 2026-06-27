// 仪表板统计概览页面
const DashboardPage = {
    charts: {},

    mount(container) {
        container.innerHTML = `
            <div class="stats-grid" id="dash-stats"></div>
            <div class="charts-row" id="dash-charts"></div>
            <div id="dash-tables"></div>
        `;
        DashboardPage.loadStats();
    },

    unmount() {
        Object.values(DashboardPage.charts).forEach(c => c.destroy());
        DashboardPage.charts = {};
    },

    async loadStats() {
        try {
            const data = await API.getStats();
            DashboardPage.renderStats(data);
            DashboardPage.renderCharts(data);
            DashboardPage.renderToolTable(data);
        } catch (err) {
            document.getElementById('dash-stats').innerHTML = `<div class="empty-state"><div class="empty-state-text">加载失败: ${escapeHTML(err.message)}</div></div>`;
        }
    },

    renderStats(data) {
        const totalEvents = data.total_events || 0;
        const totalSessions = data.total_sessions || 0;
        const blockRate = data.block_rate != null ? (data.block_rate * 100).toFixed(1) + '%' : '-';
        const p99Ms = data.p99_hook_duration_ms != null ? formatMs(data.p99_hook_duration_ms) : '-';

        document.getElementById('dash-stats').innerHTML = `
            <div class="stat-card">
                <div class="stat-icon events">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>
                </div>
                <div class="stat-title">事件总数</div>
                <div class="stat-value">${totalEvents.toLocaleString()}</div>
            </div>
            <div class="stat-card">
                <div class="stat-icon sessions">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
                </div>
                <div class="stat-title">会话数</div>
                <div class="stat-value">${totalSessions}</div>
            </div>
            <div class="stat-card">
                <div class="stat-icon blocked">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>
                </div>
                <div class="stat-title">拦截率</div>
                <div class="stat-value">${blockRate}</div>
                <div class="stat-sub">${data.total_blocked || 0} 次拦截</div>
            </div>
            <div class="stat-card">
                <div class="stat-icon latency">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
                </div>
                <div class="stat-title">P99 Hook 开销</div>
                <div class="stat-value">${p99Ms}</div>
                <div class="stat-sub">Avg ${formatMs(data.avg_hook_duration_ms)}</div>
            </div>
        `;
    },

    renderCharts(data) {
        const chartsEl = document.getElementById('dash-charts');
        if (!chartsEl) return;

        const toolDist = data.tool_distribution || {};
        const evtDist = data.event_type_distribution || {};

        chartsEl.innerHTML = `
            <div class="chart-card">
                <div class="chart-card-title">工具使用分布</div>
                <canvas id="chart-tool-dist"></canvas>
            </div>
            <div class="chart-card">
                <div class="chart-card-title">事件类型分布</div>
                <canvas id="chart-evt-dist"></canvas>
            </div>
        `;

        // 饼图 - 工具分布（取前8）
        const toolEntries = Object.entries(toolDist).sort((a,b) => b[1]-a[1]).slice(0, 8);
        const toolLabels = toolEntries.map(e => e[0].length > 15 ? e[0].slice(0,12) + '...' : e[0]);
        const toolValues = toolEntries.map(e => e[1]);

        if (toolEntries.length && typeof Chart !== 'undefined') {
            DashboardPage.charts.tool = new Chart(document.getElementById('chart-tool-dist'), {
                type: 'doughnut',
                data: {
                    labels: toolLabels,
                    datasets: [{ data: toolValues, backgroundColor: ['#6366f1','#818cf8','#60a5fa','#38bdf8','#2dd4bf','#4ade80','#a78bfa','#c084fc'] }]
                },
                options: {
                    responsive: true,
                    plugins: {
                        legend: { position: 'right', labels: { color: '#9ca3b8', font: { size: 11 }, padding: 8, boxWidth: 10 } }
                    },
                    cutout: '55%'
                }
            });
        }

        // 柱状图 - 事件类型
        const evtEntries = Object.entries(evtDist);
        if (evtEntries.length && typeof Chart !== 'undefined') {
            DashboardPage.charts.evt = new Chart(document.getElementById('chart-evt-dist'), {
                type: 'bar',
                data: {
                    labels: evtEntries.map(e => e[0]),
                    datasets: [{ data: evtEntries.map(e => e[1]), backgroundColor: '#6366f1', borderRadius: 4, maxBarThickness: 40 }]
                },
                options: {
                    responsive: true,
                    plugins: { legend: { display: false } },
                    scales: {
                        x: { ticks: { color: '#9ca3b8', font: { size: 10 } }, grid: { display: false } },
                        y: { ticks: { color: '#9ca3b8', font: { size: 10 } }, grid: { color: '#252a3a' } }
                    }
                }
            });
        }
    },

    renderToolTable(data) {
        const tablesEl = document.getElementById('dash-tables');
        if (!tablesEl) return;

        const toolDetails = data.tool_details || {};
        const entries = Object.entries(toolDetails).sort((a,b) => b[1].count - a[1].count);

        if (!entries.length) { tablesEl.innerHTML = ''; return; }

        tablesEl.innerHTML = `
            <div class="card" style="margin-top:12px">
                <div class="chart-card-title" style="margin-bottom:12px">工具详情</div>
                <div class="table-wrapper">
                    <table>
                        <thead><tr>
                            <th>工具</th>
                            <th>调用次数</th>
                            <th>拦截</th>
                            <th>平均耗时</th>
                            <th>P99 耗时</th>
                        </tr></thead>
                        <tbody>${entries.map(([name, d]) => `<tr>
                            <td><span class="tool-tag">${escapeHTML(name)}</span></td>
                            <td>${d.count}</td>
                            <td>${d.blocked > 0 ? `<span class="text-danger">${d.blocked}</span>` : '0'}</td>
                            <td>${formatMs(d.avg_duration_ms)}</td>
                            <td>${formatMs(d.p99_duration_ms)}</td>
                        </tr>`).join('')}</tbody>
                    </table>
                </div>
            </div>
        `;
    },
};
