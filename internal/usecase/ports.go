package usecase

import (
	"context"

	"github.com/google/uuid"

	"groups-control/internal/domain"
)

// Pagination описывает параметры постраничной выборки списков.
type Pagination struct {
	// Limit — максимальное число элементов на странице.
	Limit int
	// Offset — смещение от начала выборки.
	Offset int
}

// MemberCounts — количество участников группы в двух режимах.
type MemberCounts struct {
	// Direct — люди, привязанные непосредственно к группе.
	Direct int
	// Total — люди группы вместе со всеми дочерними (поддерево).
	Total int
}

// GroupWithCounts — группа вместе с подсчётом участников. Read-модель для
// списков групп; формируется репозиторием одним/двумя запросами.
type GroupWithCounts struct {
	Group  domain.Group
	Counts MemberCounts
}

// GroupRepository — порт доступа к хранилищу групп. Реализация маппит ошибки
// хранилища в доменные (например, отсутствие строки → domain.ErrNotFound).
type GroupRepository interface {
	// Create сохраняет новую группу, проставляя ID и временные метки в
	// переданной сущности.
	Create(ctx context.Context, group *domain.Group) error
	// Update сохраняет изменения существующей группы (имя, parent_id).
	// Возвращает domain.ErrNotFound, если группа отсутствует.
	Update(ctx context.Context, group *domain.Group) error
	// Delete удаляет группу по идентификатору. Возвращает domain.ErrNotFound,
	// если группа отсутствует.
	Delete(ctx context.Context, id uuid.UUID) error
	// GetByID возвращает группу по идентификатору или domain.ErrNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Group, error)
	// List возвращает страницу групп с подсчётом участников и общее количество
	// групп (для пагинации).
	List(ctx context.Context, page Pagination) (items []GroupWithCounts, total int, err error)
	// CountMembers считает участников группы: только привязанных напрямую
	// (includeDescendants=false) либо вместе со всем поддеревом (true).
	CountMembers(ctx context.Context, id uuid.UUID, includeDescendants bool) (int, error)
	// IsDescendant сообщает, является ли candidateID потомком ancestorID (или
	// самим ancestorID). Используется для запрета циклов при смене parent_id.
	IsDescendant(ctx context.Context, ancestorID, candidateID uuid.UUID) (bool, error)
}

// PersonRepository — порт доступа к хранилищу людей.
type PersonRepository interface {
	// Create сохраняет нового человека, проставляя ID и временные метки в
	// переданной сущности.
	Create(ctx context.Context, person *domain.Person) error
	// Update сохраняет изменения человека (в т.ч. смену группы). Возвращает
	// domain.ErrNotFound, если человек отсутствует.
	Update(ctx context.Context, person *domain.Person) error
	// Delete удаляет человека по идентификатору. Возвращает domain.ErrNotFound,
	// если человек отсутствует.
	Delete(ctx context.Context, id uuid.UUID) error
	// GetByID возвращает человека по идентификатору или domain.ErrNotFound.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Person, error)
	// ListByGroup возвращает страницу людей группы: только привязанных напрямую
	// (includeDescendants=false) либо вместе со всем поддеревом (true), а также
	// общее количество (для пагинации).
	ListByGroup(ctx context.Context, groupID uuid.UUID, includeDescendants bool, page Pagination) (items []domain.Person, total int, err error)
}
