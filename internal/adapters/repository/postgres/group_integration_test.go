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

// Идентификаторы из migrations/000002_seed.up.sql.
var (
	seedAcme        = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	seedEngineering = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	seedBackend     = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	seedPlatform    = uuid.MustParse("44444444-4444-4444-4444-444444444444")
	seedFrontend    = uuid.MustParse("55555555-5555-5555-5555-555555555555")
	seedSales       = uuid.MustParse("66666666-6666-6666-6666-666666666666")
)

func TestGroupRepository_CreateGetUpdate(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	repo := NewGroupRepository(testPool)

	root, err := domain.NewGroup("Root", nil)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, root))
	assert.NotEqual(t, uuid.Nil, root.ID, "ID must be assigned by the database")
	assert.False(t, root.CreatedAt.IsZero(), "created_at must be populated")

	child, err := domain.NewGroup("Child", &root.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, child))

	got, err := repo.GetByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, "Child", got.Name)
	require.NotNil(t, got.ParentID)
	assert.Equal(t, root.ID, *got.ParentID)

	got.Name = "Renamed"
	got.ParentID = nil
	require.NoError(t, repo.Update(ctx, got))

	reloaded, err := repo.GetByID(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", reloaded.Name)
	assert.Nil(t, reloaded.ParentID)
	assert.True(t, reloaded.UpdatedAt.After(reloaded.CreatedAt) ||
		reloaded.UpdatedAt.Equal(reloaded.CreatedAt))
}

func TestGroupRepository_GetByID_NotFound(t *testing.T) {
	resetDB(t)
	repo := NewGroupRepository(testPool)

	_, err := repo.GetByID(context.Background(), uuid.New())
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGroupRepository_Create_MissingParent(t *testing.T) {
	resetDB(t)
	repo := NewGroupRepository(testPool)

	orphanParent := uuid.New()
	g, err := domain.NewGroup("Orphan", &orphanParent)
	require.NoError(t, err)

	err = repo.Create(context.Background(), g)
	assert.ErrorIs(t, err, domain.ErrValidation)
}

func TestGroupRepository_Update_NotFound(t *testing.T) {
	resetDB(t)
	repo := NewGroupRepository(testPool)

	err := repo.Update(context.Background(), &domain.Group{ID: uuid.New(), Name: "Ghost"})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGroupRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewGroupRepository(testPool)

	t.Run("not found", func(t *testing.T) {
		resetDB(t)
		err := repo.Delete(ctx, uuid.New())
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("blocked by child groups", func(t *testing.T) {
		resetDB(t)
		loadSeed(t)
		err := repo.Delete(ctx, seedEngineering)
		assert.ErrorIs(t, err, domain.ErrGroupHasChildren)
	})

	t.Run("blocked by people", func(t *testing.T) {
		resetDB(t)
		loadSeed(t)
		err := repo.Delete(ctx, seedPlatform)
		assert.ErrorIs(t, err, domain.ErrGroupHasPeople)
	})

	t.Run("deletes empty leaf", func(t *testing.T) {
		resetDB(t)
		leaf, err := domain.NewGroup("Leaf", nil)
		require.NoError(t, err)
		require.NoError(t, repo.Create(ctx, leaf))

		require.NoError(t, repo.Delete(ctx, leaf.ID))
		_, err = repo.GetByID(ctx, leaf.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestGroupRepository_CountMembers(t *testing.T) {
	resetDB(t)
	loadSeed(t)
	ctx := context.Background()
	repo := NewGroupRepository(testPool)

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
			direct, err := repo.CountMembers(ctx, tt.id, false)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDirect, direct, "direct count")

			total, err := repo.CountMembers(ctx, tt.id, true)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTotal, total, "total (subtree) count")
		})
	}
}

func TestGroupRepository_IsDescendant(t *testing.T) {
	resetDB(t)
	loadSeed(t)
	ctx := context.Background()
	repo := NewGroupRepository(testPool)

	tests := []struct {
		name      string
		ancestor  uuid.UUID
		candidate uuid.UUID
		want      bool
	}{
		{"deep descendant", seedAcme, seedPlatform, true},
		{"direct child", seedEngineering, seedBackend, true},
		{"self counts as descendant", seedBackend, seedBackend, true},
		{"unrelated sibling subtree", seedSales, seedPlatform, false},
		{"ancestor is not descendant of child", seedBackend, seedAcme, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.IsDescendant(ctx, tt.ancestor, tt.candidate)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGroupRepository_List(t *testing.T) {
	resetDB(t)
	loadSeed(t)
	ctx := context.Background()
	repo := NewGroupRepository(testPool)

	items, total, err := repo.List(ctx, usecase.Pagination{Limit: 100, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 6, total, "total group count")
	require.Len(t, items, 6)

	byID := make(map[uuid.UUID]usecase.GroupWithCounts, len(items))
	for _, it := range items {
		byID[it.Group.ID] = it
	}
	assert.Equal(t, usecase.MemberCounts{Direct: 1, Total: 8}, byID[seedAcme].Counts)
	assert.Equal(t, usecase.MemberCounts{Direct: 2, Total: 6}, byID[seedEngineering].Counts)
	assert.Equal(t, usecase.MemberCounts{Direct: 1, Total: 3}, byID[seedBackend].Counts)

	// Пагинация: вторая страница по 2 элемента, общее число не меняется.
	page, total, err := repo.List(ctx, usecase.Pagination{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Equal(t, 6, total)
	assert.Len(t, page, 2)
}

func TestGroupRepository_List_Empty(t *testing.T) {
	resetDB(t)
	repo := NewGroupRepository(testPool)

	items, total, err := repo.List(context.Background(), usecase.Pagination{Limit: 10, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, items)
}
