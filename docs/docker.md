# Docker Guide

## Building Images

### Quick Start

```bash
# Build all images
make docker-all

# Build specific component
make docker-scheduler
make docker-webhook
make docker-agent
```

### Custom Builds

```bash
# Build with specific version
make VERSION=v1.0.0 docker-all

# Build for different platform
make DOCKER_PLATFORM=linux/arm64 docker

# Build with custom registry
make REGISTRY=registry.in docker-all
```

## Dockerfile Optimizations

The Dockerfile uses modern best practices for optimal builds:

### 1. Multi-Stage Build

Two stages minimize final image size:
- **Builder stage**: Full Go toolchain (golang:alpine)
- **Runtime stage**: Minimal distroless image (~2MB)

### 2. Build Cache Optimization

```dockerfile
# Dependencies cached separately from source code
COPY go.mod go.sum ./
RUN go mod download

# Source copied after dependencies
COPY . .
```

Benefits:
- Dependencies only re-download when go.mod/go.sum change
- Faster rebuilds during development

### 3. BuildKit Cache Mounts

```dockerfile
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build ...
```

Features:
- Persistent cache across builds
- Shared cache between different builds
- 10-100x faster rebuilds

### 4. Binary Optimization Flags

```bash
CGO_ENABLED=0           # Static binary, no C dependencies
-ldflags="-w -s"        # Strip debug info and symbol table
-trimpath               # Remove file system paths from binary
-installsuffix cgo      # Better caching
```

Result:
- Smaller binary size (~50% reduction)
- No external dependencies
- Reproducible builds

### 5. Version Information

```dockerfile
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

-ldflags="-X main.version=${VERSION} \
          -X main.commit=${COMMIT} \
          -X main.date=${BUILD_DATE}"
```

Usage:
```bash
# Binaries include version info
./scheduler --version
```

### 6. Security Hardening

```dockerfile
# Minimal distroless base image
FROM gcr.io/distroless/static-debian12:nonroot

# Non-root user
USER 65532:65532
```

Benefits:
- No shell or package manager (reduced attack surface)
- Non-root execution
- CVE-free base (regularly updated)
- Minimal dependencies

### 7. OCI Metadata Labels

```dockerfile
LABEL org.opencontainers.image.title="GPU Scheduler"
      org.opencontainers.image.source="https://github.com/ziwon/gpu-scheduler"
      org.opencontainers.image.licenses="MIT"
```

Enables:
- Container registry metadata
- Image provenance tracking
- License compliance

## Image Sizes

Approximate sizes:

| Component | Size |
|-----------|------|
| Scheduler | ~72MB |
| Webhook   | ~12MB |
| Agent     | ~38MB |

Compare to typical Go apps with full base image: ~50-200MB

## Development Workflow

### Local Development

```bash
# Build and load into kind
make dev-docker

# Deploy to kind cluster
make deploy

# View logs
make logs
```

### Testing Changes

```bash
# Rebuild specific component
make docker-scheduler

# Reload in kind
kind load docker-image ghcr.io/ziwon/gpu-scheduler:dev

# Restart deployment
kubectl rollout restart deployment/gpu-scheduler
```

## Push to Registry

```bash
# Push all images
make docker-push-all

# Push specific component
make docker-push-scheduler

# Push with version tag
make VERSION=v1.0.0 docker-push-all
```

## Multi-Platform Builds

Build for multiple architectures:

```bash
# Create buildx builder
docker buildx create --name multiarch --use

# Build for multiple platforms
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg CMD_PATH=cmd/scheduler \
  --push \
  -t ghcr.io/ziwon/gpu-scheduler:latest .
```

Or use the Makefile:

```bash
# Set platform
export DOCKER_PLATFORM=linux/amd64,linux/arm64
make docker-all
```

## Debugging

### Inspect Image

```bash
# Check image size
docker images ghcr.io/ziwon/gpu-scheduler:dev

# Inspect layers
docker history ghcr.io/ziwon/gpu-scheduler:dev

# Check metadata
docker inspect ghcr.io/ziwon/gpu-scheduler:dev
```

### Build with Debug Info

For debugging, use non-distroless base:

```dockerfile
# Replace final stage with debug variant
FROM gcr.io/distroless/base-debian12:debug

# Or use full Alpine
FROM alpine:3.19
```

### Run Interactively

```bash
# Distroless doesn't have shell, but you can exec into it
docker run -it --entrypoint /bin/sh alpine:3.19
# Then copy binary and test
```

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Build and push images
  run: |
    echo "${{ secrets.GITHUB_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin
    make VERSION=${{ github.ref_name }} docker-push-all
```

### GitLab CI Example

```yaml
build:
  script:
    - docker login -u $CI_REGISTRY_USER -p $CI_REGISTRY_PASSWORD $CI_REGISTRY
    - make VERSION=$CI_COMMIT_TAG docker-push-all
```

## Best Practices

### 1. Use .dockerignore

Exclude unnecessary files from build context:

```
.git/
.github/
docs/
*.md
.cache/
.config/
bin/
coverage.*
```

### 2. Pin Base Image Versions

```dockerfile
# Good: Specific version
FROM golang:1.24-alpine3.21

# Bad: Latest (unpredictable)
FROM golang:latest
```

### 3. Leverage Build Cache

- Structure Dockerfile for optimal caching
- Use BuildKit cache mounts
- Order layers from least to most frequently changed

### 4. Security Scanning

```bash
# Scan for vulnerabilities
trivy image ghcr.io/ziwon/gpu-scheduler:dev

# Or use Docker Scout
docker scout cves ghcr.io/ziwon/gpu-scheduler:dev
```

### 5. Regular Updates

- Update base images regularly
- Rebuild weekly for security patches
- Use automated dependency updates (Dependabot)

## Troubleshooting

### Build Fails with "go mod download"

```bash
# Clear build cache
docker builder prune

# Verify go.mod/go.sum
go mod tidy
```

### Image Size Too Large

```bash
# Check what's taking space
docker history ghcr.io/ziwon/gpu-scheduler:dev --human --no-trunc

# Ensure using distroless final stage
# Check .dockerignore excludes unnecessary files
```

### BuildKit Features Not Working

```bash
# Enable BuildKit
export DOCKER_BUILDKIT=1

# Or in docker-compose
export COMPOSE_DOCKER_CLI_BUILD=1
export DOCKER_BUILDKIT=1
```

### Cross-Platform Build Issues

```bash
# Install qemu for emulation
docker run --privileged --rm tonistiigi/binfmt --install all

# Verify
docker buildx ls
```

## Performance Tips

1. **Use cache mounts**: 10-100x faster rebuilds
2. **Order layers properly**: Dependencies before source
3. **Minimize context size**: Good .dockerignore
4. **Parallel builds**: Build all components concurrently
5. **Registry caching**: Pull before building

## References

- [Dockerfile best practices](https://docs.docker.com/develop/dev-best-practices/)
- [Multi-stage builds](https://docs.docker.com/build/building/multi-stage/)
- [BuildKit](https://docs.docker.com/build/buildkit/)
- [Distroless images](https://github.com/GoogleContainerTools/distroless)
