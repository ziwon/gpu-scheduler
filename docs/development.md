# Development Guide

## Prerequisites

- Go 1.22 or later
- Docker
- Kubernetes cluster (kind recommended for local development)
- kubectl
- Helm 3

## Project Structure

```
gpu-scheduler/
├── api/v1/                    # CRD type definitions
│   ├── gpuclaim_types.go
│   └── gpunodestatus_types.go
├── cmd/                       # Entry points
│   ├── scheduler/main.go      # Scheduler binary
│   ├── webhook/main.go        # Webhook binary
│   └── agent/main.go          # Agent binary
├── internal/
│   ├── plugin/gpuclaim/       # Scheduler plugin implementation
│   ├── lease/                 # GPU lease management
│   ├── topo/                  # Topology scoring logic
│   └── util/                  # Shared utilities
├── charts/gpu-scheduler/      # Helm chart
└── hack/                      # Development scripts
```

## Building

### Build all Docker images

```bash
# Scheduler
make docker

# Webhook
make docker-webhook

# Agent
make docker-agent
```

### Build specific component

The Dockerfile uses `CMD_PATH` build argument:

```bash
# Custom image tags
docker build --build-arg CMD_PATH=cmd/scheduler \
  -t my-registry/gpu-scheduler:dev .

docker build --build-arg CMD_PATH=cmd/webhook \
  -t my-registry/gpu-webhook:dev .

docker build --build-arg CMD_PATH=cmd/agent \
  -t my-registry/gpu-agent:dev .
```

### Build locally (without Docker)

```bash
# Scheduler
go build -o bin/scheduler ./cmd/scheduler

# Webhook
go build -o bin/webhook ./cmd/webhook

# Agent
go build -o bin/agent ./cmd/agent
```

## Local Development

### Setup kind cluster

```bash
# Create cluster with GPU support (requires nvidia-docker)
kind create cluster --config hack/kind-cluster.yaml

# Or basic cluster for testing
kind create cluster --name gpu-test
```

### Deploy to kind

```bash
# Build and load images into kind
make docker
kind load docker-image ghcr.io/ziwon/gpu-scheduler:dev --name gpu-test

make docker-webhook
kind load docker-image ghcr.io/ziwon/gpu-scheduler-webhook:dev --name gpu-test

make docker-agent
kind load docker-image ghcr.io/ziwon/gpu-scheduler-agent:dev --name gpu-test

# Deploy
kubectl apply -f charts/gpu-scheduler/templates/crds.yaml
helm install gpu-scheduler charts/gpu-scheduler
```

### Quick iteration loop

```bash
# 1. Make code changes
vim internal/plugin/gpuclaim/plugin.go

# 2. Rebuild
make docker

# 3. Reload into kind
kind load docker-image ghcr.io/ziwon/gpu-scheduler:dev --name gpu-test

# 4. Restart pod
kubectl rollout restart deployment gpu-scheduler

# 5. Check logs
kubectl logs -f deployment/gpu-scheduler
```

## Testing

### Run unit tests

```bash
go test ./...
```

### Run tests with coverage

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test specific package

```bash
go test ./internal/lease -v
go test ./internal/plugin/gpuclaim -v
```

### Integration testing

Create test workloads:

```bash
# Apply test claim
kubectl apply -f - <<EOF
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: test-claim
spec:
  devices:
    count: 1
    exclusivity: Exclusive
EOF

# Create test pod
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  annotations:
    gpu.scheduling/claim: test-claim
spec:
  schedulerName: gpu-scheduler
  restartPolicy: Never
  containers:
    - name: test
      image: busybox
      command: ["sh", "-c", "echo CUDA_VISIBLE_DEVICES=\$CUDA_VISIBLE_DEVICES; sleep 30"]
EOF

# Check results
kubectl logs test-pod
kubectl get pod test-pod -o jsonpath='{.metadata.annotations}'
```

## Debugging

### View scheduler logs

```bash
kubectl logs -f deployment/gpu-scheduler

# With verbosity
kubectl logs -f deployment/gpu-scheduler | grep "v=4"
```

### View webhook logs

```bash
kubectl logs -f deployment/gpu-scheduler-webhook
```

### View agent logs

```bash
# All agents
kubectl logs -f daemonset/gpu-scheduler-agent

# Specific node
kubectl logs -f daemonset/gpu-scheduler-agent -n default --selector=name=gpu-scheduler-agent --field-selector=spec.nodeName=node-a
```

### Enable debug mode

Edit the deployment to increase log verbosity:

```bash
kubectl edit deployment gpu-scheduler
```

Add to container args:
```yaml
args:
  - --v=4  # Kubernetes logging verbosity
```

### Inspect CRD status

```bash
# List all claims with status
kubectl get gpuclaim -o yaml

# Specific claim
kubectl get gpuclaim my-claim -o jsonpath='{.status}'

# Watch claim status changes
kubectl get gpuclaim -w
```

### Debug webhook

Test webhook locally:

```bash
# Port forward
kubectl port-forward svc/gpu-scheduler-webhook 8443:443

# Send test admission request (requires valid cert)
curl -k https://localhost:8443/mutate -d @test-admission-request.json
```

### Check lease state

```bash
# All GPU leases
kubectl get leases | grep gpu-

# Lease details
kubectl get lease gpu-node-a-0 -o yaml

# Watch lease creation/deletion
kubectl get leases -w | grep gpu-
```

## Adding Features

### Adding a new allocation policy

1. Update the GpuClaim type in `api/v1/gpuclaim_types.go`:
   ```go
   type DeviceRequest struct {
       Policy string `json:"policy,omitempty"` // Add "newpolicy" to comment
   }
   ```

2. Implement logic in `internal/topo/topology.go`:
   ```go
   func ScoreNewPolicy(devs []DeviceInfo, count int) (score int, pick []int) {
       // Your scoring logic
   }
   ```

3. Update scheduler plugin in `internal/plugin/gpuclaim/plugin.go`:
   ```go
   func (p *Plugin) Score(ctx context.Context, ...) (int64, *framework.Status) {
       // Call your new scoring function based on policy
   }
   ```

4. Test:
   ```bash
   go test ./internal/topo
   make docker
   kind load docker-image ghcr.io/ziwon/gpu-scheduler:dev
   kubectl rollout restart deployment gpu-scheduler
   ```

### Implementing NVML integration

The agent currently uses placeholder GPU data. To integrate NVML:

1. Add NVML dependency to `go.mod`:
   ```bash
   go get github.com/NVIDIA/go-nvml/pkg/nvml
   ```

2. Update `cmd/agent/main.go`:
   ```go
   import "github.com/NVIDIA/go-nvml/pkg/nvml"

   func discoverDevices() []apiv1.Device {
       nvml.Init()
       defer nvml.Shutdown()

       count, _ := nvml.DeviceGetCount()
       devices := make([]apiv1.Device, count)

       for i := 0; i < count; i++ {
           device, _ := nvml.DeviceGetHandleByIndex(i)
           // Populate device info
       }

       return devices
   }
   ```

3. Update Dockerfile to include NVML library:
   ```dockerfile
   FROM nvidia/cuda:12.4.1-base-ubuntu22.04 AS build
   # Install NVML headers
   ```

### Adding CRD fields

1. Update types in `api/v1/`:
   ```go
   type GpuClaimSpec struct {
       NewField string `json:"newField,omitempty"`
   }
   ```

2. Regenerate CRD manifests (requires controller-gen):
   ```bash
   controller-gen crd paths=./api/v1 output:crd:dir=./charts/gpu-scheduler/templates
   ```

3. Apply updated CRDs:
   ```bash
   kubectl apply -f charts/gpu-scheduler/templates/crds.yaml
   ```

## Code Style

### Go conventions

- Use `gofmt` for formatting (automatically applied by editors)
- Follow [Effective Go](https://go.dev/doc/effective_go)
- Keep functions small and focused
- Add godoc comments to exported functions

### Linting

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run
```

## Common Pitfalls

### Pod stuck in Pending with no errors

Check if scheduler is running with correct schedulerName:
```bash
kubectl get pods -o jsonpath='{.items[*].spec.schedulerName}'
```

### Webhook not mutating pods

Verify webhook is registered:
```bash
kubectl get mutatingwebhookconfiguration
kubectl get mutatingwebhookconfiguration gpu-scheduler-webhook -o yaml
```

Check webhook service and endpoints:
```bash
kubectl get svc gpu-scheduler-webhook
kubectl get endpoints gpu-scheduler-webhook
```

### Leases not cleaned up

Leases don't auto-delete when pods are removed. Options:

1. Add finalizers to pods
2. Implement garbage collection controller
3. Use lease duration/renewals
4. Manual cleanup in development

### Scheduler plugin not loaded

Verify plugin registration in logs:
```bash
kubectl logs deployment/gpu-scheduler | grep GpuClaimPlugin
```

Should see: `"Registered plugin" plugin="GpuClaimPlugin"`

## Release Process

1. Update version in `charts/gpu-scheduler/Chart.yaml`
2. Build and tag images:
   ```bash
   make docker SCHED_IMG=ghcr.io/ziwon/gpu-scheduler:v0.1.0
   make docker-webhook WEBHOOK_IMG=ghcr.io/ziwon/gpu-scheduler-webhook:v0.1.0
   make docker-agent AGENT_IMG=ghcr.io/ziwon/gpu-scheduler-agent:v0.1.0
   ```
3. Push images
4. Package Helm chart:
   ```bash
   helm package charts/gpu-scheduler
   ```
5. Create GitHub release with chart tarball

## Getting Help

- Check logs first: scheduler, webhook, and agent
- Search issues on GitHub
- Enable debug logging (`--v=4`)
- Use `kubectl describe` on pods and claims
