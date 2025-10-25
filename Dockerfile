# Multi-stage build for minimal image size
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

WORKDIR /build

# Copy source code first (needed for local module resolution)
COPY . .

# Initialize and download dependencies
RUN go mod tidy && go mod download

# Build with optimizations
RUN CGO_ENABLED=1 GOOS=linux go build -a \
    -ldflags="-s -w" \
    -o proxy-checker ./cmd/main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 proxychecker && \
    adduser -D -u 1000 -G proxychecker proxychecker

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/proxy-checker .
COPY --from=builder /build/config.example.json ./config.json

# Create data directory
RUN mkdir -p /data && chown -R proxychecker:proxychecker /data /app

# Switch to non-root user
USER proxychecker

# Expose API port
EXPOSE 8083

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8083/health || exit 1

# Run the application
CMD ["./proxy-checker"]

