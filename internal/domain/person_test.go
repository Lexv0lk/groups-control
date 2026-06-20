package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewPerson(t *testing.T) {
	group := uuid.New()

	tests := []struct {
		name      string
		first     string
		last      string
		birthYear int
		groupID   uuid.UUID
		wantErr   bool
		wantField string
	}{
		{
			name:      "valid person",
			first:     "Ada",
			last:      "Lovelace",
			birthYear: 1990,
			groupID:   group,
		},
		{
			name:      "trims names",
			first:     "  Ada  ",
			last:      "  Lovelace  ",
			birthYear: 2000,
			groupID:   group,
		},
		{
			name:      "empty first name",
			first:     "",
			last:      "Lovelace",
			birthYear: 1990,
			groupID:   group,
			wantErr:   true,
			wantField: "first_name",
		},
		{
			name:      "empty last name",
			first:     "Ada",
			last:      "   ",
			birthYear: 1990,
			groupID:   group,
			wantErr:   true,
			wantField: "last_name",
		},
		{
			name:      "first name too long",
			first:     strings.Repeat("a", personNameMaxLen+1),
			last:      "Lovelace",
			birthYear: 1990,
			groupID:   group,
			wantErr:   true,
			wantField: "first_name",
		},
		{
			name:      "birth year below range",
			first:     "Ada",
			last:      "Lovelace",
			birthYear: personBirthYearLo - 1,
			groupID:   group,
			wantErr:   true,
			wantField: "birth_year",
		},
		{
			name:      "birth year above range",
			first:     "Ada",
			last:      "Lovelace",
			birthYear: personBirthYearHi + 1,
			groupID:   group,
			wantErr:   true,
			wantField: "birth_year",
		},
		{
			name:      "missing group",
			first:     "Ada",
			last:      "Lovelace",
			birthYear: 1990,
			groupID:   uuid.Nil,
			wantErr:   true,
			wantField: "group_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPerson(tt.first, tt.last, tt.birthYear, tt.groupID)
			if tt.wantErr {
				requireValidationField(t, err, tt.wantField)
				if p != nil {
					t.Fatalf("expected nil person on error, got %+v", p)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.FirstName != strings.TrimSpace(tt.first) {
				t.Errorf("first_name = %q, want %q", p.FirstName, strings.TrimSpace(tt.first))
			}
			if p.LastName != strings.TrimSpace(tt.last) {
				t.Errorf("last_name = %q, want %q", p.LastName, strings.TrimSpace(tt.last))
			}
		})
	}
}

func TestPersonValidateAccumulatesFields(t *testing.T) {
	p := &Person{}
	err := p.Validate()
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %v is not *ValidationError", err)
	}
	// Пустой человек нарушает все четыре инварианта сразу.
	if len(ve.Fields) != 4 {
		t.Errorf("got %d field errors, want 4: %v", len(ve.Fields), ve.Fields)
	}
}
