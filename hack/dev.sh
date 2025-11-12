#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"

kubectl apply -f charts/gpu-scheduler/templates/crds.yaml
helm upgrade --install gpu-scheduler charts/gpu-scheduler \
  --namespace "${NAMESPACE}" \
  --create-namespace
