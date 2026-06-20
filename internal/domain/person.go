package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Ограничения на поля человека.
const (
	personNameMinLen  = 1
	personNameMaxLen  = 255
	personBirthYearLo = 1900
	personBirthYearHi = 2100
)

// Person — человек, привязанный к группе. Чистая доменная сущность без тегов
// БД/JSON.
type Person struct {
	ID        uuid.UUID
	FirstName string
	LastName  string
	BirthYear int
	GroupID   uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewPerson конструирует нового человека, проверяя инварианты. ID и временные
// метки проставляются на уровне хранилища при сохранении.
func NewPerson(firstName, lastName string, birthYear int, groupID uuid.UUID) (*Person, error) {
	p := &Person{
		FirstName: strings.TrimSpace(firstName),
		LastName:  strings.TrimSpace(lastName),
		BirthYear: birthYear,
		GroupID:   groupID,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// Validate проверяет инварианты человека и возвращает *ValidationError при
// нарушениях (совместим с errors.Is(err, ErrValidation)).
func (p *Person) Validate() error {
	var b validationBuilder

	first := strings.TrimSpace(p.FirstName)
	switch {
	case len(first) < personNameMinLen:
		b.add("first_name", "must not be empty")
	case len(first) > personNameMaxLen:
		b.add("first_name", "must be at most 255 characters")
	}

	last := strings.TrimSpace(p.LastName)
	switch {
	case len(last) < personNameMinLen:
		b.add("last_name", "must not be empty")
	case len(last) > personNameMaxLen:
		b.add("last_name", "must be at most 255 characters")
	}

	if p.BirthYear < personBirthYearLo || p.BirthYear > personBirthYearHi {
		b.add("birth_year", "must be between 1900 and 2100")
	}

	if p.GroupID == uuid.Nil {
		b.add("group_id", "must reference an existing group")
	}

	return b.err()
}
