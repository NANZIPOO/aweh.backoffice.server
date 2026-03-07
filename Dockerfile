# Multi-stage Docker build for Aweh.POS Gateway
# Stage 1: Build the Go binary
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with version info
# These can be overridden at build time with --build-arg
ARG VERSION=1.0.0
ARG BUILD_NUMBER=1
ARG GIT_HASH=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o gateway main.go

# Stage 2: Create minimal runtime image
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 aweh && \
    adduser -D -u 1000 -G aweh aweh

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/gateway .
COPY --from=builder /build/version.json . 

# Copy migrations
COPY --from=builder /build/migrations ./migrations

# Set ownership
RUN chown -R aweh:aweh /app

# Switch to non-root user
USER aweh

# Expose port (default 8081)
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/api/v1/version || exit 1

# Run the gateway
ENTRYPOINT ["./gateway"]
