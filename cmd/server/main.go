package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"log/slog"

	httpapi "groups-control/internal/adapters/http"
	"groups-control/internal/adapters/repository/postgres"
	"groups-control/internal/config"
	"groups-control/internal/server"
	"groups-control/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped with error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg.Log.Level)
	slog.SetDefault(logger)

	// Контекст приложения, отменяемый по SIGINT/SIGTERM, — единый сигнал для
	// остановки сервера и освобождения ресурсов.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{
		DSN:             cfg.DB.DSN,
		MaxConns:        cfg.DB.MaxConns,
		MinConns:        cfg.DB.MinConns,
		MaxConnLifetime: cfg.DB.MaxConnLifetime,
		MaxConnIdleTime: cfg.DB.MaxConnIdleTime,
		ConnectTimeout:  cfg.DB.ConnectTimeout,
	})
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	// Репозитории (адаптеры БД) → usecase-сервисы → HTTP-обработчик.
	// Зависимости направлены внутрь: сервисы знают только интерфейсы.
	groupRepo := postgres.NewGroupRepository(pool)
	personRepo := postgres.NewPersonRepository(pool)

	groupService := usecase.NewGroupService(groupRepo)
	personService := usecase.NewPersonService(personRepo, groupRepo)

	handler := httpapi.NewHandler(groupService, personService, pool)
	router := httpapi.NewRouter(handler, logger, cfg.HTTP.RequestTimeout)

	srv := server.New(server.Config{
		Addr:            cfg.HTTP.Addr(),
		ReadTimeout:     cfg.HTTP.ReadTimeout,
		WriteTimeout:    cfg.HTTP.WriteTimeout,
		IdleTimeout:     cfg.HTTP.IdleTimeout,
		ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
	}, router, logger)

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("run server: %w", err)
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}
