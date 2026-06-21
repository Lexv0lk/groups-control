package httpapi

import (
	"context"
	"net/http"
	"time"

	"log/slog"

	"github.com/google/uuid"

	"groups-control/internal/adapters/http/gen"
)

// requestIDHeader — заголовок для сквозного идентификатора запроса.
const requestIDHeader = "X-Request-Id"

// contextKey — приватный тип ключа контекста, исключающий коллизии.
type contextKey string

const requestIDKey contextKey = "request-id"

// RequestIDFromContext возвращает идентификатор запроса, если он установлен
// requestIDMiddleware, иначе — пустую строку.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// requestIDMiddleware принимает идентификатор запроса из заголовка либо
// генерирует новый, кладёт его в контекст и возвращает в ответе.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder перехватывает код ответа для структурного логирования.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rec *statusRecorder) WriteHeader(code int) {
	if !rec.wroteHeader {
		rec.status = code
		rec.wroteHeader = true
	}
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	if !rec.wroteHeader {
		rec.WriteHeader(http.StatusOK)
	}
	return rec.ResponseWriter.Write(b)
}

// loggingMiddleware пишет структурный лог по каждому запросу: метод, путь,
// статус, длительность и идентификатор запроса.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", RequestIDFromContext(r.Context())),
			)
		})
	}
}

// recoverMiddleware ловит панику в обработчике, логирует её и отдаёт 500 в едином
// формате ошибки вместо обрыва соединения.
func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", rec),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("request_id", RequestIDFromContext(r.Context())),
					)
					writeJSON(w, http.StatusInternalServerError, errorBody(gen.ErrorCodeInternalError, "internal server error", nil))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// timeoutMiddleware ограничивает время обработки запроса, отменяя контекст по
// истечении дедлайна.
func timeoutMiddleware(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
