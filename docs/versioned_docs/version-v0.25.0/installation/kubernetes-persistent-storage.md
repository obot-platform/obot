# Persistent Storage in Kubernetes

When Obot deploys MCP servers (including Obot Agent workloads) in Kubernetes, those pods need persistent volumes if you want workspace data to survive pod restarts and rescheduling.

Without persistence, agent state is stored in the pod filesystem and is lost when the pod is recreated.

## Storage Options

Any Kubernetes `StorageClass` can be used, including cloud block storage and shared filesystem-backed classes. Some examples:

- **AWS**: EBS-backed StorageClasses
- **GCP**: Hyperdisk-backed StorageClasses
- **Self-managed clusters**: NFS or other CSI-backed StorageClasses

Choose a `StorageClass` that matches your durability, performance, and cost requirements.

## Configure Obot Agent Persistence

Set the default storage class and size in your Helm values:

```yaml
mcpServerDefaults:
  storageClassName: <your storage class>
  nanobotWorkspaceSize: 1Gi
```

- `mcpServerDefaults.storageClassName`: StorageClass used for MCP server workspaces
- `mcpServerDefaults.nanobotWorkspaceSize`: PVC size requested for each workspace

## Configure Published Workflow Storage on a PVC

If `OBOT_ARTIFACT_STORAGE_PROVIDER` is unset, Obot stores published workflows on local disk at:

```text
/data/.local/share/obot/published-artifacts
```

The Helm chart's `persistence` PVC mounts at `/data`, which includes that directory.

### Single Replica with ReadWriteOnce

For `replicaCount: 1`, a `ReadWriteOnce` PVC is sufficient:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: obot-data
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: <your rwo storage class>
  resources:
    requests:
      storage: 10Gi
```

Mount it into Obot with:

```yaml
replicaCount: 1

persistence:
  enabled: true
  existingClaim: obot-data
```

This is the common setup when using a single Obot pod with a block-storage-backed `StorageClass`. It persists both general `/data` contents and the local published-artifact directory under `/data/.local/share/obot/published-artifacts`.

### Multiple Replicas with ReadWriteMany

For `replicaCount: 2` or higher, all Obot pods need concurrent access to the same `/data` volume. That requires a shared `ReadWriteMany` volume:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: obot-data-rwx
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: <your rwx storage class>
  resources:
    requests:
      storage: 10Gi
```

Mount it into Obot with:

```yaml
replicaCount: 2

persistence:
  enabled: true
  existingClaim: obot-data-rwx
```

Use a `StorageClass` backed by a shared filesystem such as NFS. A single shared `ReadWriteOnce` claim is not suitable for multi-replica Obot deployments.

### Use an Existing Claim

If you already have a PVC, point the chart at it:

```yaml
persistence:
  enabled: true
  existingClaim: obot-data
```

When enabled, the chart mounts that claim into the Obot container at `/data`, which includes the local published-artifact directory.

### Let Helm Create the Claim

If you prefer Helm-managed PVCs instead of creating them yourself, set the storage class, access mode, and size directly:

```yaml
persistence:
  enabled: true
  storageClass: <your storage class>
  accessModes:
    - ReadWriteOnce
  size: 10Gi
```

## Example: nfs-subdir-external-provisioner

If you do not have a cloud-managed dynamic provisioner, you can use [nfs-subdir-external-provisioner](https://github.com/kubernetes-sigs/nfs-subdir-external-provisioner) to provide dynamic PVC provisioning backed by an NFS server.

After installing the provisioner and creating its `StorageClass`, set that class in your Obot values file:

```yaml
mcpServerDefaults:
  storageClassName: nfs-client
  nanobotWorkspaceSize: 1Gi
```

Then install or upgrade Obot:

```bash
helm upgrade --install obot obot/obot -f values.yaml
```

## Validation

After deployment, verify that PVCs are created and bound when MCP servers start:

```bash
kubectl get pvc -A
kubectl get storageclass
```

If PVCs remain `Pending`, confirm that:

- The configured `storageClassName` exists
- A provisioner is running and healthy
- The cluster can reach the backing storage service
