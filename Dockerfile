# Build stage
FROM golang:1.23-alpine AS builder

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

RUN apk add --no-cache tzdata ca-certificates sqlite-dev

WORKDIR /app

# Copiar el binari des del builder
COPY --from=builder /app/bot /app/bot

CMD ["/app/bot"]
