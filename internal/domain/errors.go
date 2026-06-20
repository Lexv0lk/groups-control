package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Доменные (sentinel) ошибки. Внешние слои сопоставляют их с транспортными
// кодами (например, HTTP-статусами), не завязываясь на конкретные сообщения.
var (
	// ErrNotFound — запрашиваемый ресурс не существует.
	ErrNotFound = errors.New("resource not found")
	// ErrGroupHasChildren — попытка удалить группу, у которой есть дочерние.
	ErrGroupHasChildren = errors.New("group has child groups")
	// ErrGroupHasPeople — попытка удалить группу, к которой привязаны люди.
	ErrGroupHasPeople = errors.New("group has people")
	// ErrCyclicParent — назначение родителя создало бы цикл в дереве групп.
	ErrCyclicParent = errors.New("group cannot be its own ancestor")
	// ErrValidation — нарушены инварианты сущности. Обычно представлена
	// значением *ValidationError с детализацией по полям.
	ErrValidation = errors.New("validation failed")
)

// FieldError — одна ошибка валидации, привязанная к конкретному полю.
type FieldError struct {
	Field   string
	Message string
}

// ValidationError агрегирует ошибки валидации по нескольким полям сущности.
// Сопоставляется с ErrValidation через errors.Is.
type ValidationError struct {
	Fields []FieldError
}

// Error реализует интерфейс error.
func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return ErrValidation.Error()
	}
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}
	return fmt.Sprintf("%s: %s", ErrValidation.Error(), strings.Join(parts, "; "))
}

// Is позволяет errors.Is(err, ErrValidation) распознавать *ValidationError.
func (e *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// validationBuilder накапливает ошибки полей и возвращает агрегированную ошибку.
type validationBuilder struct {
	fields []FieldError
}

// add регистрирует ошибку для указанного поля.
func (b *validationBuilder) add(field, message string) {
	b.fields = append(b.fields, FieldError{Field: field, Message: message})
}

// err возвращает *ValidationError, если были зарегистрированы ошибки, иначе nil.
func (b *validationBuilder) err() error {
	if len(b.fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: b.fields}
}
