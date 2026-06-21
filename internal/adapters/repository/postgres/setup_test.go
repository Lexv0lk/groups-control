//go:build integration

// Пакетный тестовый харнесс для интеграционных тестов репозиториев. Поднимает
// реальный PostgreSQL в Docker через testcontainers-go, применяет миграцию
// схемы и предоставляет вспомогательные функции для тестов.
//
// Запуск: go test -tags integration ./internal/adapters/repository/postgres/...
// Требуется доступный Docker-демон.
package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPool — общий для всех тестов пакета пул соединений к контейнеру.
var testPool *pgxpool.Pool

// migrationsDir — путь к каталогу миграций относительно пакета.
const migrationsDir = "../../../../migrations"

// TestMain поднимает контейнер PostgreSQL один раз на пакет, применяет схему и
// гарантирует очистку ресурсов.
func TestMain(m *testing.M) {
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

	os.Exit(m.Run())
}

// resetDB очищает таблицы перед тестом и регистрирует повторную очистку после.
func resetDB(t *testing.T) {
	t.Helper()
	truncate := func() {
		_, err := testPool.Exec(context.Background(), `TRUNCATE people, groups CASCADE`)
		require.NoError(t, err)
	}
	truncate()
	t.Cleanup(truncate)
}

// loadSeed применяет миграцию с демонстрационными данными.
func loadSeed(t *testing.T) {
	t.Helper()
	err := applySQLFile(context.Background(), testPool, filepath.Join(migrationsDir, "000002_seed.up.sql"))
	require.NoError(t, err)
}

// applySQLFile выполняет содержимое SQL-файла целиком. Запрос без аргументов
// отправляется простым протоколом, что допускает несколько стейтментов и
// dollar-quoted тела функций/триггеров.
func applySQLFile(ctx context.Context, pool *pgxpool.Pool, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(content))
	return err
}
