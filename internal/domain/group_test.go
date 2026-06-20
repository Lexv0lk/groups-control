package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewGroup(t *testing.T) {
	parent := uuid.New()

	tests := []struct {
		name       string
		groupName  string
		parentID   *uuid.UUID
		wantErr    bool
		wantField  string
		wantParent *uuid.UUID
	}{
		{
			name:       "valid root group",
			groupName:  "Engineering",
			parentID:   nil,
			wantErr:    false,
			wantParent: nil,
		},
		{
			name:       "valid child group trims name",
			groupName:  "  Platform  ",
			parentID:   &parent,
			wantErr:    false,
			wantParent: &parent,
		},
		{
			name:      "empty name",
			groupName: "",
			wantErr:   true,
			wantField: "name",
		},
		{
			name:      "whitespace-only name",
			groupName: "   ",
			wantErr:   true,
			wantField: "name",
		},
		{
			name:      "name too long",
			groupName: strings.Repeat("a", groupNameMaxLen+1),
			wantErr:   true,
			wantField: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewGroup(tt.groupName, tt.parentID)
			if tt.wantErr {
				requireValidationField(t, err, tt.wantField)
				if g != nil {
					t.Fatalf("expected nil group on error, got %+v", g)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g.Name != strings.TrimSpace(tt.groupName) {
				t.Errorf("name = %q, want trimmed %q", g.Name, strings.TrimSpace(tt.groupName))
			}
			if tt.wantParent == nil && g.ParentID != nil {
				t.Errorf("parent = %v, want nil", g.ParentID)
			}
			if tt.wantParent != nil && (g.ParentID == nil || *g.ParentID != *tt.wantParent) {
				t.Errorf("parent = %v, want %v", g.ParentID, *tt.wantParent)
			}
		})
	}
}

func TestGroupRename(t *testing.T) {
	g, err := NewGroup("Engineering", nil)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := g.Rename("  Platform  "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Name != "Platform" {
		t.Errorf("name = %q, want %q", g.Name, "Platform")
	}

	if err := g.Rename(""); !errors.Is(err, ErrValidation) {
		t.Errorf("Rename(\"\") error = %v, want ErrValidation", err)
	}
	if g.Name != "Platform" {
		t.Errorf("name mutated after failed rename: %q", g.Name)
	}
}

// requireValidationField проверяет, что err — ошибка валидации с указанным полем.
func requireValidationField(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %v is not *ValidationError", err)
	}
	for _, f := range ve.Fields {
		if f.Field == field {
			return
		}
	}
	t.Fatalf("validation error %v does not mention field %q", err, field)
}
