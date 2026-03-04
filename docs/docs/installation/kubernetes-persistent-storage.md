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
