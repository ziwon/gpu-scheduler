# Webhook TLS Certificates Guide

## Why Certificates Are Required

Kubernetes admission webhooks **must** use HTTPS with valid TLS certificates. This is a security requirement that cannot be bypassed:

- **Port 443 is mandatory** - You cannot use custom ports like 7878
- **HTTPS is required** - Plain HTTP is not supported
- **Valid certificates** - Must match service DNS names
- **Called by API server** - Not by agents or other pods

## Architecture

```
┌──────────────────┐
│  API Server      │ Calls webhook over HTTPS
└────────┬─────────┘
         │ Port 443 (HTTPS)
         ▼
┌──────────────────┐
│  Service         │ gpu-scheduler-webhook.namespace.svc:443
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Webhook Pod     │ Listens on :8443
│  Container       │ Uses TLS cert from Secret
└──────────────────┘
```

## Certificate Requirements

Your certificates must include these DNS Subject Alternative Names (SANs):

```
DNS.1 = gpu-scheduler-webhook
DNS.2 = gpu-scheduler-webhook.${NAMESPACE}
DNS.3 = gpu-scheduler-webhook.${NAMESPACE}.svc
DNS.4 = gpu-scheduler-webhook.${NAMESPACE}.svc.cluster.local
```

The API server uses the fully-qualified name to call the webhook.

## Option 1: Self-Signed Certificates (Quick Setup)

Perfect for development and testing.

### Complete Script

```bash
#!/bin/bash
set -e

# Configuration
NAMESPACE=${NAMESPACE:-default}
SERVICE=gpu-scheduler-webhook
WEBHOOK_NAME=gpu-scheduler-webhook

# Create temporary directory
CERT_DIR=$(mktemp -d)
cd ${CERT_DIR}

echo "Generating certificates in ${CERT_DIR}..."

# 1. Generate CA private key
openssl genrsa -out ca.key 2048

# 2. Generate CA certificate
openssl req -x509 -new -nodes -key ca.key \
  -subj "/CN=${SERVICE}.${NAMESPACE}.svc" \
  -days 365 \
  -out ca.crt

# 3. Generate server private key
openssl genrsa -out tls.key 2048

# 4. Create OpenSSL configuration with SANs
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
openssl req -new -key tls.key \
  -subj "/CN=${SERVICE}.${NAMESPACE}.svc" \
  -config csr.conf \
  -out tls.csr

# 6. Sign the certificate with CA
openssl x509 -req -in tls.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out tls.crt \
  -days 365 \
  -extensions v3_req \
  -extfile csr.conf

echo "Certificates generated successfully!"
echo ""

# 7. Create namespace if it doesn't exist
kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

# 8. Create Kubernetes secret
echo "Creating Kubernetes secret..."
kubectl create secret tls gpu-scheduler-webhook-cert \
  --cert=tls.crt \
  --key=tls.key \
  -n ${NAMESPACE} \
  --dry-run=client -o yaml | kubectl apply -f -

# 9. Get CA bundle for webhook configuration
CA_BUNDLE=$(cat ca.crt | base64 | tr -d '\n')
echo ""
echo "CA Bundle (save this for webhook configuration):"
echo "${CA_BUNDLE}"
echo ""

# 10. Update webhook configuration
echo "Updating MutatingWebhookConfiguration..."
if kubectl get mutatingwebhookconfiguration ${WEBHOOK_NAME} &>/dev/null; then
  kubectl patch mutatingwebhookconfiguration ${WEBHOOK_NAME} \
    --type='json' \
    -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'${CA_BUNDLE}'}]"
  echo "Webhook configuration updated!"
else
  echo "Warning: MutatingWebhookConfiguration '${WEBHOOK_NAME}' not found."
  echo "You'll need to add the caBundle manually or install the Helm chart."
fi

# Cleanup
cd - > /dev/null
rm -rf ${CERT_DIR}

echo ""
echo "✅ Certificate setup complete!"
echo ""
echo "Next steps:"
echo "1. Deploy or restart the webhook: kubectl rollout restart deployment/gpu-scheduler-webhook -n ${NAMESPACE}"
echo "2. Verify: kubectl get pods -l app=gpu-scheduler-webhook -n ${NAMESPACE}"
```

### Save and Run

```bash
# Save script
cat > /tmp/gen-webhook-certs.sh << 'EOF'
# ... paste script above ...
EOF

# Make executable
chmod +x /tmp/gen-webhook-certs.sh

# Run with desired namespace
NAMESPACE=default /tmp/gen-webhook-certs.sh

# Or for custom namespace
NAMESPACE=gpu-scheduler /tmp/gen-webhook-certs.sh
```

## Option 2: cert-manager (Production Recommended)

cert-manager automatically manages certificate lifecycle including renewal.

### Install cert-manager

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available --timeout=300s \
  -n cert-manager deployment/cert-manager-webhook

# Verify installation
kubectl get pods -n cert-manager
```

### Create ClusterIssuer

```bash
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
EOF
```

### Create Certificate

```bash
# Update namespace if needed
NAMESPACE=default

cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gpu-scheduler-webhook-cert
  namespace: ${NAMESPACE}
spec:
  secretName: gpu-scheduler-webhook-cert
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days before expiry
  subject:
    organizations:
      - gpu-scheduler
  commonName: gpu-scheduler-webhook.${NAMESPACE}.svc
  dnsNames:
    - gpu-scheduler-webhook
    - gpu-scheduler-webhook.${NAMESPACE}
    - gpu-scheduler-webhook.${NAMESPACE}.svc
    - gpu-scheduler-webhook.${NAMESPACE}.svc.cluster.local
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
    group: cert-manager.io
EOF
```

### Verify Certificate

```bash
# Check certificate status
kubectl get certificate -n ${NAMESPACE}

# Should show READY=True
NAME                         READY   SECRET                       AGE
gpu-scheduler-webhook-cert   True    gpu-scheduler-webhook-cert   30s

# Check secret
kubectl get secret gpu-scheduler-webhook-cert -n ${NAMESPACE}

# View certificate details
kubectl describe certificate gpu-scheduler-webhook-cert -n ${NAMESPACE}
```

### Get CA Bundle

```bash
# Extract CA for webhook configuration
kubectl get secret gpu-scheduler-webhook-cert -n ${NAMESPACE} \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > ca.crt

CA_BUNDLE=$(kubectl get secret gpu-scheduler-webhook-cert -n ${NAMESPACE} \
  -o jsonpath='{.data.ca\.crt}')

echo "CA Bundle: ${CA_BUNDLE}"

# Update webhook configuration
kubectl patch mutatingwebhookconfiguration gpu-scheduler-webhook \
  --type='json' \
  -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'${CA_BUNDLE}'}]"
```

## Option 3: Kubernetes Certificates API

Use Kubernetes built-in certificate approval workflow.

```bash
# Generate key
openssl genrsa -out webhook.key 2048

# Create CSR
cat <<EOF | kubectl apply -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: gpu-scheduler-webhook.${NAMESPACE}
spec:
  request: $(cat webhook.csr | base64 | tr -d '\n')
  signerName: kubernetes.io/kubelet-serving
  usages:
    - digital signature
    - key encipherment
    - server auth
EOF

# Approve CSR
kubectl certificate approve gpu-scheduler-webhook.${NAMESPACE}

# Get signed certificate
kubectl get csr gpu-scheduler-webhook.${NAMESPACE} -o jsonpath='{.status.certificate}' | base64 -d > webhook.crt
```

## Troubleshooting

### Certificate Verification

```bash
# View certificate details
openssl x509 -in tls.crt -text -noout

# Check Subject Alternative Names
openssl x509 -in tls.crt -text -noout | grep -A1 "Subject Alternative Name"

# Should show:
#   DNS:gpu-scheduler-webhook, DNS:gpu-scheduler-webhook.default, ...
```

### Common Errors

#### "x509: certificate signed by unknown authority"

**Cause:** caBundle not set in MutatingWebhookConfiguration

**Fix:**
```bash
CA_BUNDLE=$(kubectl get secret gpu-scheduler-webhook-cert -n default \
  -o jsonpath='{.data.ca\.crt}')

kubectl patch mutatingwebhookconfiguration gpu-scheduler-webhook \
  --type='json' \
  -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'${CA_BUNDLE}'}]"
```

#### "x509: certificate is valid for ..., not ..."

**Cause:** Certificate doesn't include correct DNS names

**Fix:** Regenerate certificate with correct SANs

#### "no endpoints available for service"

**Cause:** Webhook pod not running or secret missing

**Fix:**
```bash
# Check pod
kubectl get pods -l app=gpu-scheduler-webhook

# Check secret
kubectl get secret gpu-scheduler-webhook-cert

# Restart webhook
kubectl rollout restart deployment/gpu-scheduler-webhook
```

## Certificate Rotation

### Manual Rotation (Self-Signed)

```bash
# Regenerate certificates
NAMESPACE=default /tmp/gen-webhook-certs.sh

# Restart webhook to pick up new certs
kubectl rollout restart deployment/gpu-scheduler-webhook -n ${NAMESPACE}
```

### Automatic Rotation (cert-manager)

cert-manager automatically renews certificates based on `renewBefore` setting:

```yaml
spec:
  duration: 2160h     # 90 days
  renewBefore: 360h   # Renew 15 days before expiry
```

No manual intervention needed!

## Security Best Practices

1. **Use cert-manager** for production
2. **Set short certificate lifetimes** (90 days max)
3. **Enable automatic renewal**
4. **Rotate certificates regularly**
5. **Store CA key securely** (if managing manually)
6. **Monitor certificate expiry**
7. **Use RBAC** to limit access to certificate secrets

## References

- [Kubernetes Admission Webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [TLS/SSL Certificate Guide](https://www.openssl.org/docs/)
