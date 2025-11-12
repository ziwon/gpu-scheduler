# API Reference

## GpuClaim

A GpuClaim defines a declarative GPU allocation request.

### Resource Info

- **API Group**: `gpu.scheduling/v1`
- **Kind**: `GpuClaim`
- **Scope**: Namespaced
- **Short Name**: `gclaim`

### Spec

#### `devices` (required)

Describes GPU requirements.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `count` | int | Number of GPUs needed | `2` |
| `policy` | string | Allocation strategy: `contiguous`, `spread`, or `preferIds` | `"contiguous"` |
| `preferIds` | []int | Specific GPU IDs to prefer (used with `preferIds` policy) | `[0, 1]` |
| `exclusivity` | string | Sharing mode: `Exclusive`, `Shared`, or `MIG` | `"Exclusive"` |

**Policy Details**:
- `contiguous`: Allocate GPUs with adjacent IDs (0,1,2 not 0,2,4). Best for workloads with GPU-to-GPU communication.
- `spread`: Spread GPUs across different islands/buses. Best for independent parallel tasks.
- `preferIds`: Try to allocate specific GPU IDs. Falls back if not available.

**Exclusivity Details**:
- `Exclusive`: GPU dedicated to one pod (recommended)
- `Shared`: Multiple pods can share GPU (no isolation guarantees)
- `MIG`: Multi-Instance GPU mode (not yet implemented)

#### `selector` (optional)

Node selector to target specific nodes.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `matchLabels` | map[string]string | Label selector for nodes | `{"gpu-type": "a100"}` |

#### `topology` (optional)

NVLink bandwidth preferences.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `mode` | string | Requirement level: `Required`, `Preferred`, or `Ignore` | `"Preferred"` |
| `minBandwidthGBps` | int | Minimum interconnect bandwidth | `600` |

**Mode Details**:
- `Required`: Pod won't schedule if topology requirements not met
- `Preferred`: Try to meet requirements, schedule anyway if not possible
- `Ignore`: Don't consider topology

#### `gangRef` (optional)

Reference to a gang/pod-group for multi-pod scheduling.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `gangRef` | string | Name of pod group | `"training-job-123"` |

**Status**: Not implemented in MVP

### Status

Reflects scheduler progress.

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `phase` | string | Current state: `Pending`, `Reserved`, `Bound`, or `Failed` | `"Bound"` |
| `nodeName` | string | Node where GPUs allocated | `"node-a"` |
| `gpuIds` | []int | Allocated GPU IDs | `[0, 1]` |
| `allocated` | string | Combined node and GPU info | `"node-a:0,1"` |
| `message` | string | Human-readable status message | `"Successfully allocated"` |

### Examples

#### Basic single GPU

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: single-gpu
  namespace: default
spec:
  devices:
    count: 1
    exclusivity: Exclusive
```

#### Multi-GPU with topology

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: training-gpus
  namespace: ml-workloads
spec:
  devices:
    count: 4
    policy: contiguous
    exclusivity: Exclusive
  topology:
    mode: Preferred
    minBandwidthGBps: 600
  selector:
    matchLabels:
      gpu-type: a100
      nvlink: "true"
```

#### Pinned GPU IDs

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: specific-gpus
spec:
  devices:
    count: 2
    policy: preferIds
    preferIds: [2, 3]
    exclusivity: Exclusive
```

---

## GpuNodeStatus

Reports per-node GPU inventory and health. Created and updated by the agent DaemonSet.

### Resource Info

- **API Group**: `gpu.scheduling/v1`
- **Kind**: `GpuNodeStatus`
- **Scope**: Cluster
- **Short Name**: `gns`

### Spec

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `nodeName` | string | Kubernetes node name | `"node-a"` |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `devices` | []Device | List of GPU devices on node |
| `total` | int | Total number of GPUs |

#### Device Object

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `id` | int | GPU device ID | `0` |
| `inUseBy` | []string | Pod UIDs using this GPU | `["abc-123", "def-456"]` |
| `health` | string | Health status: `Healthy`, `Unhealthy`, or `Unknown` | `"Healthy"` |
| `bandwidthGBps` | int | NVLink bandwidth to peers | `600` |
| `island` | string | NVLink island identifier | `"nvlink-group-0"` |

**Island**: GPUs in the same island have high-speed interconnect (NVLink). GPUs in different islands communicate through PCIe (slower).

### Example

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuNodeStatus
metadata:
  name: node-a
spec:
  nodeName: node-a
status:
  total: 8
  devices:
    - id: 0
      health: Healthy
      bandwidthGBps: 600
      island: nvlink-group-0
      inUseBy: ["pod-abc-123"]
    - id: 1
      health: Healthy
      bandwidthGBps: 600
      island: nvlink-group-0
      inUseBy: []
    - id: 2
      health: Healthy
      bandwidthGBps: 600
      island: nvlink-group-0
      inUseBy: []
    - id: 3
      health: Healthy
      bandwidthGBps: 600
      island: nvlink-group-0
      inUseBy: []
    - id: 4
      health: Healthy
      bandwidthGBps: 64
      island: nvlink-group-1
      inUseBy: []
    - id: 5
      health: Healthy
      bandwidthGBps: 64
      island: nvlink-group-1
      inUseBy: []
    - id: 6
      health: Healthy
      bandwidthGBps: 64
      island: nvlink-group-1
      inUseBy: []
    - id: 7
      health: Unhealthy
      bandwidthGBps: 0
      island: nvlink-group-1
      inUseBy: []
```

In this example:
- GPUs 0-3 are in one NVLink island (600 GB/s interconnect)
- GPUs 4-7 are in another island (64 GB/s interconnect)
- GPU 7 is unhealthy and shouldn't be allocated

---

## Pod Annotations

### `gpu.scheduling/claim`

**Set by**: User
**Read by**: Scheduler
**Purpose**: Links a pod to a GpuClaim

**Example**:
```yaml
metadata:
  annotations:
    gpu.scheduling/claim: my-gpu-request
```

### `gpu.scheduling/allocated`

**Set by**: Scheduler (PreBind phase)
**Read by**: Webhook
**Purpose**: Tells webhook which GPUs were allocated

**Format**: `{nodeName}:{comma-separated-gpu-ids}`

**Examples**:
- `node-a:0` (single GPU)
- `node-b:0,1,2,3` (multiple GPUs)

---

## Leases

The scheduler uses Kubernetes Coordination Leases for atomic GPU locking.

### Lease Naming

Format: `gpu-{nodeName}-{gpuId}`

Examples:
- `gpu-node-a-0`
- `gpu-node-b-3`

### Lease Spec

| Field | Type | Description |
|-------|------|-------------|
| `holderIdentity` | string | Pod UID that owns the GPU |

### Lease Lifecycle

1. **Creation**: Scheduler creates lease in Reserve phase
2. **Ownership**: Pod UID stored in `holderIdentity`
3. **Deletion**: Scheduler deletes lease in Unreserve phase (on failure) or manually

**Note**: Leases currently don't auto-delete when pods are removed. This is a known limitation.

### Example

```yaml
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  name: gpu-node-a-0
  namespace: default
spec:
  holderIdentity: "abc-123-def-456"  # Pod UID
```

---

## Scheduler Configuration

The scheduler is configured via KubeSchedulerConfiguration.

### Plugin Phases

The GpuClaimPlugin runs in these phases:

| Phase | Purpose |
|-------|---------|
| PreFilter | Read claim annotation, validate request |
| Filter | Check node selector (currently no-op) |
| Score | Rank nodes by GPU availability and topology |
| Reserve | Atomically acquire GPU leases |
| Unreserve | Release leases on failure |
| PreBind | Annotate pod with allocation |

### Example Configuration

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
profiles:
  - schedulerName: gpu-scheduler
    plugins:
      preFilter:
        enabled:
          - name: GpuClaimPlugin
      filter:
        enabled:
          - name: GpuClaimPlugin
      score:
        enabled:
          - name: GpuClaimPlugin
      reserve:
        enabled:
          - name: GpuClaimPlugin
      preBind:
        enabled:
          - name: GpuClaimPlugin
```

---

## Webhook Configuration

### MutatingWebhookConfiguration

The webhook mutates pods that have the `gpu.scheduling/allocated` annotation.

**Endpoint**: `/mutate`
**Port**: 8443 (HTTPS)
**Failure Policy**: Fail (pod won't be created if webhook fails)

### What Gets Injected

The webhook adds `CUDA_VISIBLE_DEVICES` environment variable to **all containers** in the pod.

**Example**:
```yaml
containers:
  - name: training
    env:
      - name: CUDA_VISIBLE_DEVICES
        value: "0,1,2"
```

This tells CUDA runtime which GPUs the container can see.

---

## CLI Reference

### kubectl commands

```bash
# List claims
kubectl get gpuclaim
kubectl get gclaim  # short form

# Describe claim
kubectl describe gpuclaim my-claim

# Get claim status
kubectl get gpuclaim my-claim -o jsonpath='{.status}'

# List node GPU status
kubectl get gpunodestatus
kubectl get gns  # short form

# Get detailed node GPU info
kubectl get gns node-a -o yaml

# List GPU leases
kubectl get leases | grep gpu-

# Delete specific lease
kubectl delete lease gpu-node-a-0

# Watch claims
kubectl get gclaim -w
```

### Helm commands

```bash
# Install
helm install gpu-scheduler charts/gpu-scheduler

# Install with custom values
helm install gpu-scheduler charts/gpu-scheduler \
  --set scheduler.image.tag=v0.2.0

# Upgrade
helm upgrade gpu-scheduler charts/gpu-scheduler

# Uninstall
helm uninstall gpu-scheduler

# View values
helm get values gpu-scheduler
```
