package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"groups-control/internal/domain"
)

// GroupService реализует бизнес-логику работы с группами. Зависит только от
// порта GroupRepository (DIP) — конкретная реализация инжектится в main.
type GroupService struct {
	repo GroupRepository
}

// NewGroupService создаёт сервис групп с указанным репозиторием.
func NewGroupService(repo GroupRepository) *GroupService {
	return &GroupService{repo: repo}
}

// Create создаёт новую группу. Если задан parentID, проверяет существование
// родительской группы. Возвращает *domain.ValidationError при нарушении
// инвариантов или ссылке на несуществующего родителя.
func (s *GroupService) Create(ctx context.Context, name string, parentID *uuid.UUID) (*domain.Group, error) {
	if parentID != nil {
		if err := s.ensureParentExists(ctx, *parentID); err != nil {
			return nil, err
		}
	}

	group, err := domain.NewGroup(name, parentID)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, group); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return group, nil
}

// Update изменяет имя и/или родителя существующей группы. Запрещает циклы в
// дереве: новая родительская группа не может находиться в поддереве изменяемой
// группы (и не может совпадать с ней самой).
func (s *GroupService) Update(ctx context.Context, id uuid.UUID, name string, parentID *uuid.UUID) (*domain.Group, error) {
	group, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if parentID != nil {
		if err := s.validateNewParent(ctx, id, *parentID); err != nil {
			return nil, err
		}
	}

	if err := group.Rename(name); err != nil {
		return nil, err
	}
	group.ParentID = parentID

	if err := s.repo.Update(ctx, group); err != nil {
		return nil, fmt.Errorf("update group: %w", err)
	}
	return group, nil
}

// Delete удаляет группу. По умолчанию запрещает удаление, если к группе
// привязаны люди (domain.ErrGroupHasPeople); наличие дочерних групп
// контролируется репозиторием (domain.ErrGroupHasChildren).
func (s *GroupService) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}

	direct, err := s.repo.CountMembers(ctx, id, false)
	if err != nil {
		return fmt.Errorf("count group members: %w", err)
	}
	if direct > 0 {
		return domain.ErrGroupHasPeople
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	return nil
}

// Get возвращает группу по идентификатору или domain.ErrNotFound.
func (s *GroupService) Get(ctx context.Context, id uuid.UUID) (*domain.Group, error) {
	return s.repo.GetByID(ctx, id)
}

// List возвращает страницу групп с подсчётом участников и общее число групп.
func (s *GroupService) List(ctx context.Context, page Pagination) ([]GroupWithCounts, int, error) {
	return s.repo.List(ctx, page)
}

// Counts возвращает количество участников группы: напрямую привязанных и вместе
// со всем поддеревом. Возвращает domain.ErrNotFound, если группа отсутствует.
func (s *GroupService) Counts(ctx context.Context, id uuid.UUID) (MemberCounts, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return MemberCounts{}, err
	}

	direct, err := s.repo.CountMembers(ctx, id, false)
	if err != nil {
		return MemberCounts{}, fmt.Errorf("count direct members: %w", err)
	}
	total, err := s.repo.CountMembers(ctx, id, true)
	if err != nil {
		return MemberCounts{}, fmt.Errorf("count total members: %w", err)
	}
	return MemberCounts{Direct: direct, Total: total}, nil
}

// ensureParentExists проверяет наличие родительской группы, превращая
// отсутствие строки в ошибку валидации поля parent_id.
func (s *GroupService) ensureParentExists(ctx context.Context, parentID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, parentID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return parentNotFoundError()
		}
		return fmt.Errorf("lookup parent group: %w", err)
	}
	return nil
}

// validateNewParent проверяет, что новый родитель существует и не создаёт цикл
// (не совпадает с самой группой и не лежит в её поддереве).
func (s *GroupService) validateNewParent(ctx context.Context, id, parentID uuid.UUID) error {
	if parentID == id {
		return domain.ErrCyclicParent
	}
	if err := s.ensureParentExists(ctx, parentID); err != nil {
		return err
	}

	isDescendant, err := s.repo.IsDescendant(ctx, id, parentID)
	if err != nil {
		return fmt.Errorf("check group ancestry: %w", err)
	}
	if isDescendant {
		return domain.ErrCyclicParent
	}
	return nil
}

// parentNotFoundError формирует ошибку валидации для ссылки на несуществующую
// родительскую группу.
func parentNotFoundError() error {
	return &domain.ValidationError{
		Fields: []domain.FieldError{
			{Field: "parent_id", Message: "referenced group does not exist"},
		},
	}
}
