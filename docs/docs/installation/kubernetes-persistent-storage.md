---
displayed_sidebar: sidebar
---

# Persistent Storage in Kubernetes

The Helm chart can mount a persistent volume at `/data` in the Obot server container. Enable it when your configured features need local files to survive pod replacement. Use an external PostgreSQL database for production; a server data volume does not replace database backups.

## Storage Options

Any Kubernetes `StorageClass` can be used, including cloud block storage and shared filesystem-backed classes. Some examples:

- **AWS**: EBS-backed StorageClasses
- **GCP**: Hyperdisk-backed StorageClasses
- **Self-managed clusters**: NFS or other CSI-backed StorageClasses

Choose a `StorageClass` that matches your durability, performance, and cost requirements.

## Configure the Obot data volume

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

This is the common setup when using a single Obot pod with a block-storage-backed `StorageClass`. It persists files stored under `/data`.

### Multiple Replicas with ReadWriteMany

If multiple Obot replicas need to share files under `/data`, configure a `ReadWriteMany` volume:

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

When enabled, the chart mounts that claim into the Obot container at `/data`, preserving files stored there.

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
persistence:
  enabled: true
  storageClass: nfs-client
  accessModes:
    - ReadWriteOnce
  size: 10Gi
```

Then install or upgrade Obot:

```bash
helm upgrade --install obot obot/obot -f values.yaml
```

## Validation

After deployment, verify that the Obot data PVC is created and bound:

```bash
kubectl get pvc -A
kubectl get storageclass
```

If PVCs remain `Pending`, confirm that:

- The configured `storageClassName` exists
- A provisioner is running and healthy
- The cluster can reach the backing storage service
