package httpapi

import (
	"groups-control/internal/adapters/http/gen"
	"groups-control/internal/domain"
	"groups-control/internal/usecase"
)

// Параметры пагинации списков по умолчанию и их верхняя граница.
// Значения синхронизированы с OpenAPI-контрактом (LimitParam: default 20,
// maximum 100).
const (
	defaultLimit = 20
	maxLimit     = 100
)

// pagination нормализует параметры пагинации из запроса: подставляет значения по
// умолчанию и ограничивает limit разумным максимумом.
func pagination(limit *gen.LimitParam, offset *gen.OffsetParam) usecase.Pagination {
	l := defaultLimit
	if limit != nil {
		l = int(*limit)
	}
	if l <= 0 {
		l = defaultLimit
	}
	if l > maxLimit {
		l = maxLimit
	}

	o := 0
	if offset != nil && *offset > 0 {
		o = int(*offset)
	}

	return usecase.Pagination{Limit: l, Offset: o}
}

// groupToDTO преобразует доменную группу в сгенерированный DTO.
func groupToDTO(g *domain.Group) gen.Group {
	return gen.Group{
		Id:        &g.ID,
		Name:      g.Name,
		ParentId:  g.ParentID,
		CreatedAt: &g.CreatedAt,
		UpdatedAt: &g.UpdatedAt,
	}
}

// groupWithCountsToDTO преобразует группу с подсчётом участников в DTO.
func groupWithCountsToDTO(gc usecase.GroupWithCounts) gen.GroupWithCounts {
	g := gc.Group
	return gen.GroupWithCounts{
		Id:          &g.ID,
		Name:        g.Name,
		ParentId:    g.ParentID,
		CreatedAt:   &g.CreatedAt,
		UpdatedAt:   &g.UpdatedAt,
		DirectCount: int64(gc.Counts.Direct),
		TotalCount:  int64(gc.Counts.Total),
	}
}

// groupListToDTO собирает страницу групп с подсчётами в ответ списка.
func groupListToDTO(items []usecase.GroupWithCounts, total int, page usecase.Pagination) gen.GroupList {
	dto := make([]gen.GroupWithCounts, 0, len(items))
	for i := range items {
		dto = append(dto, groupWithCountsToDTO(items[i]))
	}
	return gen.GroupList{
		Items:      dto,
		Pagination: paginationToDTO(total, page),
	}
}

// personToDTO преобразует доменного человека в сгенерированный DTO.
func personToDTO(p *domain.Person) gen.Person {
	return gen.Person{
		Id:        &p.ID,
		FirstName: p.FirstName,
		LastName:  p.LastName,
		BirthYear: int32(p.BirthYear),
		GroupId:   p.GroupID,
		CreatedAt: &p.CreatedAt,
		UpdatedAt: &p.UpdatedAt,
	}
}

// personListToDTO собирает страницу людей в ответ списка.
func personListToDTO(items []domain.Person, total int, page usecase.Pagination) gen.PersonList {
	dto := make([]gen.Person, 0, len(items))
	for i := range items {
		dto = append(dto, personToDTO(&items[i]))
	}
	return gen.PersonList{
		Items:      dto,
		Pagination: paginationToDTO(total, page),
	}
}

// paginationToDTO формирует метаданные пагинации для ответа.
func paginationToDTO(total int, page usecase.Pagination) gen.Pagination {
	return gen.Pagination{
		Limit:  int32(page.Limit),
		Offset: int32(page.Offset),
		Total:  int64(total),
	}
}
