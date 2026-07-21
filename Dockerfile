# Build stage
FROM golang:alpine AS builder

# Insta-l·la eines de compilació (gcc i musl-dev són necessaris per go-sqlite3)
RUN apk add --no-cache git tzdata ca-certificates gcc musl-dev

WORKDIR /app

# Copiar dependències
COPY go.mod go.sum ./
RUN go mod download

# Copiar el codi font
COPY . .

# Compilar el binari Go amb CGO actiu per al session store SQLite de whatsmeow
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o /app/bot ./cmd/bot

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache tzdata ca-certificates sqlite-dev python3 py3-requests

WORKDIR /app

# Copiar el binari i l'script de subprocés MCP des del builder
COPY --from=builder /app/bot /app/bot
COPY --from=builder /app/internal/infrastructure/trainingpeaks/tp_bridge.py /app/internal/infrastructure/trainingpeaks/tp_bridge.py
COPY --from=builder /app/internal/infrastructure/bondia/bondia_bridge.py /app/internal/infrastructure/bondia/bondia_bridge.py

CMD ["/app/bot"]

