# syntax=docker/dockerfile:1

# --- Builder -----------------------------------------------------------------
# Многостадийная сборка: на этом этапе компилируем статический бинарник,
# чтобы финальный образ не тащил тулчейн Go и исходники.
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Сначала только манифесты — слой с зависимостями кешируется отдельно и не
# инвалидируется при изменении исходного кода.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Затем остальной код.
COPY . .

# CGO выключен → полностью статический бинарник, который работает в минимальном
# рантайме. Символы и таблицу отладки выкидываем (-s -w) ради размера.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath -ldflags="-s -w" \
        -o /out/server ./cmd/server

# --- Runtime -----------------------------------------------------------------
# Лёгкий alpine-рантайм: даёт busybox-утилиты (wget) для контейнерного
# healthcheck и позволяет запускаться под непривилегированным пользователем.
FROM alpine:3.20 AS runtime

# Корневые сертификаты — на случай исходящих TLS-соединений.
RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app -u 10001 app

WORKDIR /app

COPY --from=builder /out/server /app/server

USER app:app

EXPOSE 8080

ENTRYPOINT ["/app/server"]
