package usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"groups-control/gen/mocks"
	"groups-control/internal/domain"
	"groups-control/internal/usecase"
)

func TestGroupService_Create(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания мока и возвращает входные параметры вызова.
		setup func(t *testing.T, repo *mocks.MockGroupRepository) (groupName string, parentID *uuid.UUID)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
		// assertGroup проверяет созданную сущность при успешном вызове.
		assertGroup func(t *testing.T, g *domain.Group)
	}{
		{
			name: "creates root group",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) (string, *uuid.UUID) {
				repo.EXPECT().
					Create(ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, g *domain.Group) error {
						g.ID = uuid.New()
						return nil
					})
				return "Engineering", nil
			},
			assertGroup: func(t *testing.T, g *domain.Group) {
				assert.NotEqual(t, uuid.Nil, g.ID, "expected ID to be assigned")
				assert.Equal(t, "Engineering", g.Name)
				assert.Nil(t, g.ParentID)
			},
		},
		{
			name: "verifies parent exists before creating",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) (string, *uuid.UUID) {
				parentID := uuid.New()
				repo.EXPECT().GetByID(ctx, parentID).Return(&domain.Group{ID: parentID, Name: "Parent"}, nil)
				repo.EXPECT().Create(ctx, gomock.Any()).Return(nil)
				return "Backend", &parentID
			},
			assertGroup: func(t *testing.T, g *domain.Group) {
				require.NotNil(t, g.ParentID)
			},
		},
		{
			name: "rejects missing parent with validation error",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) (string, *uuid.UUID) {
				parentID := uuid.New()
				repo.EXPECT().GetByID(ctx, parentID).Return(nil, domain.ErrNotFound)
				return "Backend", &parentID
			},
			wantErr: domain.ErrValidation,
		},
		{
			name: "rejects invalid name without touching the repository",
			setup: func(_ *testing.T, _ *mocks.MockGroupRepository) (string, *uuid.UUID) {
				return "   ", nil
			},
			wantErr: domain.ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockGroupRepository(gomock.NewController(t))
			groupName, parentID := tt.setup(t, repo)
			svc := usecase.NewGroupService(repo)

			group, err := svc.Create(ctx, groupName, parentID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, group)
			if tt.assertGroup != nil {
				tt.assertGroup(t, group)
			}
		})
	}
}

func TestGroupService_Update(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания мока и возвращает входные параметры вызова.
		setup func(t *testing.T, repo *mocks.MockGroupRepository) (id uuid.UUID, newName string, parentID *uuid.UUID)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
		// assertGroup проверяет обновлённую сущность при успешном вызове.
		assertGroup func(t *testing.T, g *domain.Group)
	}{
		{
			name: "renames and reparents group",
			setup: func(t *testing.T, repo *mocks.MockGroupRepository) (uuid.UUID, string, *uuid.UUID) {
				id := uuid.New()
				newParent := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(&domain.Group{ID: id, Name: "Old"}, nil)
				repo.EXPECT().GetByID(ctx, newParent).Return(&domain.Group{ID: newParent, Name: "Parent"}, nil)
				repo.EXPECT().IsDescendant(ctx, id, newParent).Return(false, nil)
				repo.EXPECT().
					Update(ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, g *domain.Group) error {
						require.NotNil(t, g.ParentID)
						assert.Equal(t, newParent, *g.ParentID, "parent must be persisted")
						assert.Equal(t, "New", g.Name, "rename must be persisted")
						return nil
					})
				return id, "New", &newParent
			},
			assertGroup: func(t *testing.T, g *domain.Group) {
				assert.Equal(t, "New", g.Name)
			},
		},
		{
			name: "returns not found when group is absent",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) (uuid.UUID, string, *uuid.UUID) {
				id := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(nil, domain.ErrNotFound)
				return id, "New", nil
			},
			wantErr: domain.ErrNotFound,
		},
		{
			name: "rejects group as its own parent",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) (uuid.UUID, string, *uuid.UUID) {
				id := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(&domain.Group{ID: id, Name: "Self"}, nil)
				return id, "Self", &id
			},
			wantErr: domain.ErrCyclicParent,
		},
		{
			name: "rejects parent from own subtree",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) (uuid.UUID, string, *uuid.UUID) {
				id := uuid.New()
				descendant := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(&domain.Group{ID: id, Name: "Node"}, nil)
				repo.EXPECT().GetByID(ctx, descendant).Return(&domain.Group{ID: descendant, Name: "Child"}, nil)
				repo.EXPECT().IsDescendant(ctx, id, descendant).Return(true, nil)
				return id, "Node", &descendant
			},
			wantErr: domain.ErrCyclicParent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockGroupRepository(gomock.NewController(t))
			id, newName, parentID := tt.setup(t, repo)
			svc := usecase.NewGroupService(repo)

			group, err := svc.Update(ctx, id, newName, parentID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, group)
			if tt.assertGroup != nil {
				tt.assertGroup(t, group)
			}
		})
	}
}

func TestGroupService_Delete(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания мока и возвращает удаляемый идентификатор.
		setup func(t *testing.T, repo *mocks.MockGroupRepository) (id uuid.UUID)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
	}{
		{
			name: "deletes empty group",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) uuid.UUID {
				id := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(&domain.Group{ID: id}, nil)
				repo.EXPECT().CountMembers(ctx, id, false).Return(0, nil)
				repo.EXPECT().Delete(ctx, id).Return(nil)
				return id
			},
		},
		{
			name: "rejects deletion when group has people",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) uuid.UUID {
				id := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(&domain.Group{ID: id}, nil)
				repo.EXPECT().CountMembers(ctx, id, false).Return(3, nil)
				return id
			},
			wantErr: domain.ErrGroupHasPeople,
		},
		{
			name: "propagates not found",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) uuid.UUID {
				id := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(nil, domain.ErrNotFound)
				return id
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockGroupRepository(gomock.NewController(t))
			id := tt.setup(t, repo)
			svc := usecase.NewGroupService(repo)

			err := svc.Delete(ctx, id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGroupService_Counts(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания мока и возвращает идентификатор группы.
		setup func(t *testing.T, repo *mocks.MockGroupRepository) (id uuid.UUID)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
		// wantCounts — ожидаемые счётчики при успешном вызове.
		wantCounts usecase.MemberCounts
	}{
		{
			name: "returns direct and total counts",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) uuid.UUID {
				id := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(&domain.Group{ID: id}, nil)
				repo.EXPECT().CountMembers(ctx, id, false).Return(4, nil)
				repo.EXPECT().CountMembers(ctx, id, true).Return(10, nil)
				return id
			},
			wantCounts: usecase.MemberCounts{Direct: 4, Total: 10},
		},
		{
			name: "propagates not found",
			setup: func(_ *testing.T, repo *mocks.MockGroupRepository) uuid.UUID {
				id := uuid.New()
				repo.EXPECT().GetByID(ctx, id).Return(nil, domain.ErrNotFound)
				return id
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockGroupRepository(gomock.NewController(t))
			id := tt.setup(t, repo)
			svc := usecase.NewGroupService(repo)

			counts, err := svc.Counts(ctx, id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantCounts, counts)
		})
	}
}
