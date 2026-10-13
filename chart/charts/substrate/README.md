# Agent Substrate for Obot (experimental)

This bundled chart supports the **experimental** Claude Environments feature.
It packages [Agent Substrate commit 531c935370b9](https://github.com/agent-substrate/substrate/tree/531c935370b9a14463c6588a63102922daebb810), published as
`v0.1.0` in [obot-platform/substrate-images](https://github.com/obot-platform/substrate-images/releases/tag/v0.1.0).

Read the documentation site for:

- [Cluster prerequisites and provider compatibility](../../../docs/docs/claude-environments/cluster-requirements.md)
- [Helm installation, storage, database, and image configuration](../../../docs/docs/claude-environments/installation.md)
- [Maintenance, backup, and removal](../../../docs/docs/claude-environments/operations.md)

Enable the integration with `substrate.enabled=true` in the parent Obot chart and
supply an existing snapshot bucket. It is disabled by default. Install only once
per cluster, without another Substrate installation. The chart uses fixed
`ate-system` and `podcertificate-controller-system` namespaces and cluster-wide
CRDs and signer names.

## Maintaining this package

Templates and CRDs are adapted from upstream `manifests/ate-install`. Keep the
upstream Apache-2.0 headers and LICENSE. Update template sources, generated Go
clients, and runtime images together; an image tag change alone is not a protocol
upgrade. `Chart.yaml` records the upstream commit separately from the published
image release in `appVersion`.

The parent chart supplies the Claude egress Deployment and configuration,
including HTTPS interception, audit capture, and the mTLS credential provider.
Enabling Substrate enables this integration; there is no separate infrastructure-only
switch. Obot creates WorkerPools from environments stored in its database.

The bootstrap Job preserves existing CA/JWT pools, discovers the cluster issuer,
and installs the SandboxConfig after CRDs are established. The bundled database
uses mutual TLS plus a generated SCRAM password in a retained Secret. The chart
limits its network ingress to the API pods; the CNI must enforce NetworkPolicy.

CRDs are rendered as retained Helm resources to support enabling the feature on
an existing release. Namespace and signing-material retention is deliberate.
Follow the operations guide before disabling or uninstalling; Helm uninstall is
not a complete deletion of session state.

Run `bash tools/test-substrate-chart.sh` from the repository root after template
changes. The shell script checks rendered resources, credential wiring, routing,
database variants, and packaged-chart equivalence.
