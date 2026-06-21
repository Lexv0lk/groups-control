//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"groups-control/internal/domain"
	"groups-control/internal/usecase"
)

func TestPersonRepository_CreateGetUpdate(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	groups := NewGroupRepository(testPool)
	repo := NewPersonRepository(testPool)

	group, err := domain.NewGroup("Group", nil)
	require.NoError(t, err)
	require.NoError(t, groups.Create(ctx, group))

	other, err := domain.NewGroup("Other", nil)
	require.NoError(t, err)
	require.NoError(t, groups.Create(ctx, other))

	person, err := domain.NewPerson("Alice", "Anderson", 1980, group.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, person))
	assert.NotEqual(t, uuid.Nil, person.ID, "ID must be assigned by the database")
	assert.False(t, person.CreatedAt.IsZero(), "created_at must be populated")

	got, err := repo.GetByID(ctx, person.ID)
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.FirstName)
	assert.Equal(t, "Anderson", got.LastName)
	assert.Equal(t, 1980, got.BirthYear)
	assert.Equal(t, group.ID, got.GroupID)

	// Обновление: смена имени и группы.
	got.LastName = "Brown"
	got.GroupID = other.ID
	require.NoError(t, repo.Update(ctx, got))

	reloaded, err := repo.GetByID(ctx, person.ID)
	require.NoError(t, err)
	assert.Equal(t, "Brown", reloaded.LastName)
	assert.Equal(t, other.ID, reloaded.GroupID)
	assert.True(t, reloaded.UpdatedAt.After(reloaded.CreatedAt) ||
		reloaded.UpdatedAt.Equal(reloaded.CreatedAt))
}

func TestPersonRepository_GetByID_NotFound(t *testing.T) {
	resetDB(t)
	repo := NewPersonRepository(testPool)

	_, err := repo.GetByID(context.Background(), uuid.New())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPersonRepository_Create_MissingGroup(t *testing.T) {
	resetDB(t)
	repo := NewPersonRepository(testPool)

	person, err := domain.NewPerson("Orphan", "Person", 1990, uuid.New())
	require.NoError(t, err)

	err = repo.Create(context.Background(), person)
	assert.ErrorIs(t, err, domain.ErrValidation)
}

func TestPersonRepository_Update_NotFound(t *testing.T) {
	resetDB(t)
	loadSeed(t)
	repo := NewPersonRepository(testPool)

	err := repo.Update(context.Background(), &domain.Person{
		ID:        uuid.New(),
		FirstName: "Ghost",
		LastName:  "Ghost",
		BirthYear: 1990,
		GroupID:   seedAcme,
	})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPersonRepository_Update_MissingGroup(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	groups := NewGroupRepository(testPool)
	repo := NewPersonRepository(testPool)

	group, err := domain.NewGroup("Group", nil)
	require.NoError(t, err)
	require.NoError(t, groups.Create(ctx, group))

	person, err := domain.NewPerson("Alice", "Anderson", 1980, group.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, person))

	person.GroupID = uuid.New()
	err = repo.Update(ctx, person)
	assert.ErrorIs(t, err, domain.ErrValidation)
}

func TestPersonRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewPersonRepository(testPool)

	t.Run("not found", func(t *testing.T) {
		resetDB(t)
		err := repo.Delete(ctx, uuid.New())
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("deletes existing", func(t *testing.T) {
		resetDB(t)
		groups := NewGroupRepository(testPool)
		group, err := domain.NewGroup("Group", nil)
		require.NoError(t, err)
		require.NoError(t, groups.Create(ctx, group))

		person, err := domain.NewPerson("Alice", "Anderson", 1980, group.ID)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, person))

		require.NoError(t, repo.Delete(ctx, person.ID))
		_, err = repo.GetByID(ctx, person.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestPersonRepository_ListByGroup(t *testing.T) {
	resetDB(t)
	loadSeed(t)
	ctx := context.Background()
	repo := NewPersonRepository(testPool)

	tests := []struct {
		name       string
		id         uuid.UUID
		wantDirect int
		wantTotal  int
	}{
		{"root", seedAcme, 1, 8},
		{"engineering", seedEngineering, 2, 6},
		{"backend", seedBackend, 1, 3},
		{"platform leaf", seedPlatform, 2, 2},
		{"frontend leaf", seedFrontend, 1, 1},
		{"sales leaf", seedSales, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := usecase.Pagination{Limit: 100, Offset: 0}

			direct, total, err := repo.ListByGroup(ctx, tt.id, false, page)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDirect, total, "direct total")
			assert.Len(t, direct, tt.wantDirect)
			for _, p := range direct {
				assert.Equal(t, tt.id, p.GroupID, "direct people belong to the group")
			}

			subtree, total, err := repo.ListByGroup(ctx, tt.id, true, page)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTotal, total, "subtree total")
			assert.Len(t, subtree, tt.wantTotal)
		})
	}
}

func TestPersonRepository_ListByGroup_Pagination(t *testing.T) {
	resetDB(t)
	loadSeed(t)
	ctx := context.Background()
	repo := NewPersonRepository(testPool)

	page, total, err := repo.ListByGroup(ctx, seedAcme, true, usecase.Pagination{Limit: 3, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 8, total, "total is independent of the page size")
	assert.Len(t, page, 3)

	last, total, err := repo.ListByGroup(ctx, seedAcme, true, usecase.Pagination{Limit: 3, Offset: 6})
	require.NoError(t, err)
	assert.Equal(t, 8, total)
	assert.Len(t, last, 2)
}

func TestPersonRepository_ListByGroup_Empty(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	groups := NewGroupRepository(testPool)
	repo := NewPersonRepository(testPool)

	group, err := domain.NewGroup("Empty", nil)
	require.NoError(t, err)
	require.NoError(t, groups.Create(ctx, group))

	page := usecase.Pagination{Limit: 10, Offset: 0}

	direct, total, err := repo.ListByGroup(ctx, group.ID, false, page)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, direct)

	subtree, total, err := repo.ListByGroup(ctx, group.ID, true, page)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, subtree)
}
