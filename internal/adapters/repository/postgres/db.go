package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"groups-control/internal/domain"
)

// PoolConfig — параметры пула соединений с PostgreSQL. Заполняется в
// composition root из конфигурации приложения.
type PoolConfig struct {
	// DSN — строка подключения к базе.
	DSN string
	// MaxConns — максимальный размер пула.
	MaxConns int32
	// MinConns — минимальное число поддерживаемых соединений.
	MinConns int32
	// MaxConnLifetime — максимальное время жизни соединения.
	MaxConnLifetime time.Duration
	// MaxConnIdleTime — максимальное время простоя соединения.
	MaxConnIdleTime time.Duration
	// ConnectTimeout — таймаут установки соединения и проверки доступности.
	ConnectTimeout time.Duration
}

// NewPool создаёт пул соединений pgxpool по переданной конфигурации и проверяет
// доступность базы (Ping). При ошибке пул закрывается, чтобы не оставлять
// «висящих» соединений. Вызывающая сторона обязана вызвать Close у успешно
// созданного пула.
func NewPool(ctx context.Context, cfg PoolConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}

	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx := ctx
	if cfg.ConnectTimeout > 0 {
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(ctx, cfg.ConnectTimeout)
		defer cancel()
	}
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

// rowScanner абстрагирует pgx.Row и pgx.Rows, позволяя переиспользовать
// функции маппинга как для одиночных строк, так и при итерации по результату.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanGroup читает одну строку результата в доменную сущность Group.
// Порядок столбцов: id, parent_id, name, created_at, updated_at.
func scanGroup(row rowScanner) (*domain.Group, error) {
	var g domain.Group
	if err := row.Scan(&g.ID, &g.ParentID, &g.Name, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, err
	}
	return &g, nil
}

// scanPerson читает одну строку результата в доменную сущность Person.
// Порядок столбцов: id, first_name, last_name, birth_year, group_id,
// created_at, updated_at.
func scanPerson(row rowScanner) (*domain.Person, error) {
	var p domain.Person
	if err := row.Scan(
		&p.ID,
		&p.FirstName,
		&p.LastName,
		&p.BirthYear,
		&p.GroupID,
		&p.CreatedAt,
		&p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}
