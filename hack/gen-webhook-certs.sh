#!/bin/bash
# Generate TLS certificates for gpu-scheduler webhook
# Usage: ./gen-webhook-certs.sh [namespace]

set -e

# Configuration
NAMESPACE=${1:-default}
SERVICE=gpu-scheduler-webhook
WEBHOOK_NAME=gpu-scheduler-webhook

echo "=================================================="
echo "GPU Scheduler Webhook Certificate Generator"
echo "=================================================="
echo ""
echo "Namespace: ${NAMESPACE}"
echo "Service:   ${SERVICE}"
echo ""

# Create temporary directory
CERT_DIR=$(mktemp -d)
trap "rm -rf ${CERT_DIR}" EXIT

cd "${CERT_DIR}"
echo "Working directory: ${CERT_DIR}"
echo ""

# 1. Generate CA private key
echo "[1/10] Generating CA private key..."
openssl genrsa -out ca.key 2048 2>/dev/null

# 2. Generate CA certificate
echo "[2/10] Generating CA certificate..."
openssl req -x509 -new -nodes -key ca.key \
  -subj "/CN=${SERVICE}.${NAMESPACE}.svc" \
  -days 365 \
  -out ca.crt 2>/dev/null

# 3. Generate server private key
echo "[3/10] Generating server private key..."
openssl genrsa -out tls.key 2048 2>/dev/null

# 4. Create OpenSSL configuration with SANs
echo "[4/10] Creating OpenSSL configuration..."
cat > csr.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE}
DNS.2 = ${SERVICE}.${NAMESPACE}
DNS.3 = ${SERVICE}.${NAMESPACE}.svc
DNS.4 = ${SERVICE}.${NAMESPACE}.svc.cluster.local
EOF

# 5. Generate Certificate Signing Request (CSR)
echo "[5/10] Generating Certificate Signing Request..."
openssl req -new -key tls.key \
  -subj "/CN=${SERVICE}.${NAMESPACE}.svc" \
  -config csr.conf \
  -out tls.csr 2>/dev/null

# 6. Sign the certificate with CA
echo "[6/10] Signing certificate..."
openssl x509 -req -in tls.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out tls.crt \
  -days 365 \
  -extensions v3_req \
  -extfile csr.conf 2>/dev/null

# 7. Create namespace if it doesn't exist
echo "[7/10] Creating namespace (if needed)..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1

# 8. Create Kubernetes secret
echo "[8/10] Creating Kubernetes secret..."
kubectl create secret tls gpu-scheduler-webhook-cert \
  --cert=tls.crt \
  --key=tls.key \
  -n "${NAMESPACE}" \
  --dry-run=client -o yaml | kubectl apply -f - >/dev/null

# 9. Get CA bundle for webhook configuration
echo "[9/10] Extracting CA bundle..."
CA_BUNDLE=$(cat ca.crt | base64 | tr -d '\n')

# 10. Update webhook configuration if it exists
echo "[10/10] Updating MutatingWebhookConfiguration..."
if kubectl get mutatingwebhookconfiguration "${WEBHOOK_NAME}" &>/dev/null; then
  kubectl patch mutatingwebhookconfiguration "${WEBHOOK_NAME}" \
    --type='json' \
    -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'${CA_BUNDLE}'}]" >/dev/null
  echo "Webhook configuration updated!"
else
  echo "⚠️  MutatingWebhookConfiguration '${WEBHOOK_NAME}' not found."
  echo "   The caBundle will need to be added when you deploy the webhook."
  echo ""
  echo "   CA Bundle (base64-encoded):"
  echo "   ${CA_BUNDLE}"
fi

echo ""
echo "=================================================="
echo "Certificate setup complete!"
echo "=================================================="
echo ""
echo "Secret created: gpu-scheduler-webhook-cert (namespace: ${NAMESPACE})"
echo ""
echo "Next steps:"
echo "  1. Deploy webhook: helm upgrade gpu-scheduler charts/gpu-scheduler -n ${NAMESPACE}"
echo "  2. Verify pod:     kubectl get pods -l app=gpu-scheduler-webhook -n ${NAMESPACE}"
echo "  3. Check logs:     kubectl logs -l app=gpu-scheduler-webhook -n ${NAMESPACE}"
echo ""
echo "Certificate details:"
openssl x509 -in tls.crt -noout -subject -dates 2>/dev/null | sed 's/^/  /'
echo ""

# Verify certificate
echo "Subject Alternative Names:"
openssl x509 -in tls.crt -noout -text 2>/dev/null | grep -A1 "Subject Alternative Name" | tail -1 | sed 's/^/  /'
echo ""
