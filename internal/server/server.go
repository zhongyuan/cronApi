// Package server 封装 HTTP Server 的启动与优雅关闭。
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"cronApi/internal/config"
	"cronApi/internal/logger"

	"go.uber.org/zap"
)

// Run 启动 HTTP 服务并阻塞直到收到 ctx.Done()。
func Run(ctx context.Context, cfg *config.Config, h http.Handler) error {
	srv := &http.Server{
		Addr:         cfg.App.Addr,
		Handler:      h,
		ReadTimeout:  cfg.App.ReadTimeout,
		WriteTimeout: cfg.App.WriteTimeout,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", zap.String("addr", cfg.App.Addr), zap.String("env", cfg.App.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()
	logger.Info("http server shutting down")
	return srv.Shutdown(shutdownCtx)
}
