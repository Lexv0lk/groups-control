package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config — корневая конфигурация приложения, собранная из env-переменных.
type Config struct {
	HTTP HTTP
	DB   DB
	Log  Log
}

// HTTP — параметры HTTP-сервера.
type HTTP struct {
	// Host — адрес, на котором слушает сервер.
	Host string `env:"HTTP_HOST" envDefault:"0.0.0.0"`
	// Port — TCP-порт сервера.
	Port string `env:"HTTP_PORT" envDefault:"8080"`
	// ReadTimeout — таймаут чтения запроса целиком.
	ReadTimeout time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	// WriteTimeout — таймаут записи ответа.
	WriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
	// IdleTimeout — таймаут простоя keep-alive соединения.
	IdleTimeout time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
	// ShutdownTimeout — отведённое время на graceful shutdown.
	ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"15s"`
}

// Addr возвращает адрес для http.Server в формате host:port.
func (h HTTP) Addr() string {
	return h.Host + ":" + h.Port
}

// DB — параметры подключения к PostgreSQL и пула соединений.
type DB struct {
	// DSN — строка подключения (postgres://user:pass@host:port/db?sslmode=...).
	DSN string `env:"DB_DSN,required"`
	// MaxConns — максимальный размер пула соединений.
	MaxConns int32 `env:"DB_MAX_CONNS" envDefault:"10"`
	// MinConns — минимальное число поддерживаемых соединений.
	MinConns int32 `env:"DB_MIN_CONNS" envDefault:"2"`
	// MaxConnLifetime — максимальное время жизни соединения.
	MaxConnLifetime time.Duration `env:"DB_MAX_CONN_LIFETIME" envDefault:"1h"`
	// MaxConnIdleTime — максимальное время простоя соединения до закрытия.
	MaxConnIdleTime time.Duration `env:"DB_MAX_CONN_IDLE_TIME" envDefault:"30m"`
	// ConnectTimeout — таймаут установки соединения и проверки доступности (Ping).
	ConnectTimeout time.Duration `env:"DB_CONNECT_TIMEOUT" envDefault:"5s"`
}

// Log — параметры логирования.
type Log struct {
	// Level — уровень логирования: debug | info | warn | error.
	Level string `env:"LOG_LEVEL" envDefault:"info"`
}

// Load читает конфигурацию из переменных окружения, применяя значения по
// умолчанию. Возвращает ошибку, если обязательные переменные не заданы.
func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
