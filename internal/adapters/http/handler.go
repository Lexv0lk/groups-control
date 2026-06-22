package httpapi

import (
	"context"
	"net/http"
	"time"

	"log/slog"

	"github.com/go-chi/chi/v5"

	"groups-control/internal/adapters/http/gen"
	"groups-control/internal/usecase"
)

// Pinger проверяет доступность инфраструктурных зависимостей (например, БД) для
// readiness-проб. Объявлен на стороне потребителя (HTTP-слоя) — конкретная
// реализация (pgxpool) инжектится в composition root.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler реализует сгенерированный gen.StrictServerInterface, делегируя
// бизнес-логику usecase-сервисам и преобразуя domain <-> сгенерированные DTO.
// Доменные ошибки возвращаются как есть — их маппинг в HTTP-статусы выполняет
// responseErrorHandler, настроенный в NewRouter.
type Handler struct {
	groups *usecase.GroupService
	people *usecase.PersonService
	pinger Pinger
}

// NewHandler создаёт HTTP-обработчик поверх usecase-сервисов.
func NewHandler(groups *usecase.GroupService, people *usecase.PersonService, pinger Pinger) *Handler {
	return &Handler{groups: groups, people: people, pinger: pinger}
}

var _ gen.StrictServerInterface = (*Handler)(nil)

// ListGroups возвращает страницу групп с подсчётом участников.
func (h *Handler) ListGroups(ctx context.Context, request gen.ListGroupsRequestObject) (gen.ListGroupsResponseObject, error) {
	page := pagination(request.Params.Limit, request.Params.Offset)
	items, total, err := h.groups.List(ctx, page)
	if err != nil {
		return nil, err
	}
	return gen.ListGroups200JSONResponse(groupListToDTO(items, total, page)), nil
}

// CreateGroup создаёт новую группу.
func (h *Handler) CreateGroup(ctx context.Context, request gen.CreateGroupRequestObject) (gen.CreateGroupResponseObject, error) {
	if request.Body == nil {
		return nil, errEmptyBody
	}
	group, err := h.groups.Create(ctx, request.Body.Name, request.Body.ParentId)
	if err != nil {
		return nil, err
	}
	return gen.CreateGroup201JSONResponse(groupToDTO(group)), nil
}

// DeleteGroup удаляет группу.
func (h *Handler) DeleteGroup(ctx context.Context, request gen.DeleteGroupRequestObject) (gen.DeleteGroupResponseObject, error) {
	if err := h.groups.Delete(ctx, request.Id); err != nil {
		return nil, err
	}
	return gen.DeleteGroup204Response{}, nil
}

// GetGroup возвращает одну группу с подсчётом участников (direct + total).
func (h *Handler) GetGroup(ctx context.Context, request gen.GetGroupRequestObject) (gen.GetGroupResponseObject, error) {
	group, err := h.groups.Get(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	counts, err := h.groups.Counts(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetGroup200JSONResponse(groupWithCountsToDTO(usecase.GroupWithCounts{Group: *group, Counts: counts})), nil
}

// UpdateGroup обновляет имя и/или родителя группы.
func (h *Handler) UpdateGroup(ctx context.Context, request gen.UpdateGroupRequestObject) (gen.UpdateGroupResponseObject, error) {
	if request.Body == nil {
		return nil, errEmptyBody
	}
	group, err := h.groups.Update(ctx, request.Id, request.Body.Name, request.Body.ParentId)
	if err != nil {
		return nil, err
	}
	return gen.UpdateGroup200JSONResponse(groupToDTO(group)), nil
}

// ListGroupPeople возвращает людей группы: только своих либо вместе с дочерними.
func (h *Handler) ListGroupPeople(ctx context.Context, request gen.ListGroupPeopleRequestObject) (gen.ListGroupPeopleResponseObject, error) {
	includeDescendants := false
	if request.Params.IncludeDescendants != nil {
		includeDescendants = *request.Params.IncludeDescendants
	}
	page := pagination(request.Params.Limit, request.Params.Offset)
	people, total, err := h.people.ListByGroup(ctx, request.Id, includeDescendants, page)
	if err != nil {
		return nil, err
	}
	return gen.ListGroupPeople200JSONResponse(personListToDTO(people, total, page)), nil
}

// CreatePerson создаёт человека в указанной группе.
func (h *Handler) CreatePerson(ctx context.Context, request gen.CreatePersonRequestObject) (gen.CreatePersonResponseObject, error) {
	if request.Body == nil {
		return nil, errEmptyBody
	}
	person, err := h.people.Create(ctx, request.Body.FirstName, request.Body.LastName, int(request.Body.BirthYear), request.Body.GroupId)
	if err != nil {
		return nil, err
	}
	return gen.CreatePerson201JSONResponse(personToDTO(person)), nil
}

// DeletePerson удаляет человека.
func (h *Handler) DeletePerson(ctx context.Context, request gen.DeletePersonRequestObject) (gen.DeletePersonResponseObject, error) {
	if err := h.people.Delete(ctx, request.Id); err != nil {
		return nil, err
	}
	return gen.DeletePerson204Response{}, nil
}

// GetPerson возвращает одного человека.
func (h *Handler) GetPerson(ctx context.Context, request gen.GetPersonRequestObject) (gen.GetPersonResponseObject, error) {
	person, err := h.people.Get(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetPerson200JSONResponse(personToDTO(person)), nil
}

// UpdatePerson обновляет данные человека (в т.ч. смену группы).
func (h *Handler) UpdatePerson(ctx context.Context, request gen.UpdatePersonRequestObject) (gen.UpdatePersonResponseObject, error) {
	if request.Body == nil {
		return nil, errEmptyBody
	}
	person, err := h.people.Update(ctx, request.Id, request.Body.FirstName, request.Body.LastName, int(request.Body.BirthYear), request.Body.GroupId)
	if err != nil {
		return nil, err
	}
	return gen.UpdatePerson200JSONResponse(personToDTO(person)), nil
}

// GetHealthz — liveness-проба: процесс жив и обслуживает запросы.
func (h *Handler) GetHealthz(_ context.Context, _ gen.GetHealthzRequestObject) (gen.GetHealthzResponseObject, error) {
	return gen.GetHealthz200JSONResponse{Status: "ok"}, nil
}

// GetReadyz — readiness-проба: проверяет доступность БД через Pinger.
func (h *Handler) GetReadyz(ctx context.Context, _ gen.GetReadyzRequestObject) (gen.GetReadyzResponseObject, error) {
	if err := h.pinger.Ping(ctx); err != nil {
		return gen.GetReadyz503JSONResponse(errorBody(gen.ErrorCodeInternalError, "database is not ready", nil)), nil
	}
	return gen.GetReadyz200JSONResponse{Status: "ok"}, nil
}

// NewRouter собирает chi-роутер: оборачивает strict-обработчик общими
// middleware (request-id, логирование, recover, таймаут), настраивает единый
// формат ошибок и монтирует маршруты из OpenAPI-контракта.
func NewRouter(h gen.StrictServerInterface, logger *slog.Logger, requestTimeout time.Duration) http.Handler {
	strict := gen.NewStrictHandlerWithOptions(h, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler(),
		ResponseErrorHandlerFunc: responseErrorHandler(logger),
	})

	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Use(recoverMiddleware(logger))
	r.Use(loggingMiddleware(logger))
	if requestTimeout > 0 {
		r.Use(timeoutMiddleware(requestTimeout))
	}

	// Единый формат ошибок и для непрописанных в контракте маршрутов/методов:
	// иначе chi отдаёт текстовый 404 и пустой 405 в обход конверта Error.
	r.NotFound(notFoundHandler())
	r.MethodNotAllowed(methodNotAllowedHandler(r))

	return gen.HandlerWithOptions(strict, gen.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: requestErrorHandler(),
	})
}
