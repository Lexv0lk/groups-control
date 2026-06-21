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

// pgForeignKeyViolation — SQLSTATE-код нарушения внешнего ключа (23503).
// Используется вместо отдельной зависимости pgerrcode.
const pgForeignKeyViolation = "23503"

// Имена ограничений внешнего ключа, генерируемые PostgreSQL по умолчанию
// (см. migrations/000001_init.up.sql). По ним различаем причину нарушения.
const (
	fkGroupsParentID = "groups_parent_id_fkey"
	fkPeopleGroupID  = "people_group_id_fkey"
)

// GroupRepository — реализация usecase.GroupRepository на pgx/pgxpool.
// Маппит ошибки PostgreSQL в доменные (отсутствие строки → domain.ErrNotFound,
// нарушения внешнего ключа → доменные ошибки).
type GroupRepository struct {
	pool *pgxpool.Pool
}

// проверка соответствия порту на этапе компиляции.
var _ usecase.GroupRepository = (*GroupRepository)(nil)

// NewGroupRepository создаёт репозиторий групп поверх пула соединений.
func NewGroupRepository(pool *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{pool: pool}
}

// Create сохраняет новую группу, заполняя ID и временные метки из БД.
func (r *GroupRepository) Create(ctx context.Context, group *domain.Group) error {
	const query = `
		INSERT INTO groups (parent_id, name)
		VALUES ($1, $2)
		RETURNING id, parent_id, name, created_at, updated_at`

	row := r.pool.QueryRow(ctx, query, group.ParentID, group.Name)
	created, err := scanGroup(row)
	if err != nil {
		return mapGroupParentError(err)
	}
	*group = *created
	return nil
}

// Update сохраняет имя и parent_id существующей группы. Возвращает
// domain.ErrNotFound, если группа отсутствует.
func (r *GroupRepository) Update(ctx context.Context, group *domain.Group) error {
	const query = `
		UPDATE groups
		SET name = $2, parent_id = $3
		WHERE id = $1
		RETURNING id, parent_id, name, created_at, updated_at`

	row := r.pool.QueryRow(ctx, query, group.ID, group.Name, group.ParentID)
	updated, err := scanGroup(row)
	if err != nil {
		return mapGroupParentError(err)
	}
	*group = *updated
	return nil
}

// Delete удаляет группу по идентификатору. Возвращает domain.ErrNotFound, если
// группы нет; domain.ErrGroupHasChildren/ErrGroupHasPeople — если удаление
// блокируется внешними ключами (ON DELETE RESTRICT).
func (r *GroupRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM groups WHERE id = $1`

	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return mapGroupDeleteError(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GetByID возвращает группу по идентификатору или domain.ErrNotFound.
func (r *GroupRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Group, error) {
	const query = `
		SELECT id, parent_id, name, created_at, updated_at
		FROM groups
		WHERE id = $1`

	row := r.pool.QueryRow(ctx, query, id)
	group, err := scanGroup(row)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return group, nil
}

// List возвращает страницу групп с подсчётом участников (direct + total по
// поддереву) и общее число групп для пагинации. Счётчики поддерева считаются
// рекурсивным CTE одним запросом для всей страницы.
func (r *GroupRepository) List(ctx context.Context, page usecase.Pagination) ([]usecase.GroupWithCounts, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM groups`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count groups: %w", err)
	}
	if total == 0 {
		return []usecase.GroupWithCounts{}, 0, nil
	}

	const query = `
		WITH RECURSIVE page AS (
			SELECT id, parent_id, name, created_at, updated_at
			FROM groups
			ORDER BY created_at, id
			LIMIT $1 OFFSET $2
		),
		tree AS (
			SELECT id AS root, id AS node FROM page
			UNION ALL
			SELECT t.root, c.id
			FROM tree t
			JOIN groups c ON c.parent_id = t.node
		),
		total AS (
			SELECT t.root AS id, count(p.id) AS cnt
			FROM tree t
			LEFT JOIN people p ON p.group_id = t.node
			GROUP BY t.root
		),
		direct AS (
			SELECT g.id, count(p.id) AS cnt
			FROM page g
			LEFT JOIN people p ON p.group_id = g.id
			GROUP BY g.id
		)
		SELECT g.id, g.parent_id, g.name, g.created_at, g.updated_at,
		       COALESCE(direct.cnt, 0) AS direct_count,
		       COALESCE(total.cnt, 0) AS total_count
		FROM page g
		LEFT JOIN direct ON direct.id = g.id
		LEFT JOIN total ON total.id = g.id
		ORDER BY g.created_at, g.id`

	rows, err := r.pool.Query(ctx, query, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list groups: %w", err)
	}
	defer rows.Close()

	items := make([]usecase.GroupWithCounts, 0)
	for rows.Next() {
		var (
			g      domain.Group
			counts usecase.MemberCounts
		)
		if err := rows.Scan(
			&g.ID, &g.ParentID, &g.Name, &g.CreatedAt, &g.UpdatedAt,
			&counts.Direct, &counts.Total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan group row: %w", err)
		}
		items = append(items, usecase.GroupWithCounts{Group: g, Counts: counts})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate group rows: %w", err)
	}
	return items, total, nil
}

// CountMembers считает участников группы: только напрямую привязанных
// (includeDescendants=false) либо по всему поддереву (true, рекурсивный CTE).
func (r *GroupRepository) CountMembers(ctx context.Context, id uuid.UUID, includeDescendants bool) (int, error) {
	if includeDescendants {
		return r.countSubtreeMembers(ctx, id)
	}
	return r.countDirectMembers(ctx, id)
}

// countDirectMembers считает людей, привязанных непосредственно к группе.
func (r *GroupRepository) countDirectMembers(ctx context.Context, id uuid.UUID) (int, error) {
	const query = `SELECT count(*) FROM people WHERE group_id = $1`

	var count int
	if err := r.pool.QueryRow(ctx, query, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("count direct members: %w", err)
	}
	return count, nil
}

// countSubtreeMembers считает людей во всём поддереве группы через рекурсивный
// обход дерева групп.
func (r *GroupRepository) countSubtreeMembers(ctx context.Context, id uuid.UUID) (int, error) {
	const query = `
		WITH RECURSIVE subtree AS (
			SELECT id FROM groups WHERE id = $1
			UNION ALL
			SELECT g.id FROM groups g JOIN subtree s ON g.parent_id = s.id
		)
		SELECT count(p.id)
		FROM subtree
		LEFT JOIN people p ON p.group_id = subtree.id`

	var count int
	if err := r.pool.QueryRow(ctx, query, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("count subtree members: %w", err)
	}
	return count, nil
}

// IsDescendant сообщает, лежит ли candidateID в поддереве ancestorID (включая
// сам ancestorID). Используется для запрета циклов при смене parent_id.
func (r *GroupRepository) IsDescendant(ctx context.Context, ancestorID, candidateID uuid.UUID) (bool, error) {
	const query = `
		WITH RECURSIVE subtree AS (
			SELECT id FROM groups WHERE id = $1
			UNION ALL
			SELECT g.id FROM groups g JOIN subtree s ON g.parent_id = s.id
		)
		SELECT EXISTS (SELECT 1 FROM subtree WHERE id = $2)`

	var ok bool
	if err := r.pool.QueryRow(ctx, query, ancestorID, candidateID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check descendant: %w", err)
	}
	return ok, nil
}

// mapNoRows превращает pgx.ErrNoRows в domain.ErrNotFound, остальные ошибки
// возвращает как есть.
func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// mapGroupParentError обрабатывает ошибки create/update группы: отсутствие
// строки → domain.ErrNotFound, нарушение FK на parent_id (ссылка на
// несуществующую родительскую группу) → ошибка валидации поля parent_id.
func mapGroupParentError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == pgForeignKeyViolation &&
		pgErr.ConstraintName == fkGroupsParentID {
		return &domain.ValidationError{Fields: []domain.FieldError{
			{Field: "parent_id", Message: "referenced group does not exist"},
		}}
	}
	return err
}

// mapGroupDeleteError различает причину блокировки удаления по нарушенному
// внешнему ключу: дочерние группы либо привязанные люди.
func mapGroupDeleteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
		switch pgErr.ConstraintName {
		case fkGroupsParentID:
			return domain.ErrGroupHasChildren
		case fkPeopleGroupID:
			return domain.ErrGroupHasPeople
		}
	}
	return err
}
