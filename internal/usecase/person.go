package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"groups-control/internal/domain"
)

// PersonService реализует бизнес-логику работы с людьми. Зависит от порта
// PersonRepository, а также от GroupRepository — для проверки существования
// группы при создании человека и смене группы (DIP). Конкретные реализации
// инжектятся в main.
type PersonService struct {
	people PersonRepository
	groups GroupRepository
}

// NewPersonService создаёт сервис людей с указанными репозиториями.
func NewPersonService(people PersonRepository, groups GroupRepository) *PersonService {
	return &PersonService{people: people, groups: groups}
}

// Create создаёт нового человека в указанной группе. Группа обязательна и должна
// существовать. Возвращает *domain.ValidationError при нарушении инвариантов или
// ссылке на несуществующую группу.
func (s *PersonService) Create(ctx context.Context, firstName, lastName string, birthYear int, groupID uuid.UUID) (*domain.Person, error) {
	if err := s.ensureGroupExists(ctx, groupID); err != nil {
		return nil, err
	}

	person, err := domain.NewPerson(firstName, lastName, birthYear, groupID)
	if err != nil {
		return nil, err
	}

	if err := s.people.Create(ctx, person); err != nil {
		return nil, fmt.Errorf("create person: %w", err)
	}
	return person, nil
}

// Update изменяет данные человека, в том числе позволяет сменить группу. Целевая
// группа должна существовать. Возвращает domain.ErrNotFound, если человек
// отсутствует.
func (s *PersonService) Update(ctx context.Context, id uuid.UUID, firstName, lastName string, birthYear int, groupID uuid.UUID) (*domain.Person, error) {
	person, err := s.people.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if groupID != person.GroupID {
		if err := s.ensureGroupExists(ctx, groupID); err != nil {
			return nil, err
		}
	}

	updated, err := domain.NewPerson(firstName, lastName, birthYear, groupID)
	if err != nil {
		return nil, err
	}
	person.FirstName = updated.FirstName
	person.LastName = updated.LastName
	person.BirthYear = updated.BirthYear
	person.GroupID = updated.GroupID

	if err := s.people.Update(ctx, person); err != nil {
		return nil, fmt.Errorf("update person: %w", err)
	}
	return person, nil
}

// Delete удаляет человека по идентификатору или возвращает domain.ErrNotFound.
func (s *PersonService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.people.Delete(ctx, id); err != nil {
		return err
	}
	return nil
}

// Get возвращает человека по идентификатору или domain.ErrNotFound.
func (s *PersonService) Get(ctx context.Context, id uuid.UUID) (*domain.Person, error) {
	return s.people.GetByID(ctx, id)
}

// ListByGroup возвращает страницу людей группы: только привязанных напрямую
// (includeDescendants=false) либо вместе со всем поддеревом (true), а также
// общее количество для пагинации. Проверяет существование группы.
func (s *PersonService) ListByGroup(ctx context.Context, groupID uuid.UUID, includeDescendants bool, page Pagination) ([]domain.Person, int, error) {
	if _, err := s.groups.GetByID(ctx, groupID); err != nil {
		return nil, 0, err
	}
	return s.people.ListByGroup(ctx, groupID, includeDescendants, page)
}

// ensureGroupExists проверяет наличие группы, превращая отсутствие строки в
// ошибку валидации поля group_id.
func (s *PersonService) ensureGroupExists(ctx context.Context, groupID uuid.UUID) error {
	if _, err := s.groups.GetByID(ctx, groupID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return groupNotFoundError()
		}
		return fmt.Errorf("lookup group: %w", err)
	}
	return nil
}

// groupNotFoundError формирует ошибку валидации для ссылки на несуществующую
// группу человека.
func groupNotFoundError() error {
	return &domain.ValidationError{
		Fields: []domain.FieldError{
			{Field: "group_id", Message: "referenced group does not exist"},
		},
	}
}
