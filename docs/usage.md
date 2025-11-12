# Usage Guide

## Prerequisites

- Kubernetes cluster 1.33.x (verified) — newer minors preferred; limited regression on 1.32/1.31
- Nodes with NVIDIA GPUs
- NVIDIA device plugin installed (for GPU discovery)
- Helm 3 installed

## Installation

### Step 1: Apply CRDs

```bash
kubectl apply -f charts/gpu-scheduler/templates/crds.yaml
```

This installs the custom resource definitions:
- `GpuClaim`
- `GpuNodeStatus`

### Step 2: Setup Webhook TLS Certificates

The admission webhook **requires TLS certificates** to function. Kubernetes mandates HTTPS for admission webhooks.

**Quick Setup (Self-Signed):**
```bash
NAMESPACE=default

# Download and run certificate generation script
curl -sL https://raw.githubusercontent.com/ziwon/gpu-scheduler/main/hack/gen-webhook-certs.sh | bash -s -- ${NAMESPACE}

# Or manually - see detailed guide
```

**Production Setup (cert-manager):**
```bash
# Install cert-manager first, then create Certificate resource
# See detailed guide below
```

📖 **For detailed certificate setup instructions, see [Webhook Certificates Guide](webhook-certificates.md)**

The guide covers:
- Why certificates are required (port 443, HTTPS mandatory)
- Complete certificate generation scripts
- cert-manager setup for production
- Troubleshooting common certificate issues

### Step 3: Install with Helm

```bash
helm install gpu-scheduler charts/gpu-scheduler
```

Or with custom namespace:

```bash
export NAMESPACE=gpu-system

# Don't forget to create certificates in the custom namespace!
# Repeat Step 2 with NAMESPACE=gpu-system

./hack/dev.sh
```

### Step 4: Verify Installation

Check that all components are running:

```bash
# Check scheduler
kubectl get deploy gpu-scheduler

# Check webhook
kubectl get deploy gpu-scheduler-webhook

# Check agent (should be on each GPU node)
kubectl get daemonset gpu-scheduler-agent
```

## Basic Usage

### Example 1: Single GPU

Create a claim for one GPU:

```yaml
# claim.yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: single-gpu
spec:
  devices:
    count: 1
    exclusivity: Exclusive
```

Create a pod using the claim:

```yaml
# pod.yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-test
  annotations:
    gpu.scheduling/claim: single-gpu
spec:
  schedulerName: gpu-scheduler
  restartPolicy: Never
  containers:
    - name: cuda-test
      image: nvidia/cuda:12.4.1-runtime-ubuntu22.04
      command: ["nvidia-smi"]
      resources:
        limits:
          nvidia.com/gpu: "1"
```

Apply both:

```bash
kubectl apply -f claim.yaml
kubectl apply -f pod.yaml
```

Check the allocation:

```bash
# See which GPU was assigned
kubectl get pod gpu-test -o jsonpath='{.metadata.annotations.gpu\.scheduling/allocated}'
# Output: node-a:0

# Check the pod logs
kubectl logs gpu-test
```

### Example 2: Multiple GPUs (Contiguous)

For workloads needing fast GPU-to-GPU communication:

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: multi-gpu
spec:
  devices:
    count: 4
    policy: contiguous  # Prefer GPUs with adjacent IDs
    exclusivity: Exclusive
  topology:
    mode: Preferred  # Try to get GPUs in same NVLink island
---
apiVersion: v1
kind: Pod
metadata:
  name: training-job
  annotations:
    gpu.scheduling/claim: multi-gpu
spec:
  schedulerName: gpu-scheduler
  containers:
    - name: training
      image: pytorch/pytorch:2.0.0-cuda11.7-cudnn8-runtime
      resources:
        limits:
          nvidia.com/gpu: "4"
```

### Example 3: Pinned GPU IDs

If you need specific GPU IDs:

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: pinned-gpus
spec:
  devices:
    count: 2
    policy: preferIds
    preferIds: [0, 1]  # Try to get GPUs 0 and 1
    exclusivity: Exclusive
```

### Example 4: Node Selection

To target specific nodes:

```yaml
apiVersion: gpu.scheduling/v1
kind: GpuClaim
metadata:
  name: specific-node
spec:
  selector:
    matchLabels:
      gpu-type: a100
      topology: nvlink
  devices:
    count: 8
    policy: contiguous
```

## Checking GPU Status

### View all GPU claims

```bash
kubectl get gpuclaim
# or short form
kubectl get gclaim
```

Example output:
```
NAME         REQ   POLICY       TOPOLOGY   ALLOCATED
single-gpu   1     contiguous              node-a:0
multi-gpu    4     contiguous   Preferred  node-b:0,1,2,3
```

### View node GPU status

```bash
kubectl get gpunodestatus
# or short form
kubectl get gns
```

Example output:
```
NAME     NODE     DEVICES
node-a   node-a   4
node-b   node-b   8
```

Get detailed GPU info for a node:

```bash
kubectl get gpunodestatus node-a -o yaml
```

### Check GPU leases

See which GPUs are currently locked:

```bash
kubectl get leases | grep gpu-
```

Example output:
```
gpu-node-a-0   25s
gpu-node-a-1   25s
gpu-node-b-2   1m
```

## Troubleshooting

### Pod stuck in Pending

Check scheduler logs:

```bash
kubectl logs -l app=gpu-scheduler
```

Common reasons:
- No nodes with enough free GPUs
- Node selector doesn't match any nodes
- GPU leases stuck (manual cleanup needed)

### Webhook errors: "no endpoints available"

**Error message:**
```
Internal error occurred: failed calling webhook "pods.gpu-scheduler.svc":
failed to call webhook: Post "https://gpu-scheduler-webhook.default.svc:443/mutate?timeout=10s":
no endpoints available for service "gpu-scheduler-webhook"
```

This usually means **TLS certificates are missing** or the webhook pod isn't running.

**Quick diagnosis:**
```bash
# Check webhook pod
kubectl get pods -l app=gpu-scheduler-webhook

# Check certificate secret
kubectl get secret gpu-scheduler-webhook-cert

# Check logs
kubectl logs -l app=gpu-scheduler-webhook
```

**Fix:** If the certificate secret is missing, generate certificates following [Step 2](#step-2-setup-webhook-tls-certificates).

📖 **For detailed troubleshooting, see [Webhook Certificates Guide - Troubleshooting](webhook-certificates.md#troubleshooting)**

**Key points:**
- Port 443 is mandatory (Kubernetes requirement)
- HTTPS/TLS is required (not optional)
- API server calls the webhook (not the agent)

### GPU not visible in container

Check if the annotation was set:

```bash
kubectl get pod <pod-name> -o jsonpath='{.metadata.annotations}'
```

Should see both:
- `gpu.scheduling/claim: <claim-name>`
- `gpu.scheduling/allocated: <node>:<gpu-ids>`

Check webhook logs:

```bash
kubectl logs -l app=gpu-scheduler-webhook
```

### Wrong CUDA_VISIBLE_DEVICES

Describe the pod and check environment variables:

```bash
kubectl get pod <pod-name> -o jsonpath='{.spec.containers[0].env}'
```

Should see:
```json
[{"name":"CUDA_VISIBLE_DEVICES","value":"0,1"}]
```

### Cleanup stuck leases

If GPUs are locked but no pods are using them:

```bash
# List all GPU leases
kubectl get leases | grep gpu-

# Delete a specific lease
kubectl delete lease gpu-node-a-0

# Delete all GPU leases (caution!)
kubectl delete leases -l gpu.scheduling/managed=true
```

## Advanced Usage

### Shared GPUs (Not Recommended)

```yaml
spec:
  devices:
    count: 1
    exclusivity: Shared  # Multiple pods can share the GPU
```

**Warning**: Shared mode doesn't enforce memory limits. Pods can interfere with each other.

### Topology Requirements

For tight coupling requiring high bandwidth:

```yaml
spec:
  topology:
    mode: Required  # Fail if topology requirements not met
    minBandwidthGBps: 600  # Require NVLink speed
```

Modes:
- **Required**: Pod won't schedule if requirements not met
- **Preferred**: Try to meet requirements, but schedule anyway (default)
- **Ignore**: Don't consider topology at all

## Best Practices

1. **Always specify exclusivity**: Use `Exclusive` unless you have a good reason
2. **Use contiguous policy for multi-GPU**: Better performance for workloads with GPU-to-GPU communication
3. **Set resource limits**: Always include `resources.limits.nvidia.com/gpu`
4. **Name claims descriptively**: Use names like `training-4gpu` not `claim1`
5. **Clean up claims**: Delete GpuClaim resources when done to avoid confusion

## Uninstallation

```bash
# Delete the Helm release
helm uninstall gpu-scheduler

# Clean up CRDs (this deletes all GpuClaims and GpuNodeStatus)
kubectl delete crd gpuclaims.gpu.scheduling
kubectl delete crd gpunodestatuses.gpu.scheduling

# Clean up any remaining leases
kubectl delete leases -l gpu.scheduling/managed=true
```
