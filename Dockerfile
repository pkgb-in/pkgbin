# Build stage
FROM golang:1.24.2-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux go build -o /npm_cache ./cmd/npm_cache
RUN CGO_ENABLED=0 GOOS=linux go build -o /ruby_cache ./cmd/ruby_cache
RUN CGO_ENABLED=0 GOOS=linux go build -o /python_cache ./cmd/python_cache

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy built binaries from builder
COPY --from=builder /npm_cache /app/npm_cache
COPY --from=builder /ruby_cache /app/ruby_cache
COPY --from=builder /python_cache /app/python_cache

# Copy migration files (needed if you want to run migrations)
COPY db/migrations /app/db/migrations

# Copy static files
COPY static /app/static

# Create cache directories_data /app/pypi_cache_data
RUN mkdir -p /app/npm_cache_data /app/gem_cache

# Expose default port
EXPOSE 8080

# Default command (will be overridden in docker-compose)
CMD ["/app/npm_cache"]
