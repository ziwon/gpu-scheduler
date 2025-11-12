# Architecture

## Overview

The GPU scheduler is a Kubernetes extension that provides smart GPU allocation for workloads. It has three main components that work together:

```
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│  Scheduler  │      │   Webhook   │      │    Agent    │
│   (Plugin)  │      │  (Mutator)  │      │ (DaemonSet) │
└─────────────┘      └─────────────┘      └─────────────┘
       │                    │                    │
       │                    │                    │
       ▼                    ▼                    ▼
┌──────────────────────────────────────────────────────┐
│              Kubernetes API Server                    │
│  - GpuClaim CRDs                                     │
│  - GpuNodeStatus CRDs                                │
│  - Coordination Leases (for GPU locking)             │
└──────────────────────────────────────────────────────┘
```

## How It Works

### Step 1: User Creates Resources

1. User creates a **GpuClaim** defining their GPU needs (e.g., "I need 2 GPUs")
2. User creates a **Pod** with an annotation pointing to that GpuClaim

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: my-gpu-request
spec:
  devices:
    count: 2
    policy: contiguous
---
apiVersion: v1
kind: Pod
metadata:
  name: my-workload
  annotations:
    gpu.scheduling/claim: my-gpu-request  # Links to the claim above
spec:
  schedulerName: gpu-scheduler  # Use our custom scheduler
  containers:
    - name: training
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
```

### Step 2: Scheduler Allocates GPUs

The scheduler plugin runs through several phases:

#### PreFilter Phase
- Reads the `gpu.scheduling/claim` annotation
- Validates the claim exists
- Stores request details (how many GPUs needed)

#### Filter Phase
- Checks which nodes match the requirements
- Currently allows all nodes (MVP)

#### Score Phase
- Ranks nodes based on GPU availability
- Prefers nodes with contiguous GPUs in the same NVLink island
- Currently returns static score (topology scoring TODO)

#### Reserve Phase (The Key Part!)
- **Atomically acquires GPU leases** on the chosen node
- For each GPU ID (0-15), tries to create a Kubernetes Lease object
- Lease name format: `gpu-{nodeName}-{gpuID}`
- If the lease already exists, that GPU is busy → try next ID
- If not enough GPUs available, rolls back all acquired leases

This is how we prevent double-booking GPUs!

#### PreBind Phase
- Adds annotation to pod: `gpu.scheduling/allocated: node-a:0,1`
- This tells the webhook which GPUs were assigned

### Step 3: Webhook Injects Environment Variable

When the pod is about to be created:

1. Webhook sees the `gpu.scheduling/allocated` annotation
2. Parses it: `node-a:0,1` means GPUs 0 and 1 on node-a
3. Injects `CUDA_VISIBLE_DEVICES=0,1` into all containers
4. NVIDIA runtime uses this to restrict the container to only those GPUs

### Step 4: Agent Reports GPU Status

The agent runs as a DaemonSet on each node:

1. Discovers available GPUs (currently placeholder, NVML integration TODO)
2. Creates/updates a `GpuNodeStatus` resource every 30 seconds
3. Reports GPU health, NVLink topology, and which pods are using which GPUs

## Key Design Decisions

### Why Leases?

We use Kubernetes **Coordination Leases** for atomic GPU allocation:

- **Atomic**: Creating a lease either succeeds (GPU is ours) or fails (GPU already taken)
- **Simple**: No need for custom locking mechanisms
- **Kubernetes-native**: Uses built-in resources
- **Automatic cleanup**: Leases can have expiration times

### Why Annotations?

Annotations connect the scheduler and webhook:

- **`gpu.scheduling/claim`**: User → Scheduler (which claim to use)
- **`gpu.scheduling/allocated`**: Scheduler → Webhook (which GPUs were assigned)

This decouples the two components while keeping them synchronized.

### Why Three Components?

1. **Scheduler Plugin**: Needs deep integration with Kubernetes scheduling framework
2. **Webhook**: Separate service for admission control (can scale independently)
3. **Agent**: Runs on each node to discover local GPU hardware

## Data Flow

```
User creates Pod with claim annotation
         ↓
Scheduler reads claim, finds available GPUs
         ↓
Scheduler creates Leases (locks GPUs)
         ↓
Scheduler adds "allocated" annotation to Pod
         ↓
Webhook sees "allocated" annotation
         ↓
Webhook injects CUDA_VISIBLE_DEVICES env var
         ↓
Pod runs with correct GPUs visible
```

## What Happens on Failure?

### Pod scheduling fails
- Scheduler's `Unreserve` phase runs
- All acquired leases are deleted
- GPUs become available for other pods

### Pod is deleted
- Leases remain (they're not automatically tied to pod lifecycle)
- Need garbage collection (TODO) or lease expiration

### Node goes down
- Agent stops reporting
- Leases remain until explicitly cleaned up
- This is a known limitation of the MVP

## Topology Awareness

The system tracks GPU topology through `GpuNodeStatus`:

```yaml
status:
  devices:
    - id: 0
      island: "nvlink-group-0"  # GPUs in same island have fast interconnect
      bandwidthGBps: 600
    - id: 1
      island: "nvlink-group-0"
      bandwidthGBps: 600
    - id: 2
      island: "nvlink-group-1"  # Different island = slower communication
      bandwidthGBps: 64
```

**Contiguous policy**: Prefers GPUs 0,1 over 0,2 (same island, better interconnect)

## Future: Gang Scheduling

The `GpuClaim` has a `gangRef` field for multi-pod workloads:

```yaml
spec:
  devices:
    count: 4
  gangRef: "my-distributed-training-job"
```

All pods in the gang must be schedulable together, or none run. This prevents deadlocks in distributed training.

**Status**: Not implemented in MVP
