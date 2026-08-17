FROM golang:1.26-alpine AS builder

WORKDIR /app

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Собираем конкретно ваш архивер
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o suno-archiver ./cmd/suno-archiver

# Финальный образ
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata python3 py3-pip ffmpeg

# Копируем бинарник
COPY --from=builder /app/suno-archiver .

# Копируем анализатор (python-скрипты + зависимости)
COPY analyzer/ /root/analyzer/

# Устанавливаем зависимости анализатора в системный python
WORKDIR /root/analyzer
RUN pip3 install --no-cache-dir -r requirements.txt

WORKDIR /root/

# Анализ использует системный python, а не venv
ENV SUNO_PYTHON_BIN=/usr/bin/python3

# По умолчанию запускаем команду serve
CMD ["./suno-archiver", "serve"]