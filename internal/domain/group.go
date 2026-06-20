package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Ограничения на поля группы (согласованы с OpenAPI-контрактом).
const (
	groupNameMinLen = 1
	groupNameMaxLen = 255
)

// Group — узел дерева групп. Чистая доменная сущность без тегов БД/JSON.
type Group struct {
	ID        uuid.UUID
	ParentID  *uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewGroup конструирует новую группу, проверяя инварианты. ID и временные метки
// проставляются на уровне хранилища при сохранении.
func NewGroup(name string, parentID *uuid.UUID) (*Group, error) {
	g := &Group{
		Name:     strings.TrimSpace(name),
		ParentID: parentID,
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	return g, nil
}

// Validate проверяет инварианты группы и возвращает *ValidationError при
// нарушениях (совместим с errors.Is(err, ErrValidation)).
func (g *Group) Validate() error {
	var b validationBuilder
	name := strings.TrimSpace(g.Name)
	switch {
	case len(name) < groupNameMinLen:
		b.add("name", "must not be empty")
	case len(name) > groupNameMaxLen:
		b.add("name", "must be at most 255 characters")
	}
	return b.err()
}

// Rename меняет имя группы с проверкой инвариантов.
func (g *Group) Rename(name string) error {
	trimmed := strings.TrimSpace(name)
	candidate := &Group{Name: trimmed, ParentID: g.ParentID}
	if err := candidate.Validate(); err != nil {
		return err
	}
	g.Name = trimmed
	return nil
}
