package dashboard

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/dartagnanli/agent-insight/internal/config"
	"github.com/dartagnanli/agent-insight/internal/session"
	"github.com/dartagnanli/agent-insight/internal/stats"
	"github.com/dartagnanli/agent-insight/internal/storage"
	"github.com/dartagnanli/agent-insight/web"
)

// Version 通过 -ldflags 注入，与 config.Version 保持一致
var Version = "0.1.0-dev"

// Server 仪表板 HTTP 服务
type Server struct {
	cfg        config.DashboardConfig
	storage    storage.Storage
	hub        *Hub
	poller     *Poller
	api        *APIHandler
	httpServer *http.Server
	cancel     context.CancelFunc
}

// NewServer 创建仪表板服务
func NewServer(cfg config.DashboardConfig, s storage.Storage) *Server {
	hub := NewHub()
	poller := NewPoller(s, hub)
	api := NewAPIHandler(s)

	return &Server{
		cfg:     cfg,
		storage: s,
		hub:     hub,
		poller:  poller,
		api:     api,
	}
}

// Start 启动 HTTP 服务和后台 goroutine
func (s *Server) Start(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)

	// 启动 poller
	go s.poller.Run(ctx)

	// 启动 stats flusher（5 分钟间隔）
	flusher := stats.NewFlusher(s.storage)
	go flusher.Start(ctx, 5*time.Minute)

	// 启动 session scanner（10 分钟间隔）
	scanner := session.NewScanner(s.storage)
	go scanner.Start(ctx, 10*time.Minute)

	// 构建 HTTP 路由
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := s.cfg.Host + ":" + formatPort(s.cfg.Port)
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("http://%s", addr)
	slog.Info("dashboard 正在启动", "addr", url)

	// 优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
		case <-ctx.Done():
		}
		s.Shutdown()
	}()

	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭服务
func (s *Server) Shutdown() {
	if s.cancel != nil {
		s.cancel()
	}
	s.hub.CloseAll()
	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}
}

// OpenBrowser 在默认浏览器中打开仪表板 URL
func (s *Server) OpenBrowser() {
	url := fmt.Sprintf("http://%s:%d", s.cfg.Host, s.cfg.Port)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		slog.Warn("不支持自动打开浏览器", "os", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Warn("打开浏览器失败", "error", err)
	}
}

// registerRoutes 注册所有路由
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// REST API
	mux.HandleFunc("GET /api/v1/events", s.api.HandleListEvents)
	mux.HandleFunc("GET /api/v1/events/{eventID}", s.api.HandleGetEvent)
	mux.HandleFunc("GET /api/v1/stats", s.api.HandleStats)
	mux.HandleFunc("GET /api/v1/stats/hourly", s.api.HandleStatsHourly)
	mux.HandleFunc("GET /api/v1/traces/{sessionID}", s.api.HandleTrace)
	mux.HandleFunc("GET /api/v1/sessions", s.api.HandleListSessions)
	mux.HandleFunc("GET /api/v1/version", s.api.HandleVersion)

	// WebSocket
	mux.HandleFunc("GET /api/v1/ws/events", s.HandleWebSocket)

	// 静态资源（go:embed）
	sub, err := fs.Sub(web.AssetsFS, "assets")
	if err != nil {
		slog.Error("failed to create sub filesystem", "error", err)
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("GET /css/", fileServer)
	mux.Handle("GET /js/", fileServer)

	// SPA 回退
	mux.HandleFunc("GET /", s.handleSPA)
	mux.HandleFunc("GET /dashboard", s.handleSPA)
	mux.HandleFunc("GET /events", s.handleSPA)
	mux.HandleFunc("GET /trace", s.handleSPA)
	mux.HandleFunc("GET /sessions", s.handleSPA)
}

// handleSPA 返回 index.html（单页应用入口）
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	data, err := web.AssetsFS.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func formatPort(port int) string {
	return fmt.Sprintf("%d", port)
}
