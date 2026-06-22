//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"groups-control/internal/adapters/http/gen"
)

// createGroup создаёт группу через API и возвращает её представление, проверяя
// статус 201.
func createGroup(t *testing.T, name string, parentID *openapi_types.UUID) gen.Group {
	t.Helper()
	status, data := doRequest(t, http.MethodPost, "/groups", gen.CreateGroupRequest{
		Name:     name,
		ParentId: parentID,
	})
	require.Equal(t, http.StatusCreated, status, "create group %q: %s", name, string(data))
	group := decode[gen.Group](t, data)
	require.NotNil(t, group.Id)
	return group
}

// createPerson создаёт человека через API и возвращает его представление,
// проверяя статус 201.
func createPerson(t *testing.T, first, last string, year int32, groupID openapi_types.UUID) gen.Person {
	t.Helper()
	status, data := doRequest(t, http.MethodPost, "/people", gen.CreatePersonRequest{
		FirstName: first,
		LastName:  last,
		BirthYear: year,
		GroupId:   groupID,
	})
	require.Equal(t, http.StatusCreated, status, "create person %s %s: %s", first, last, string(data))
	person := decode[gen.Person](t, data)
	require.NotNil(t, person.Id)
	return person
}

// getGroup запрашивает группу с подсчётами участников, проверяя статус 200.
func getGroup(t *testing.T, id openapi_types.UUID) gen.GroupWithCounts {
	t.Helper()
	status, data := doRequest(t, http.MethodGet, "/groups/"+id.String(), nil)
	require.Equal(t, http.StatusOK, status, "get group: %s", string(data))
	return decode[gen.GroupWithCounts](t, data)
}

// TestGroupTreeCountsAndListings строит дерево групп минимум на три уровня,
// раскладывает людей по узлам и проверяет ключевое требование ТЗ: подсчёт
// участников (direct/total) и выборку людей с/без дочерних групп.
//
// Дерево:
//
//	root            (2 человека)
//	├── childA      (3 человека)
//	│   └── grandC  (1 человек)
//	└── childB      (0 человек)
func TestGroupTreeCountsAndListings(t *testing.T) {
	resetDB(t)

	root := createGroup(t, "root", nil)
	childA := createGroup(t, "childA", root.Id)
	childB := createGroup(t, "childB", root.Id)
	grandC := createGroup(t, "grandC", childA.Id)

	// Раскладываем людей по узлам дерева.
	createPerson(t, "Root", "One", 1990, *root.Id)
	createPerson(t, "Root", "Two", 1991, *root.Id)
	createPerson(t, "A", "One", 1992, *childA.Id)
	createPerson(t, "A", "Two", 1993, *childA.Id)
	createPerson(t, "A", "Three", 1994, *childA.Id)
	createPerson(t, "Grand", "One", 1995, *grandC.Id)

	t.Run("counts respect subtree", func(t *testing.T) {
		rootG := getGroup(t, *root.Id)
		assert.EqualValues(t, 2, rootG.DirectCount, "root direct")
		assert.EqualValues(t, 6, rootG.TotalCount, "root total (whole tree)")

		childAG := getGroup(t, *childA.Id)
		assert.EqualValues(t, 3, childAG.DirectCount, "childA direct")
		assert.EqualValues(t, 4, childAG.TotalCount, "childA total (with grandC)")

		childBG := getGroup(t, *childB.Id)
		assert.EqualValues(t, 0, childBG.DirectCount, "childB direct")
		assert.EqualValues(t, 0, childBG.TotalCount, "childB total")

		grandG := getGroup(t, *grandC.Id)
		assert.EqualValues(t, 1, grandG.DirectCount, "grandC direct")
		assert.EqualValues(t, 1, grandG.TotalCount, "grandC total")
	})

	t.Run("list people without descendants", func(t *testing.T) {
		status, data := doRequest(t, http.MethodGet,
			"/groups/"+root.Id.String()+"/people?include_descendants=false", nil)
		require.Equal(t, http.StatusOK, status, "%s", string(data))
		list := decode[gen.PersonList](t, data)
		assert.Len(t, list.Items, 2, "only people directly in root")
		assert.EqualValues(t, 2, list.Pagination.Total)
	})

	t.Run("list people with descendants", func(t *testing.T) {
		status, data := doRequest(t, http.MethodGet,
			"/groups/"+root.Id.String()+"/people?include_descendants=true", nil)
		require.Equal(t, http.StatusOK, status, "%s", string(data))
		list := decode[gen.PersonList](t, data)
		assert.Len(t, list.Items, 6, "all people in the subtree")
		assert.EqualValues(t, 6, list.Pagination.Total)
	})

	t.Run("list groups reports all nodes", func(t *testing.T) {
		status, data := doRequest(t, http.MethodGet, "/groups?limit=50&offset=0", nil)
		require.Equal(t, http.StatusOK, status, "%s", string(data))
		list := decode[gen.GroupList](t, data)
		assert.EqualValues(t, 4, list.Pagination.Total, "four groups total")
		assert.Len(t, list.Items, 4)
	})

	t.Run("list groups applies contract default limit", func(t *testing.T) {
		status, data := doRequest(t, http.MethodGet, "/groups", nil)
		require.Equal(t, http.StatusOK, status, "%s", string(data))
		list := decode[gen.GroupList](t, data)
		// Дефолтный limit должен совпадать с OpenAPI-контрактом (20), а не с
		// внутренним значением реализации.
		assert.EqualValues(t, 20, list.Pagination.Limit, "default limit from contract")
	})
}

// TestPersonLifecycle проверяет жизненный цикл человека через API: создание,
// смену группы при обновлении и удаление — со сквозной проверкой состояния.
func TestPersonLifecycle(t *testing.T) {
	resetDB(t)

	g1 := createGroup(t, "g1", nil)
	g2 := createGroup(t, "g2", nil)

	person := createPerson(t, "John", "Doe", 1988, *g1.Id)
	require.Equal(t, *g1.Id, person.GroupId)

	t.Run("move to another group on update", func(t *testing.T) {
		status, data := doRequest(t, http.MethodPut, "/people/"+person.Id.String(), gen.UpdatePersonRequest{
			FirstName: "John",
			LastName:  "Doe",
			BirthYear: 1988,
			GroupId:   *g2.Id,
		})
		require.Equal(t, http.StatusOK, status, "%s", string(data))
		updated := decode[gen.Person](t, data)
		assert.Equal(t, *g2.Id, updated.GroupId, "person moved to g2")

		// g1 опустел, g2 получил человека.
		assert.EqualValues(t, 0, getGroup(t, *g1.Id).DirectCount)
		assert.EqualValues(t, 1, getGroup(t, *g2.Id).DirectCount)
	})

	t.Run("delete person", func(t *testing.T) {
		status, _ := doRequest(t, http.MethodDelete, "/people/"+person.Id.String(), nil)
		require.Equal(t, http.StatusNoContent, status)

		status, _ = doRequest(t, http.MethodGet, "/people/"+person.Id.String(), nil)
		assert.Equal(t, http.StatusNotFound, status, "person no longer exists")
	})
}

// TestErrorScenarios покрывает обработку ошибок из ТЗ: 404 для отсутствующих
// ресурсов, 409 при конфликте удаления/циклах и 422 при нарушении валидации.
func TestErrorScenarios(t *testing.T) {
	resetDB(t)

	t.Run("404 on missing group", func(t *testing.T) {
		status, data := doRequest(t, http.MethodGet, "/groups/"+uuidNew().String(), nil)
		require.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, gen.ErrorCodeNotFound, decode[gen.Error](t, data).Code)
	})

	t.Run("404 on unknown route returns error envelope", func(t *testing.T) {
		status, data := doRequest(t, http.MethodGet, "/does-not-exist", nil)
		require.Equal(t, http.StatusNotFound, status, "%s", string(data))
		assert.Equal(t, gen.ErrorCodeNotFound, decode[gen.Error](t, data).Code)
	})

	t.Run("405 on unsupported method returns envelope and Allow", func(t *testing.T) {
		status, header, data := doRequestHeaders(t, http.MethodPatch, "/groups", nil)
		require.Equal(t, http.StatusMethodNotAllowed, status, "%s", string(data))
		assert.Equal(t, gen.ErrorCodeBadRequest, decode[gen.Error](t, data).Code)
		allow := header.Values("Allow")
		assert.Contains(t, allow, http.MethodGet, "Allow must advertise GET")
		assert.Contains(t, allow, http.MethodPost, "Allow must advertise POST")
	})

	t.Run("422 on empty group name", func(t *testing.T) {
		status, data := doRequest(t, http.MethodPost, "/groups", gen.CreateGroupRequest{Name: ""})
		require.Equal(t, http.StatusUnprocessableEntity, status, "%s", string(data))
		assert.Equal(t, gen.ErrorCodeValidationError, decode[gen.Error](t, data).Code)
	})

	t.Run("422 on person referencing missing group", func(t *testing.T) {
		status, data := doRequest(t, http.MethodPost, "/people", gen.CreatePersonRequest{
			FirstName: "Ghost",
			LastName:  "User",
			BirthYear: 2000,
			GroupId:   uuidNew(),
		})
		require.Equal(t, http.StatusUnprocessableEntity, status, "%s", string(data))
		body := decode[gen.Error](t, data)
		assert.Equal(t, gen.ErrorCodeValidationError, body.Code)
	})

	t.Run("409 deleting group with people", func(t *testing.T) {
		g := createGroup(t, "with-people", nil)
		createPerson(t, "Has", "Person", 1999, *g.Id)
		status, data := doRequest(t, http.MethodDelete, "/groups/"+g.Id.String(), nil)
		require.Equal(t, http.StatusConflict, status, "%s", string(data))
		assert.Equal(t, gen.ErrorCodeConflict, decode[gen.Error](t, data).Code)
	})

	t.Run("409 deleting group with children", func(t *testing.T) {
		parent := createGroup(t, "parent", nil)
		createGroup(t, "child", parent.Id)
		status, data := doRequest(t, http.MethodDelete, "/groups/"+parent.Id.String(), nil)
		require.Equal(t, http.StatusConflict, status, "%s", string(data))
		assert.Equal(t, gen.ErrorCodeConflict, decode[gen.Error](t, data).Code)
	})

	t.Run("409 on cyclic parent assignment", func(t *testing.T) {
		root := createGroup(t, "cyc-root", nil)
		child := createGroup(t, "cyc-child", root.Id)
		// Попытка сделать root потомком собственного ребёнка — цикл.
		status, data := doRequest(t, http.MethodPut, "/groups/"+root.Id.String(), gen.UpdateGroupRequest{
			Name:     "cyc-root",
			ParentId: child.Id,
		})
		require.Equal(t, http.StatusConflict, status, "%s", string(data))
		assert.Equal(t, gen.ErrorCodeConflict, decode[gen.Error](t, data).Code)
	})
}

// uuidNew генерирует случайный UUID для ссылок на заведомо отсутствующие
// ресурсы. openapi_types.UUID — псевдоним uuid.UUID.
func uuidNew() openapi_types.UUID {
	return uuid.New()
}
