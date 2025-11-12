# Fabric-Aware Scheduling Roadmap

Goal: make GPU placement aware of NVLink islands, InfiniBand topology, and GPUDirect (RDMA + Storage / cuFile) capabilities.

## 1. Data Modeling
- Extend `GpuNodeStatus.Status` with:
  - `nvlinkIslands`, `links`, and measured `bandwidthGBps`.
  - InfiniBand metadata: `hcaModel`, `ports`, link speed/state, GDR capability.
  - Storage DMA flags: `cuFileEnabled`, `nvmePeers`, `gdsVersion`.
- Add `GpuClaimSpec` sections:
  - `topology.mode` + `minBandwidthGBps` (already present) → enforce in Filter/Score.
  - `network.rdmaRequired`, `minPorts`, `preferredHcas`.
  - `storage.cuFile` (Required/Preferred) to signal GPUDirect Storage needs.

## 2. Agent Enhancements
- Integrate NVML to collect per-GPU NVLink island IDs and link bandwidth.
- Query OFED/rdma-core (`rdma sysfs`, `ibstat`, `devlink`) to map HCAs, ports, and GPUDirect RDMA support.
- Detect cuFile readiness (`/etc/cufile.json`, kernel modules, `nvidia-fs`).
- Publish the new metadata via server-side apply; keep payload size manageable (compress arrays, avoid noisy diffs).

## 3. Scheduler / Plugin Logic
- In `PreFilter`, fetch the referenced `GpuClaim` and validate the new fabric fields.
- In `Filter`, reject nodes that lack mandatory fabric capabilities (e.g., no RDMA when `network.rdmaRequired`).
- Enhance `Score` (or a future `PreScore`) to:
  - Prefer nodes whose NVLink graph meets `minBandwidthGBps` or exclusive island requirements.
  - Boost scores for nodes with surplus RDMA ports when gang scheduling multiple pods.
- Keep lease acquisition logic unchanged; we only gate candidate nodes earlier.

## 4. Admission Webhook
- When a claim requests RDMA or cuFile:
  - Inject env vars such as `NCCL_IB_HCA`, `NCCL_NET_GDR_LEVEL`, `CUFILE_ENV_PATH`.
  - Optionally add projected ConfigMaps/Secrets (cuFile policy, `nvidia-fabricmanager` config).
- Ensure mutations are conditional so regular pods stay untouched.

## 5. Packaging & Ops
- Update Helm chart values to toggle fabric-aware mode (feature gates, sampling periods).
- Provide metrics (Prometheus) showing NVLink/IB inventory and scheduling decisions.
- Document required node dependencies (NVML, rdma-core, cuFile libraries, permissions).

## 6. Testing
- Create fake `GpuNodeStatus` fixtures covering:
  - Mixed NVLink bandwidths and islands.
  - Nodes with/without RDMA ports.
  - cuFile-enabled vs. disabled storage stacks.
- Add e2e scenarios in kind or a CI-friendly simulator to verify:
  - Claims with `network.rdmaRequired` avoid CPU-only nodes.
  - `topology.minBandwidthGBps` drives pods onto NVLink-rich nodes.
  - Webhook injects correct env/config when cuFile is requested.

Tracking this plan in docs keeps the MVP stable while providing a clear path toward fabric-aware placement.
