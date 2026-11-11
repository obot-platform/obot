# OpenAPI Workers Kubernetes example

This example assumes that the Namespace, PostgreSQL Service/Secret, image pull policy, ingress, and TLS entry point already exist. It creates only Obot/OpenAPI Worker workloads and supporting objects inside the current Namespace.

Before applying, build or import the two images, then replace every `REPLACE_WITH_...` Secret value. The Worker certificate SAN must include `openapi-worker` and `localhost`; the Obot client and Worker prober certificates are issued by `client-ca.crt`. Generate all test material with OpenSSL or another approved internal tool. Never use a real business token, JWT, database password, or production key in this example.

The integration Secret intentionally holds both sides' synthetic mTLS material in one object for a compact example. Production should split it into `obot-mtls-client`, `openapi-worker-mtls-server`, and `openapi-worker-prober` Secrets so each workload receives only its own private keys. PostgreSQL is reused through `server-dsn`; no database credential is sent to Workers.

Apply and verify with:

```bash
kubectl apply --dry-run=client -f deploy/k8s/openapi-workers.example.yaml
kubectl apply -f deploy/k8s/openapi-workers.example.yaml
kubectl rollout status deployment/obot
kubectl rollout status deployment/openapi-worker
```

The manifest does not create a Namespace, ClusterRole, ClusterRoleBinding, ServiceAccount, Role, or RoleBinding, and it does not mount `/var/run/docker.sock`. Worker Pods are stateless: scaling or rolling restarts rely on Obot to resend immutable snapshots. Use the existing PostgreSQL migration/state; no Worker database volume is created.
