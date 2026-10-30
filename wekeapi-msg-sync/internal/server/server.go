// Package server 提供 HTTP 服务：消息回调入口与 REST 查询 API。
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"wekeapi-msg-sync/internal/callback"
	"wekeapi-msg-sync/internal/config"
	"wekeapi-msg-sync/internal/store"
)

// Server HTTP 服务。
type Server struct {
	cfg       *config.Config
	st        *store.Store
	cb        *callback.Handler
	logger    *log.Logger
	startedAt time.Time
}

// New 创建 HTTP 服务。
func New(cfg *config.Config, st *store.Store, cb *callback.Handler, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{cfg: cfg, st: st, cb: cb, logger: logger, startedAt: time.Now()}
}

// Handler 返回带中间件的 HTTP 处理器（供 httptest 与 Run 复用）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /callback", s.handleCallback)
	mux.HandleFunc("GET /api/contacts", s.handleContacts)
	mux.HandleFunc("GET /api/messages", s.handleMessages)
	mux.HandleFunc("POST /api/messages", s.handleCreateMessage)
	mux.HandleFunc("GET /api/rooms", s.handleRooms)
	mux.HandleFunc("GET /api/sync-status", s.handleSyncStatus)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	return s.withMiddleware(mux)
}

// Run 启动 HTTP 服务：先监听端口，监听成功后回调 onReady 再开始服务，
// 保证启动全量同步期间不丢回调；ctx 取消后优雅关闭。监听失败不会触发 onReady。
func (s *Server) Run(ctx context.Context, onReady func()) error {
	ln, err := net.Listen("tcp", s.cfg.Addr())
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", s.cfg.Addr(), err)
	}
	if onReady != nil {
		onReady()
	}

	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		defer func() {
			if v := recover(); v != nil {
				s.logger.Printf("panic: %v", v)
				if !rec.wroteHeader {
					writeError(rec, http.StatusInternalServerError, CodeInternalError, "服务器内部错误")
				}
			}
			if r.URL.Path != "/healthz" {
				s.logger.Printf("%s %s -> %d（%s）", r.Method, r.URL.Path, rec.statusCode(),
					time.Since(start).Round(time.Millisecond))
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
