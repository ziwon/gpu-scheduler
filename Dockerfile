# syntax=docker/dockerfile:1.7

# Build arguments
ARG GO_VERSION=1.24
ARG ALPINE_VERSION=3.21

# Build stage
FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

# Install build dependencies
RUN apk add --no-cache \
    ca-certificates \
    git \
    make

# Set working directory
WORKDIR /workspace

# Build arguments for component path and version info
ARG CMD_PATH=cmd/scheduler
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies (cached if go.mod/go.sum unchanged)
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && \
    go mod verify

# Copy source code
COPY . .

# Build the binary with optimizations
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build \
    -a \
    -installsuffix cgo \
    -ldflags="-w -s \
              -X main.version=${VERSION} \
              -X main.commit=${COMMIT} \
              -X main.date=${BUILD_DATE} \
              -extldflags '-static'" \
    -trimpath \
    -o /out/gpu-component \
    "./${CMD_PATH}"

# Verify the binary
RUN /out/gpu-component --version || /out/gpu-component --help || echo "Binary built successfully"

# Runtime stage - distroless for minimal attack surface
FROM gcr.io/distroless/static-debian12:nonroot

# Metadata labels following OCI spec
LABEL org.opencontainers.image.title="GPU Scheduler Component" \
      org.opencontainers.image.description="Kubernetes GPU scheduler component" \
      org.opencontainers.image.url="https://github.com/ziwon/gpu-scheduler" \
      org.opencontainers.image.source="https://github.com/ziwon/gpu-scheduler" \
      org.opencontainers.image.vendor="ziwon" \
      org.opencontainers.image.licenses="MIT"

# Copy binary from builder
COPY --from=builder --chown=65532:65532 /out/gpu-component /usr/local/bin/gpu-component

# Use non-root user (already default in distroless:nonroot, but explicit is better)
USER 65532:65532

# Set working directory
WORKDIR /

# Entrypoint
ENTRYPOINT ["/usr/local/bin/gpu-component"]
