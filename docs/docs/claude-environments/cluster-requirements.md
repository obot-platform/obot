---
title: Cluster compatibility and prerequisites
description: Certificate APIs, worker requirements, and the validation status of GKE, EKS, AKS, and k3d.
---

:::warning Experimental compatibility
This is an experimental integration. Provider research below was checked on **October 13, 2026**. GKE has an upstream installation guide; EKS and AKS are evaluation candidates, not validated Obot deployment targets. No end-to-end cloud-provider test is implied by this table.
:::

## Provider matrix

| Platform | Evaluation path | Status and constraints |
|---|---|---|
| GKE Standard | 1.36 with the certificate APIs enabled, or 1.37+ | Explicit upstream deployment path. On 1.36, enabling the APIs on an existing cluster also requires replacing existing nodes. Google's documented node baseline is `c3-standard-4` or larger; E2 is excluded. Obot's chart still needs an end-to-end test on your cluster. |
| GKE Autopilot | None for the current chart | Unsupported by upstream; host access does not fit Autopilot restrictions. |
| EKS with Linux EC2 nodes | Evaluate 1.37 control plane **and nodes** | AWS lists 1.37 as available; upstream Kubernetes makes the certificate features stable in that version. This is an inference about prerequisites, not confirmation that Substrate works on EKS. Check API discovery, projections, node security, storage, and suspend/resume. Older EKS versions are not a documented path here. |
| EKS Fargate | None for the current chart | Fargate does not support the DaemonSet and host ports used by atelet. EKS Auto Mode is separately unvalidated; use conventional EC2 node groups for initial evaluation. |
| AKS with Linux VM node pools | Evaluate 1.37 where available in your region | Microsoft's schedule lists 1.37 for October 2026; availability rolls out by region. The certificate prerequisites follow upstream Kubernetes, but we have not established an AKS-specific Substrate support path. Earlier versions and AKS Automatic are unvalidated. Do not target virtual nodes for the node-level runtime. |
| k3d / K3s | K3s `v1.36.4+k3s1` with the flags below | Local development configuration exercised during this integration's development; not a production reference architecture. |

Sources: [Google's Substrate guide](https://docs.cloud.google.com/kubernetes-engine/ai-ml/install-overview-substrate), [EKS version notes](https://docs.aws.amazon.com/eks/latest/userguide/kubernetes-versions-standard.html), [Fargate limitations](https://docs.aws.amazon.com/eks/latest/userguide/fargate.html), [AKS version policy](https://learn.microsoft.com/en-us/azure/aks/supported-kubernetes-versions), and [Kubernetes 1.37 certificate graduation](https://kubernetes.io/blog/2026/08/28/kubernetes-v1-37-pod-certificates-and-cluster-trust-bundles/).

Google's upstream production support is a separate allowlisted program; it does not change Obot's experimental status. An existing Obot installation working on a provider is not proof that its nodes can run Substrate.

## Certificate APIs and projections

Substrate uses `ClusterTrustBundle` and `PodCertificateRequest` for mutual TLS. The control plane must serve these APIs and the kubelets must support the corresponding projected volumes. Helm cannot enable cluster feature gates.

For the tested K3s 1.36 configuration, the server uses these arguments:

```text
--kube-apiserver-arg=runtime-config=certificates.k8s.io/v1beta1=true
--kube-apiserver-arg=feature-gates=ClusterTrustBundle=true,PodCertificateRequest=true
--kubelet-arg=feature-gates=ClusterTrustBundle=true,ClusterTrustBundleProjection=true,PodCertificateRequest=true
```

Apply the kubelet settings to every node that will run certificate-dependent pods, including separate agent nodes. On managed providers, use their supported API enablement mechanism, not K3s flags. Kubernetes 1.37 graduates these features, but check the provider's actual control-plane and node versions and available API versions.

For GKE 1.36, an authorized cluster administrator can enable the APIs using the following command. Replace the placeholders. This changes cluster configuration; schedule the subsequent node replacement as maintenance.

```nu
gcloud container clusters update CLUSTER_NAME --location CONTROL_PLANE_LOCATION --enable-kubernetes-unstable-apis=certificates.k8s.io/v1beta1/podcertificaterequests,certificates.k8s.io/v1beta1/clustertrustbundles
```

Follow [GKE's beta API instructions](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/use-beta-apis) and its Substrate guide for node replacement. Changing the control plane alone on 1.36 can leave pods failing with `MountVolume.SetUp failed: unimplemented`. Some older upstream setup notes recommend recreating the cluster; Google's current guide describes updating the cluster and replacing its nodes.

Before installing, run these read-only checks in the intended context:

```nu
kubectl version
kubectl get nodes -o wide
kubectl api-resources --api-group=certificates.k8s.io
kubectl get storageclass
```

API discovery must include `clustertrustbundles` and `podcertificaterequests`. Discovery alone does **not** verify kubelet projection support. After installation, confirm the certificate controller issues credentials, projected volumes mount successfully, and both Obot and Substrate pods become Ready. Do not label a provider compatible until the [acceptance checks](./operations.md#acceptance-checks) pass.

## Worker nodes and admission policies

Use Linux nodes with sufficient allocatable CPU, memory, and disk for the control plane, worker pools, and downloaded runtime assets. The chart selects Linux atelet nodes. Its gVisor assets cover amd64 and arm64; select worker and runner images that support your architecture.

Atelet uses host ports **8085 and 9090** and host paths including `/var/lib/ate`, `/var/lib/kubelet/plugins`, `/var/lib/kubelet/device-plugins`, and `/dev`. Cluster admission policies must permit the chart's pod specifications, capabilities, and host access. A restricted serverless pool is not interchangeable with a Linux VM node pool.

The chart provides the gVisor SandboxConfig; it is not a request to enable GKE Sandbox's RuntimeClass. Upstream also has micro-VM options with different device requirements; this guide covers Obot's packaged gVisor runtime.

If you restrict `substrate.atelet.nodeSelector`, configure each environment's worker node selector to land on those nodes too. The chart does not automatically copy the atelet selector into environment settings.

## Storage, networking, and scope

- Provide an existing S3-compatible or GCS snapshot bucket. Obot does not provision it. Both `ate-system/atelet` and `ate-system/ate-api-server` need the appropriate object access; test writing, restoring, and deleting snapshots.
- Use bundled PostgreSQL with a default StorageClass (or explicitly select one), or supply an external database. Keep Substrate's database separate from Obot's database.
- Permit DNS and internal service traffic among Obot, Substrate, and workers. Permit outbound access to Anthropic, registries, snapshot storage, and the gVisor assets referenced by the SandboxConfig. Repository and tool destinations also need environment egress permission.
- The bundled database's ingress restriction requires a CNI that enforces NetworkPolicy.
- Install only one Substrate instance per cluster. Its namespaces (`ate-system` and `podcertificate-controller-system`), signer names, and CRDs are cluster-wide conventions. Use a cluster without a separate Substrate installation; do not run Google's installer alongside the Obot chart.

On AKS, Azure Blob Storage is not a configured snapshot backend in this chart. Use supported S3-compatible storage or GCS and validate its authentication. On EKS or GKE, workload identity can avoid static bucket keys, but the IAM associations must be configured for the Substrate service accounts; Obot's own identity is not automatically reused.
