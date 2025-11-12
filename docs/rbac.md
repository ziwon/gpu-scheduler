# RBAC Configuration

## Overview

The GPU Scheduler uses a single ServiceAccount with three separate ClusterRoles for granular permission management:

1. **Scheduler Role** - Comprehensive permissions for scheduling decisions
2. **Agent Role** - Limited permissions for GPU inventory reporting
3. **Webhook Role** - Minimal permissions for admission control

All three components use the same ServiceAccount but are bound to different ClusterRoles based on their needs.

## ServiceAccount

**Name:** `gpu-scheduler` (configurable via `values.yaml`)

**Namespace:** Same as the Helm release namespace

Used by:
- Scheduler deployment
- Webhook deployment
- Agent daemonset

## ClusterRoles

### 1. Scheduler Role (`gpu-scheduler-scheduler`)

The scheduler requires extensive read permissions to make informed scheduling decisions.

#### Core Resources
```yaml
- pods, nodes, pods/status        # Schedule and track pods
- pods/binding                    # Bind pods to nodes
- events                          # Record scheduling events
```

#### Cluster Information
```yaml
- namespaces                      # Understand namespace boundaries
- services                        # Service topology awareness
- configmaps                      # Authentication config (kube-system)
```

#### Workload Ownership
```yaml
- replicationcontrollers          # Pod controllers
- replicasets, statefulsets       # Workload types
```

#### Storage Resources
```yaml
- persistentvolumes               # Volume information
- persistentvolumeclaims          # Volume claims
- storageclasses                  # Storage class details
- csinodes, csidrivers            # CSI resources
- csistoragecapacities            # Storage capacity
- volumeattachments               # Volume attachments
```

#### Scheduling Features
```yaml
- poddisruptionbudgets            # Pod disruption policies
- leases (coordination.k8s.io)    # GPU locking mechanism
```

#### GPU Custom Resources
```yaml
- gpuclaims                       # GPU allocation requests
- gpuclaims/status                # Claim status updates
- gpunodestatuses                 # GPU node inventory (read-only)
```

### 2. Agent Role (`gpu-scheduler-agent`)

The agent needs minimal permissions to report GPU inventory.

#### Permissions
```yaml
- nodes                           # Read node information
- gpunodestatuses                 # Create and update GPU inventory
- gpunodestatuses/status          # Update status subresource
```

**Key Feature:** The agent can create and patch `GpuNodeStatus` resources to report GPU availability and health.

### 3. Webhook Role (`gpu-scheduler-webhook`)

The webhook requires minimal read-only access for validation.

#### Permissions
```yaml
- pods                            # Read pod specifications
- gpuclaims                       # Validate claim references
```

**Security Note:** The webhook operates with least privilege - read-only access only.

## Permission Matrix

| Resource | Scheduler | Agent | Webhook |
|----------|-----------|-------|---------|
| pods | get, list, watch, update, patch | - | get, list |
| nodes | get, list, watch | get, list, watch | - |
| leases | get, list, watch, create, update, patch, delete | - | - |
| gpuclaims | get, list, watch, update, patch | - | get, list |
| gpunodestatuses | get, list, watch | get, list, watch, create, update, patch | - |
| gpunodestatuses/status | - | get, update, patch | - |

## Deployment

The RBAC resources are automatically created when you install the Helm chart:

```bash
helm install gpu-scheduler charts/gpu-scheduler
```

This creates:
- 1 ServiceAccount
- 3 ClusterRoles
- 3 ClusterRoleBindings

## Verifying Permissions

### Check ServiceAccount

```bash
kubectl get serviceaccount gpu-scheduler -n gpu-scheduler
```

### Check ClusterRoles

```bash
kubectl get clusterrole | grep gpu-scheduler

# Output:
# gpu-scheduler-scheduler
# gpu-scheduler-agent
# gpu-scheduler-webhook
```

### Check ClusterRoleBindings

```bash
kubectl get clusterrolebinding | grep gpu-scheduler

# Output:
# gpu-scheduler-scheduler
# gpu-scheduler-agent
# gpu-scheduler-webhook
```

### Test Permissions

```bash
# Check scheduler permissions
kubectl auth can-i list pods \
  --as=system:serviceaccount:gpu-scheduler:gpu-scheduler

# Check agent permissions
kubectl auth can-i patch gpunodestatuses/status \
  --as=system:serviceaccount:gpu-scheduler:gpu-scheduler

# Check webhook permissions
kubectl auth can-i get gpuclaims \
  --as=system:serviceaccount:gpu-scheduler:gpu-scheduler
```

## Troubleshooting

### Permission Denied Errors

If you see errors like:
```
"pods" is forbidden: User "system:serviceaccount:gpu-scheduler:gpu-scheduler"
cannot list resource "pods" in API group "" at the cluster scope
```

**Solution:**
1. Verify ClusterRoleBindings exist:
   ```bash
   kubectl get clusterrolebinding gpu-scheduler-scheduler -o yaml
   ```

2. Check if the binding references the correct ServiceAccount:
   ```yaml
   subjects:
   - kind: ServiceAccount
     name: gpu-scheduler
     namespace: gpu-scheduler  # Should match your namespace
   ```

3. Re-apply RBAC if needed:
   ```bash
   kubectl apply -f charts/gpu-scheduler/templates/rbac.yaml
   ```

### Agent Cannot Update GpuNodeStatus

Error:
```
"gpunodestatuses/status" is forbidden: cannot patch resource "gpunodestatuses/status"
```

**Solution:** Ensure the agent role includes the status subresource:
```bash
kubectl get clusterrole gpu-scheduler-agent -o yaml | grep -A2 gpunodestatuses
```

Should show:
```yaml
- apiGroups: ["gpu.scheduling"]
  resources: ["gpunodestatuses/status"]
  verbs: ["get", "update", "patch"]
```

### Webhook Cannot Read Pods

**Solution:** Verify the webhook role exists and is bound:
```bash
kubectl get clusterrole gpu-scheduler-webhook
kubectl get clusterrolebinding gpu-scheduler-webhook
```

## Security Best Practices

### 1. Least Privilege

Each component only has the permissions it needs:
- ✅ Scheduler: Extensive read access, limited write to pods/leases
- ✅ Agent: Only writes to gpunodestatuses
- ✅ Webhook: Read-only access

### 2. Separation of Concerns

Three separate ClusterRoles instead of one monolithic role:
- Easier to audit
- Principle of least privilege
- Clear permission boundaries

### 3. No Elevated Privileges

- ❌ No `create` on pods (scheduler uses binding)
- ❌ No `delete` on pods
- ❌ No cluster-admin
- ❌ No access to secrets

### 4. Audit Trail

All actions are logged with the ServiceAccount identity:
```
User: system:serviceaccount:gpu-scheduler:gpu-scheduler
```

## Customization

### Using a Different ServiceAccount Name

Edit `values.yaml`:
```yaml
serviceAccountName: my-custom-sa
```

Then upgrade:
```bash
helm upgrade gpu-scheduler charts/gpu-scheduler
```

### Adding Additional Permissions

If you need additional permissions, edit `charts/gpu-scheduler/templates/rbac.yaml`:

```yaml
# Add to the appropriate ClusterRole
- apiGroups: ["custom.api.group"]
  resources: ["customresources"]
  verbs: ["get", "list"]
```

### Namespace-Scoped Permissions

To limit permissions to specific namespaces, convert ClusterRoles to Roles:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: gpu-scheduler-scheduler
  namespace: specific-namespace
rules:
  # ... same rules ...
```

**Note:** The scheduler typically needs cluster-wide access to schedule pods across all namespaces.

## References

- [Kubernetes RBAC Documentation](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [ServiceAccounts](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/)
- [Using RBAC Authorization](https://kubernetes.io/docs/reference/access-authn-authz/authorization/)
