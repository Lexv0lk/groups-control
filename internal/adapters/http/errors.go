package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"log/slog"

	"groups-control/internal/adapters/http/gen"
	"groups-control/internal/domain"
)

// errEmptyBody — тело запроса обязательно, но не передано.
var errEmptyBody = errors.New("request body is required")

// errorBody конструирует единое тело ошибки; details опускается, если пуст.
func errorBody(code gen.ErrorCode, message string, details []gen.ErrorDetail) gen.Error {
	body := gen.Error{Code: code, Message: message}
	if len(details) > 0 {
		body.Details = &details
	}
	return body
}

// writeJSON сериализует тело ответа с указанным статусом.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// mapError классифицирует ошибку в HTTP-статус и тело ответа. Доменные
// sentinel-ошибки сопоставляются с кодами 404/409/422/400, всё остальное —
// с 500 (без раскрытия деталей наружу).
func mapError(err error) (int, gen.Error) {
	var vErr *domain.ValidationError
	switch {
	case errors.As(err, &vErr):
		details := make([]gen.ErrorDetail, 0, len(vErr.Fields))
		for _, f := range vErr.Fields {
			details = append(details, gen.ErrorDetail{Field: f.Field, Message: f.Message})
		}
		return http.StatusUnprocessableEntity, errorBody(gen.ErrorCodeValidationError, "validation failed", details)
	case errors.Is(err, domain.ErrValidation):
		return http.StatusUnprocessableEntity, errorBody(gen.ErrorCodeValidationError, err.Error(), nil)
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, errorBody(gen.ErrorCodeNotFound, "resource not found", nil)
	case errors.Is(err, domain.ErrCyclicParent),
		errors.Is(err, domain.ErrGroupHasChildren),
		errors.Is(err, domain.ErrGroupHasPeople):
		return http.StatusConflict, errorBody(gen.ErrorCodeConflict, err.Error(), nil)
	case errors.Is(err, errEmptyBody):
		return http.StatusBadRequest, errorBody(gen.ErrorCodeBadRequest, err.Error(), nil)
	default:
		return http.StatusInternalServerError, errorBody(gen.ErrorCodeInternalError, "internal server error", nil)
	}
}

// responseErrorHandler маппит ошибки, возвращённые обработчиками, в HTTP-ответы.
// Серверные ошибки (5xx) логируются с деталями; наружу детали не уходят.
func responseErrorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		status, body := mapError(err)
		if status >= http.StatusInternalServerError {
			logger.ErrorContext(r.Context(), "request failed",
				slog.String("error", err.Error()),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
		}
		writeJSON(w, status, body)
	}
}

// requestErrorHandler формирует ответ 400 для ошибок разбора запроса (битый JSON,
// неверный формат path/query-параметров).
func requestErrorHandler() func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, _ *http.Request, err error) {
		writeJSON(w, http.StatusBadRequest, errorBody(gen.ErrorCodeBadRequest, "invalid request: "+err.Error(), nil))
	}
}
