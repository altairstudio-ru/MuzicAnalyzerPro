FROM golang:1.21-alpine AS builder

WORKDIR /app

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Собираем конкретно ваш архивер
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o suno-archiver ./cmd/suno-archiver

# Финальный образ
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/suno-archiver .

# По умолчанию запускаем команду serve
CMD ["./suno-archiver", "serve"]
