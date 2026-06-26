// SPA Hash 路由
const App = {
    currentPage: null,

    init() {
        window.addEventListener('hashchange', App.route);
        WS.connect();
        App.route();
    },

    route() {
        const hash = location.hash || '#/dashboard';
        const appEl = document.getElementById('app');

        // 卸载当前页面
        if (App.currentPage && App.currentPage.unmount) {
            App.currentPage.unmount();
        }
        App.currentPage = null;

        // 更新导航高亮
        document.querySelectorAll('.nav-link').forEach(link => {
            link.classList.toggle('active', link.getAttribute('href') === hash.split('/')[1] && hash.startsWith('#/' + link.getAttribute('href').split('/')[1])));
        });

        // 更精确的导航高亮
        const pageName = hash.split('/')[1];
        document.querySelectorAll('.nav-link').forEach(link => {
            const linkPage = link.dataset.page;
            link.classList.toggle('active', linkPage === pageName);
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
        } else if (hash.startsWith('#/trace/')) {
            const sessionID = hash.replace('#/trace/', '');
            App.currentPage = TracePage;
            TracePage.mount(appEl, sessionID);
        } else if (hash === '#/trace') {
            App.currentPage = TracePage;
            TracePage.mount(appEl, '');
        } else {
            appEl.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128559;</div><div class="empty-state-text">页面不存在</div></div>';
        }
    },
};

// 启动
document.addEventListener('DOMContentLoaded', App.init);
