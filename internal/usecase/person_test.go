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

func TestPersonService_Create(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания моков и возвращает входные параметры вызова.
		setup func(t *testing.T, people *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (firstName, lastName string, birthYear int, groupID uuid.UUID)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
		// assertPerson проверяет созданную сущность при успешном вызове.
		assertPerson func(t *testing.T, p *domain.Person)
	}{
		{
			name: "creates person in existing group",
			setup: func(_ *testing.T, people *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (string, string, int, uuid.UUID) {
				groupID := uuid.New()
				groups.EXPECT().GetByID(ctx, groupID).Return(&domain.Group{ID: groupID, Name: "Eng"}, nil)
				people.EXPECT().
					Create(ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, p *domain.Person) error {
						p.ID = uuid.New()
						return nil
					})
				return "Ada", "Lovelace", 1990, groupID
			},
			assertPerson: func(t *testing.T, p *domain.Person) {
				assert.NotEqual(t, uuid.Nil, p.ID, "expected ID to be assigned")
				assert.Equal(t, "Ada", p.FirstName)
				assert.Equal(t, "Lovelace", p.LastName)
				assert.Equal(t, 1990, p.BirthYear)
			},
		},
		{
			name: "rejects missing group with validation error",
			setup: func(_ *testing.T, _ *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (string, string, int, uuid.UUID) {
				groupID := uuid.New()
				groups.EXPECT().GetByID(ctx, groupID).Return(nil, domain.ErrNotFound)
				return "Ada", "Lovelace", 1990, groupID
			},
			wantErr: domain.ErrValidation,
		},
		{
			name: "rejects invalid fields without touching the person repository",
			setup: func(_ *testing.T, _ *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (string, string, int, uuid.UUID) {
				groupID := uuid.New()
				groups.EXPECT().GetByID(ctx, groupID).Return(&domain.Group{ID: groupID}, nil)
				return "  ", "Lovelace", 1990, groupID
			},
			wantErr: domain.ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			people := mocks.NewMockPersonRepository(ctrl)
			groups := mocks.NewMockGroupRepository(ctrl)
			firstName, lastName, birthYear, groupID := tt.setup(t, people, groups)
			svc := usecase.NewPersonService(people, groups)

			person, err := svc.Create(ctx, firstName, lastName, birthYear, groupID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, person)
			if tt.assertPerson != nil {
				tt.assertPerson(t, person)
			}
		})
	}
}

func TestPersonService_Update(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания моков и возвращает входные параметры вызова.
		setup func(t *testing.T, people *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (id uuid.UUID, firstName, lastName string, birthYear int, groupID uuid.UUID)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
		// assertPerson проверяет обновлённую сущность при успешном вызове.
		assertPerson func(t *testing.T, p *domain.Person)
	}{
		{
			name: "updates fields and moves to another group",
			setup: func(t *testing.T, people *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (uuid.UUID, string, string, int, uuid.UUID) {
				id := uuid.New()
				oldGroup := uuid.New()
				newGroup := uuid.New()
				people.EXPECT().GetByID(ctx, id).Return(&domain.Person{ID: id, FirstName: "Ada", LastName: "Lovelace", BirthYear: 1990, GroupID: oldGroup}, nil)
				groups.EXPECT().GetByID(ctx, newGroup).Return(&domain.Group{ID: newGroup, Name: "Research"}, nil)
				people.EXPECT().
					Update(ctx, gomock.Any()).
					DoAndReturn(func(_ context.Context, p *domain.Person) error {
						assert.Equal(t, newGroup, p.GroupID, "group change must be persisted")
						assert.Equal(t, "Grace", p.FirstName, "rename must be persisted")
						return nil
					})
				return id, "Grace", "Hopper", 1985, newGroup
			},
			assertPerson: func(t *testing.T, p *domain.Person) {
				assert.Equal(t, "Grace", p.FirstName)
				assert.Equal(t, "Hopper", p.LastName)
				assert.Equal(t, 1985, p.BirthYear)
			},
		},
		{
			name: "keeps same group without re-checking it",
			setup: func(_ *testing.T, people *mocks.MockPersonRepository, _ *mocks.MockGroupRepository) (uuid.UUID, string, string, int, uuid.UUID) {
				id := uuid.New()
				groupID := uuid.New()
				people.EXPECT().GetByID(ctx, id).Return(&domain.Person{ID: id, FirstName: "Ada", LastName: "Lovelace", BirthYear: 1990, GroupID: groupID}, nil)
				people.EXPECT().Update(ctx, gomock.Any()).Return(nil)
				return id, "Ada", "Byron", 1990, groupID
			},
			assertPerson: func(t *testing.T, p *domain.Person) {
				assert.Equal(t, "Byron", p.LastName)
			},
		},
		{
			name: "returns not found when person is absent",
			setup: func(_ *testing.T, people *mocks.MockPersonRepository, _ *mocks.MockGroupRepository) (uuid.UUID, string, string, int, uuid.UUID) {
				id := uuid.New()
				people.EXPECT().GetByID(ctx, id).Return(nil, domain.ErrNotFound)
				return id, "Ada", "Lovelace", 1990, uuid.New()
			},
			wantErr: domain.ErrNotFound,
		},
		{
			name: "rejects move to missing group with validation error",
			setup: func(_ *testing.T, people *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (uuid.UUID, string, string, int, uuid.UUID) {
				id := uuid.New()
				oldGroup := uuid.New()
				newGroup := uuid.New()
				people.EXPECT().GetByID(ctx, id).Return(&domain.Person{ID: id, FirstName: "Ada", LastName: "Lovelace", BirthYear: 1990, GroupID: oldGroup}, nil)
				groups.EXPECT().GetByID(ctx, newGroup).Return(nil, domain.ErrNotFound)
				return id, "Ada", "Lovelace", 1990, newGroup
			},
			wantErr: domain.ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			people := mocks.NewMockPersonRepository(ctrl)
			groups := mocks.NewMockGroupRepository(ctrl)
			id, firstName, lastName, birthYear, groupID := tt.setup(t, people, groups)
			svc := usecase.NewPersonService(people, groups)

			person, err := svc.Update(ctx, id, firstName, lastName, birthYear, groupID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, person)
			if tt.assertPerson != nil {
				tt.assertPerson(t, person)
			}
		})
	}
}

func TestPersonService_Delete(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания мока и возвращает удаляемый идентификатор.
		setup func(t *testing.T, people *mocks.MockPersonRepository) (id uuid.UUID)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
	}{
		{
			name: "deletes person",
			setup: func(_ *testing.T, people *mocks.MockPersonRepository) uuid.UUID {
				id := uuid.New()
				people.EXPECT().Delete(ctx, id).Return(nil)
				return id
			},
		},
		{
			name: "propagates not found",
			setup: func(_ *testing.T, people *mocks.MockPersonRepository) uuid.UUID {
				id := uuid.New()
				people.EXPECT().Delete(ctx, id).Return(domain.ErrNotFound)
				return id
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			people := mocks.NewMockPersonRepository(ctrl)
			groups := mocks.NewMockGroupRepository(ctrl)
			id := tt.setup(t, people)
			svc := usecase.NewPersonService(people, groups)

			err := svc.Delete(ctx, id)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPersonService_ListByGroup(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// setup настраивает ожидания моков и возвращает параметры выборки.
		setup func(t *testing.T, people *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (groupID uuid.UUID, includeDescendants bool, page usecase.Pagination)
		// wantErr — ожидаемая sentinel-ошибка (nil — успех).
		wantErr error
		// wantTotal — ожидаемое общее число людей при успешном вызове.
		wantTotal int
	}{
		{
			name: "lists people of an existing group",
			setup: func(_ *testing.T, people *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (uuid.UUID, bool, usecase.Pagination) {
				groupID := uuid.New()
				page := usecase.Pagination{Limit: 10, Offset: 0}
				groups.EXPECT().GetByID(ctx, groupID).Return(&domain.Group{ID: groupID}, nil)
				people.EXPECT().ListByGroup(ctx, groupID, true, page).Return([]domain.Person{{ID: uuid.New()}, {ID: uuid.New()}}, 2, nil)
				return groupID, true, page
			},
			wantTotal: 2,
		},
		{
			name: "propagates not found for missing group",
			setup: func(_ *testing.T, _ *mocks.MockPersonRepository, groups *mocks.MockGroupRepository) (uuid.UUID, bool, usecase.Pagination) {
				groupID := uuid.New()
				groups.EXPECT().GetByID(ctx, groupID).Return(nil, domain.ErrNotFound)
				return groupID, false, usecase.Pagination{Limit: 10}
			},
			wantErr: domain.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			people := mocks.NewMockPersonRepository(ctrl)
			groups := mocks.NewMockGroupRepository(ctrl)
			groupID, includeDescendants, page := tt.setup(t, people, groups)
			svc := usecase.NewPersonService(people, groups)

			items, total, err := svc.ListByGroup(ctx, groupID, includeDescendants, page)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, items, tt.wantTotal)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}
