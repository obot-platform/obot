---
title: "Capacity"
---

## Configuration {#server-scheduling-configuration}


### Affinity {#server-scheduling-affinity}

Defines the affinity field for pods in every MCP deployment. This value sets `spec.template.spec.affinity` on Kubernetes deployments and must be a valid [Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#affinity-v1-core) object.

See the [Kubernetes affinity documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity) for details.


### Tolerations {#server-scheduling-tolerations}

Defines the tolerations field for pods in every MCP deployment. This value sets `spec.template.spec.tolerations` on Kubernetes deployments and must be a valid list of [Toleration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core) objects.

See the [Kubernetes taints and tolerations documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) for details.


### Resource Limits & Requests {#server-scheduling-resource-limits--requests}

Defines the CPU and memory requests and limits for pods in every MCP deployment.

See the [Kubernetes resource management documentation](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#requests-and-limits) for details.
