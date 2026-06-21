package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"log/slog"
)

// Config — параметры HTTP-сервера, необходимые для его запуска и остановки.
type Config struct {
	// Addr — адрес прослушивания в формате host:port.
	Addr string
	// ReadTimeout — таймаут чтения запроса целиком.
	ReadTimeout time.Duration
	// WriteTimeout — таймаут записи ответа.
	WriteTimeout time.Duration
	// IdleTimeout — таймаут простоя keep-alive соединения.
	IdleTimeout time.Duration
	// ShutdownTimeout — отведённое время на graceful shutdown.
	ShutdownTimeout time.Duration
}

// Server инкапсулирует *http.Server вместе с параметрами завершения.
type Server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

// New создаёт сервер поверх готового http.Handler (роутера). Сам handler
// конструируется в composition root и передаётся сюда как зависимость.
func New(cfg Config, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
		logger:          logger,
	}
}

// Run запускает сервер и блокируется до отмены ctx, после чего выполняет
// graceful shutdown в пределах ShutdownTimeout. Возвращает ошибку, если сервер
// упал не по причине штатного закрытия либо shutdown не успел завершиться.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", slog.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutdown signal received, stopping http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		s.logger.Info("http server stopped gracefully")
		return nil
	}
}
