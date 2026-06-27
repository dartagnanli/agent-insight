// SPA Hash 路由
const App = {
    currentPage: null,

    async init() {
        window.addEventListener('hashchange', App.route);
        WS.connect();
        App.route();

        // 加载版本号
        try {
            const v = await API.getVersion();
            const el = document.getElementById('nav-version');
            if (el && v.version) el.textContent = 'v' + v.version;
        } catch (_) {}
    },

    route() {
        const hash = location.hash || '#/dashboard';
        const appEl = document.getElementById('app');

        // 卸载当前页面
        if (App.currentPage && App.currentPage.unmount) {
            App.currentPage.unmount();
        }
        App.currentPage = null;

        // 导航高亮
        const pageName = hash.split('/')[1] || 'dashboard';
        document.querySelectorAll('.nav-link').forEach(link => {
            link.classList.toggle('active', link.dataset.page === pageName);
        });

        // 路由分发
        if (hash === '#/dashboard' || hash === '#/' || hash === '') {
            App.currentPage = DashboardPage;
            DashboardPage.mount(appEl);
        } else if (hash === '#/events') {
            App.currentPage = EventsPage;
            EventsPage.mount(appEl);
        } else if (hash === '#/sessions') {
            App.currentPage = SessionsPage;
            SessionsPage.mount(appEl);
        } else if (hash.startsWith('#/trace')) {
            const params = new URLSearchParams(hash.split('?')[1] || '');
            const sessionID = params.get('sid') || '';
            App.currentPage = TracePage;
            TracePage.mount(appEl, sessionID);
        } else {
            appEl.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128559;</div><div class="empty-state-text">页面不存在</div></div>';
        }
    },
};

// 启动
document.addEventListener('DOMContentLoaded', App.init);
