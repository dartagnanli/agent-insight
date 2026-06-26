// 仪表板统计概览页面
const DashboardPage = {
    charts: {},

    mount(container) {
        container.innerHTML = `
            <h2 style="margin-bottom:16px">统计概览</h2>
            <div class="stats-grid" id="dash-stats">
                <div class="card"><div class="card-title">事件总数</div><div class="card-value" id="stat-total">-</div></div>
                <div class="card"><div class="card-title">会话数</div><div class="card-value" id="stat-sessions">-</div></div>
                <div class="card"><div class="card-title">拦截总数</div><div class="card-value" id="stat-blocked">-</div></div>
                <div class="card"><div class="card-title">拦截率</div><div class="card-value" id="stat-block-rate">-</div></div>
            </div>
            <div class="charts-row">
                <div class="chart-container">
                    <div class="card-title" style="margin-bottom:12px">工具使用分布</div>
                    <canvas id="chart-tools"></canvas>
                </div>
                <div class="chart-container">
                    <div class="card-title" style="margin-bottom:12px">事件类型分布</div>
                    <canvas id="chart-types"></canvas>
                </div>
            </div>
            <div class="chart-container" style="margin-bottom:24px">
                <div class="card-title" style="margin-bottom:12px">小时级趋势</div>
                <canvas id="chart-hourly" style="max-height:220px"></canvas>
            </div>
            <h3 style="margin-bottom:8px">工具耗时详情</h3>
            <div class="table-wrapper card" id="dash-tool-table"></div>
        `;

        DashboardPage.loadStats();

        // 实时更新
        WS.on('onEvent', () => {
            DashboardPage.loadStats();
        });
    },

    unmount() {
        // 销毁图表
        Object.values(DashboardPage.charts).forEach(c => c && c.destroy());
        DashboardPage.charts = {};
        WS.on('onEvent', null);
    },

    async loadStats() {
        try {
            const data = await API.getStats();
            DashboardPage.renderStats(data);
            DashboardPage.renderToolChart(data.tool_distribution || {});
            DashboardPage.renderTypeChart(data.event_type_distribution || {});
            DashboardPage.renderHourlyChart(data.hourly_trend || []);
            DashboardPage.renderToolTable(data.tool_details || {});
        } catch (err) {
            console.error('Failed to load stats:', err);
        }
    },

    renderStats(data) {
        document.getElementById('stat-total').textContent = data.total_events || 0;
        document.getElementById('stat-sessions').textContent = data.total_sessions || 0;
        document.getElementById('stat-blocked').textContent = data.total_blocked || 0;
        const rate = data.block_rate != null ? (data.block_rate * 100).toFixed(1) + '%' : '-';
        document.getElementById('stat-block-rate').textContent = rate;
    },

    renderToolChart(toolDist) {
        const labels = Object.keys(toolDist);
        const values = Object.values(toolDist);
        const colors = ['#6366f1','#8b5cf6','#a78bfa','#c4b5fd','#818cf8','#6ee7b7','#fbbf24','#f87171'];

        if (DashboardPage.charts.tools) DashboardPage.charts.tools.destroy();

        const ctx = document.getElementById('chart-tools');
        if (!ctx) return;
        DashboardPage.charts.tools = new Chart(ctx, {
            type: 'doughnut',
            data: { labels, datasets: [{ data: values, backgroundColor: colors.slice(0, labels.length) }] },
            options: {
                responsive: true,
                plugins: { legend: { position: 'right', labels: { color: '#e1e4ed', font: { size: 12 } } } }
            }
        });
    },

    renderTypeChart(typeDist) {
        const labels = Object.keys(typeDist);
        const values = Object.values(typeDist);
        const colors = ['#22c55e','#6366f1','#ef4444','#f59e0b','#3b82f6'];

        if (DashboardPage.charts.types) DashboardPage.charts.types.destroy();

        const ctx = document.getElementById('chart-types');
        if (!ctx) return;
        DashboardPage.charts.types = new Chart(ctx, {
            type: 'pie',
            data: { labels, datasets: [{ data: values, backgroundColor: colors.slice(0, labels.length) }] },
            options: {
                responsive: true,
                plugins: { legend: { position: 'right', labels: { color: '#e1e4ed', font: { size: 12 } } } }
            }
        });
    },

    renderHourlyChart(buckets) {
        if (!buckets || !buckets.length) return;

        // 按时间桶聚合 event_count
        const byHour = {};
        buckets.forEach(b => {
            const hour = b.bucket_hour;
            byHour[hour] = (byHour[hour] || 0) + b.event_count;
        });
        const sortedHours = Object.keys(byHour).sort();
        const labels = sortedHours.map(h => h.replace('T', ' ').slice(0, 16));
        const values = sortedHours.map(h => byHour[h]);

        if (DashboardPage.charts.hourly) DashboardPage.charts.hourly.destroy();

        const ctx = document.getElementById('chart-hourly');
        if (!ctx) return;
        DashboardPage.charts.hourly = new Chart(ctx, {
            type: 'bar',
            data: {
                labels,
                datasets: [{ label: '事件数', data: values, backgroundColor: '#6366f1', borderRadius: 4 }]
            },
            options: {
                responsive: true,
                scales: {
                    x: { ticks: { color: '#8b92a8', maxTicksLimit: 24 }, grid: { color: '#2d3348' } },
                    y: { ticks: { color: '#8b92a8' }, grid: { color: '#2d3348' }, beginAtZero: true }
                },
                plugins: { legend: { display: false } }
            }
        });
    },

    renderToolTable(toolDetails) {
        const el = document.getElementById('dash-tool-table');
        if (!el) return;
        const tools = Object.entries(toolDetails);
        if (!tools.length) { el.innerHTML = '<div class="empty-state" style="padding:16px"><div class="empty-state-text">暂无工具耗时数据</div></div>'; return; }

        el.innerHTML = `<table>
            <thead><tr><th>工具名称</th><th>调用次数</th><th>拦截次数</th><th>平均耗时</th><th>P99 耗时</th></tr></thead>
            <tbody>${tools.map(([name, d]) => `<tr>
                <td style="color:var(--accent);font-weight:500">${escapeHTML(name)}</td>
                <td>${d.count}</td>
                <td style="color:${d.blocked > 0 ? 'var(--danger)' : 'var(--text)'}">${d.blocked}</td>
                <td>${d.avg_duration_ms != null ? d.avg_duration_ms.toFixed(1) + 'ms' : '-'}</td>
                <td>${d.p99_duration_ms != null ? d.p99_duration_ms.toFixed(1) + 'ms' : '-'}</td>
            </tr>`).join('')}</tbody>
        </table>`;
    },
};
