package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"groups-control/internal/domain"
	"groups-control/internal/usecase"
)

// PersonRepository — реализация usecase.PersonRepository на pgx/pgxpool.
// Маппит ошибки PostgreSQL в доменные (отсутствие строки → domain.ErrNotFound,
// нарушение FK на group_id → ошибка валидации поля group_id).
type PersonRepository struct {
	pool *pgxpool.Pool
}

// проверка соответствия порту на этапе компиляции.
var _ usecase.PersonRepository = (*PersonRepository)(nil)

// NewPersonRepository создаёт репозиторий людей поверх пула соединений.
func NewPersonRepository(pool *pgxpool.Pool) *PersonRepository {
	return &PersonRepository{pool: pool}
}

// Create сохраняет нового человека, заполняя ID и временные метки из БД.
func (r *PersonRepository) Create(ctx context.Context, person *domain.Person) error {
	const query = `
		INSERT INTO people (first_name, last_name, birth_year, group_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, first_name, last_name, birth_year, group_id, created_at, updated_at`

	row := r.pool.QueryRow(ctx, query,
		person.FirstName, person.LastName, person.BirthYear, person.GroupID)
	created, err := scanPerson(row)
	if err != nil {
		return mapPersonGroupError(err)
	}
	*person = *created
	return nil
}

// Update сохраняет изменения человека (в т.ч. смену группы). Возвращает
// domain.ErrNotFound, если человек отсутствует.
func (r *PersonRepository) Update(ctx context.Context, person *domain.Person) error {
	const query = `
		UPDATE people
		SET first_name = $2, last_name = $3, birth_year = $4, group_id = $5
		WHERE id = $1
		RETURNING id, first_name, last_name, birth_year, group_id, created_at, updated_at`

	row := r.pool.QueryRow(ctx, query,
		person.ID, person.FirstName, person.LastName, person.BirthYear, person.GroupID)
	updated, err := scanPerson(row)
	if err != nil {
		return mapPersonGroupError(err)
	}
	*person = *updated
	return nil
}

// Delete удаляет человека по идентификатору. Возвращает domain.ErrNotFound,
// если человека нет.
func (r *PersonRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM people WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete person: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GetByID возвращает человека по идентификатору или domain.ErrNotFound.
func (r *PersonRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Person, error) {
	const query = `
		SELECT id, first_name, last_name, birth_year, group_id, created_at, updated_at
		FROM people
		WHERE id = $1`

	row := r.pool.QueryRow(ctx, query, id)
	person, err := scanPerson(row)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return person, nil
}

// ListByGroup возвращает страницу людей группы: только привязанных напрямую
// (includeDescendants=false) либо вместе со всем поддеревом (true, рекурсивный
// CTE по дереву групп), а также общее число людей для пагинации.
func (r *PersonRepository) ListByGroup(ctx context.Context, groupID uuid.UUID, includeDescendants bool, page usecase.Pagination) ([]domain.Person, int, error) {
	if includeDescendants {
		return r.listSubtree(ctx, groupID, page)
	}
	return r.listDirect(ctx, groupID, page)
}

// listDirect выбирает людей, привязанных непосредственно к группе.
func (r *PersonRepository) listDirect(ctx context.Context, groupID uuid.UUID, page usecase.Pagination) ([]domain.Person, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM people WHERE group_id = $1`, groupID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count people by group: %w", err)
	}
	if total == 0 {
		return []domain.Person{}, 0, nil
	}

	const query = `
		SELECT id, first_name, last_name, birth_year, group_id, created_at, updated_at
		FROM people
		WHERE group_id = $1
		ORDER BY created_at, id
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, groupID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list people by group: %w", err)
	}
	defer rows.Close()

	items, err := collectPeople(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// listSubtree выбирает людей всего поддерева группы через рекурсивный обход
// дерева групп.
func (r *PersonRepository) listSubtree(ctx context.Context, groupID uuid.UUID, page usecase.Pagination) ([]domain.Person, int, error) {
	const countQuery = `
		WITH RECURSIVE subtree AS (
			SELECT id FROM groups WHERE id = $1
			UNION ALL
			SELECT g.id FROM groups g JOIN subtree s ON g.parent_id = s.id
		)
		SELECT count(p.id)
		FROM subtree
		JOIN people p ON p.group_id = subtree.id`

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, groupID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count subtree people: %w", err)
	}
	if total == 0 {
		return []domain.Person{}, 0, nil
	}

	const query = `
		WITH RECURSIVE subtree AS (
			SELECT id FROM groups WHERE id = $1
			UNION ALL
			SELECT g.id FROM groups g JOIN subtree s ON g.parent_id = s.id
		)
		SELECT p.id, p.first_name, p.last_name, p.birth_year, p.group_id,
		       p.created_at, p.updated_at
		FROM people p
		JOIN subtree ON p.group_id = subtree.id
		ORDER BY p.created_at, p.id
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, groupID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list subtree people: %w", err)
	}
	defer rows.Close()

	items, err := collectPeople(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// mapPersonGroupError обрабатывает ошибки create/update человека: отсутствие
// строки → domain.ErrNotFound, нарушение FK на group_id (ссылка на
// несуществующую группу) → ошибка валидации поля group_id.
func mapPersonGroupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == pgForeignKeyViolation &&
		pgErr.ConstraintName == fkPeopleGroupID {
		return &domain.ValidationError{Fields: []domain.FieldError{
			{Field: "group_id", Message: "referenced group does not exist"},
		}}
	}
	return err
}

// collectPeople читает все строки результата в срез доменных сущностей Person.
func collectPeople(rows pgx.Rows) ([]domain.Person, error) {
	items := make([]domain.Person, 0)
	for rows.Next() {
		p, err := scanPerson(rows)
		if err != nil {
			return nil, fmt.Errorf("scan person row: %w", err)
		}
		items = append(items, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate person rows: %w", err)
	}
	return items, nil
}
