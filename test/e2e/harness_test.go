//go:build e2e

// Пакет e2e содержит сквозные тесты REST-API сервиса управления группами.
// Тесты поднимают весь стек в памяти процесса: реальный PostgreSQL в Docker
// (через testcontainers-go), реальные репозитории, usecase-сервисы и HTTP-слой,
// смонтированный в httptest.Server. Запросы идут по настоящему HTTP — так
// проверяется вся цепочка: маршрутизация, разбор контракта, бизнес-логика,
// рекурсивные запросы к БД и маппинг ошибок в HTTP-коды.
//
// Запуск: go test -tags e2e ./test/e2e/...
// Требуется доступный Docker-демон.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	httpapi "groups-control/internal/adapters/http"
	"groups-control/internal/adapters/repository/postgres"
	"groups-control/internal/usecase"
)

// migrationsDir — путь к каталогу миграций относительно пакета.
const migrationsDir = "../../migrations"

var (
	// baseURL — адрес поднятого на время тестов HTTP-сервера.
	baseURL string
	// testPool — пул соединений к контейнеру PostgreSQL, общий на пакет.
	testPool *pgxpool.Pool
)

// TestMain собирает весь стек один раз на пакет: контейнер PostgreSQL, схема,
// репозитории → usecase → HTTP-роутер → httptest.Server. os.Exit пропускает
// отложенные вызовы, поэтому очистка вынесена в run.
func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("groups"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		panic("start postgres container: " + err.Error())
	}
	defer func() { _ = testcontainers.TerminateContainer(ctr) }()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("container connection string: " + err.Error())
	}

	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		panic("create pool: " + err.Error())
	}
	defer testPool.Close()

	if err := applySQLFile(ctx, testPool, filepath.Join(migrationsDir, "000001_init.up.sql")); err != nil {
		panic("apply schema migration: " + err.Error())
	}

	// Та же сборка зависимостей, что и в composition root (cmd/server), но поверх
	// тестового пула: репозитории → сервисы → HTTP-обработчик → роутер.
	groupRepo := postgres.NewGroupRepository(testPool)
	personRepo := postgres.NewPersonRepository(testPool)
	groupService := usecase.NewGroupService(groupRepo)
	personService := usecase.NewPersonService(personRepo, groupRepo)

	handler := httpapi.NewHandler(groupService, personService, testPool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpapi.NewRouter(handler, logger, 5*time.Second)

	srv := httptest.NewServer(router)
	defer srv.Close()
	baseURL = srv.URL

	return m.Run()
}

// resetDB очищает таблицы перед тестом и регистрирует повторную очистку после,
// обеспечивая изоляцию сценариев друг от друга.
func resetDB(t *testing.T) {
	t.Helper()
	truncate := func() {
		_, err := testPool.Exec(context.Background(), `TRUNCATE people, groups CASCADE`)
		require.NoError(t, err)
	}
	truncate()
	t.Cleanup(truncate)
}

// applySQLFile выполняет содержимое SQL-файла целиком простым протоколом,
// допускающим несколько стейтментов и dollar-quoted тела функций/триггеров.
func applySQLFile(ctx context.Context, pool *pgxpool.Pool, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(content))
	return err
}

// doRequest выполняет HTTP-запрос к тестовому серверу. body, если не nil,
// сериализуется в JSON. Возвращает статус ответа и сырое тело.
func doRequest(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, baseURL+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, data
}

// doRequestHeaders — как doRequest, но дополнительно возвращает заголовки ответа.
// Нужен для проверок, завязанных на заголовки (например, Allow в ответе 405).
func doRequestHeaders(t *testing.T, method, path string, body any) (int, http.Header, []byte) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, baseURL+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, data
}

// decode разбирает JSON-тело ответа в значение типа T, проваливая тест при
// ошибке десериализации.
func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(data, &out), "unmarshal response: %s", string(data))
	return out
}
