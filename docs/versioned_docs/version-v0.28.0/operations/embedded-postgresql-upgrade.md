---
title: Upgrade embedded PostgreSQL from 17 to 18
---

Obot's embedded database now uses PostgreSQL 18. Existing PostgreSQL 17 data directories require a dump and restore before starting the new image. Obot exits with a migration link when it finds an incompatible directory.

This affects Docker installations without `OBOT_SERVER_DSN` and Helm installations using `dev.useEmbeddedDb: true`. The embedded database is for development and evaluation. External PostgreSQL installations and Substrate's separate database do not require this migration.

## Before you begin

Plan downtime for the entire procedure. Keep Obot stopped while taking the final dump and restoring it: starting the application can create schemas and write data before restoration finishes.

- Record the old image digest, new image tag, deployment settings, volume/PVC, and database credentials. Use an explicit new image tag, not `latest`.
- Preserve all other files under `/data` and access to the original encryption keys. See [Backup and recovery](./backup.md).
- Allow enough space for the old cluster, new cluster, and SQL dump. Keep the dump outside the volume being migrated, with restricted permissions; it contains data and role password hashes.
- `pg_dumpall` includes every database on this server, including `<database>_llm_gateway`. A gateway database configured on another server through `OBOT_SERVER_LLM_GATEWAY_POSTGRES_DSN` needs its own coordinated backup.
- These steps assume the default tablespaces. If you added tablespaces or customized PostgreSQL configuration, preserve their storage and reproduce those settings before restoring.

Run the commands in Bash and stop if any command fails. The examples use the database superuser `obot` and `/data/postgresql`; substitute your configured values. Keep the old image and old directory until all verification is complete.

## Docker

### Stop Obot and dump PostgreSQL 17

Set the container name, named volume, new image, and database settings:

```bash
umask 077
CONTAINER=obot
DATA_VOLUME=obot-data
NEW_IMAGE='ghcr.io/obot-platform/obot:<new-release-tag>'
DB_USER=obot
OLD_IMAGE=$(docker inspect --format '{{.Image}}' "$CONTAINER")

docker inspect "$CONTAINER" > obot-container-before-upgrade.json
docker exec "$CONTAINER" sh -c 'printf "POSTGRES_USER=%s\nPOSTGRES_PASSWORD=%s\nPGDATA=%s\n" "$POSTGRES_USER" "$POSTGRES_PASSWORD" "$PGDATA"' > postgres.env
docker stop --time 60 "$CONTAINER"
```

For a bind mount, replace each `--mount "type=volume,source=$DATA_VOLUME,target=/data"` below with your original bind mount. Confirm the volume from the saved container configuration. Do not remove it.

Start only PostgreSQL using the old image. No database port is exposed; commands connect over its Unix socket.

```bash
docker run -d --name obot-pg-maintenance --network none \
  --mount "type=volume,source=$DATA_VOLUME,target=/data" \
  --env-file postgres.env -e POSTGRES_DB=postgres \
  --entrypoint /usr/bin/docker-entrypoint.sh \
  "$OLD_IMAGE" postgres -c listen_addresses=

docker exec obot-pg-maintenance pg_isready -U "$DB_USER" -d postgres
```

Wait for readiness before continuing. The version query must report `17` as its major version.

```bash
docker exec obot-pg-maintenance psql -X -U "$DB_USER" -d postgres -c 'SHOW server_version;'
docker exec obot-pg-maintenance pg_dumpall -U "$DB_USER" --quote-all-identifiers > obot-backup.sql
test -s obot-backup.sql
docker stop --time 60 obot-pg-maintenance
docker rm obot-pg-maintenance
```

### Initialize PostgreSQL 18 and restore

Retain the old directory. The guard prevents overwriting an earlier backup.

```bash
docker run --rm --network none \
  --mount "type=volume,source=$DATA_VOLUME,target=/data" \
  --env-file postgres.env --entrypoint sh "$NEW_IMAGE" -ec \
  'test "$(cat "$PGDATA/PG_VERSION")" = 17; test ! -e "$PGDATA-17-backup"; mv "$PGDATA" "$PGDATA-17-backup"'

docker run -d --name obot-pg-maintenance --network none \
  --mount "type=volume,source=$DATA_VOLUME,target=/data" \
  --env-file postgres.env -e POSTGRES_DB=postgres \
  --entrypoint /usr/bin/docker-entrypoint.sh \
  "$NEW_IMAGE" postgres -c listen_addresses=

docker exec obot-pg-maintenance sh -c \
  'test "$(head -n 1 "$PGDATA/postmaster.pid")" = 1 && pg_isready -U "$POSTGRES_USER" -d postgres'
```

Wait until the last command succeeds. Checking PID 1 avoids mistaking the entrypoint's temporary initialization server for the final server. `POSTGRES_DB=postgres` is used only for maintenance, so the application database does not already exist when the dump creates it.

```bash
docker exec -i obot-pg-maintenance psql -X -U "$DB_USER" -d postgres \
  -v ON_ERROR_STOP=off -v VERBOSITY=terse \
  < obot-backup.sql > restore.out 2> restore.log
```

Complete [Check the restore](#check-the-restore) before starting Obot. Then refresh statistics and stop maintenance:

```bash
docker exec obot-pg-maintenance vacuumdb -U "$DB_USER" --all --analyze
docker exec obot-pg-maintenance psql -X -U "$DB_USER" -d postgres -c '\l'
docker stop --time 60 obot-pg-maintenance
docker rm obot-pg-maintenance
```

Recreate Obot using the new image and its original mounts, network, ports, authentication, encryption, and other settings from your deployment configuration. Restore the original `POSTGRES_DB`; do not use the maintenance override. The stopped old container must be removed or renamed before reusing its name. Keep the original data volume attached and perform [Verify Obot](#verify-obot).

## Helm with `dev.useEmbeddedDb: true`

### Record the installation and stop Obot

Use one Obot replica and persistent storage for this procedure. Suspend any GitOps controller or autoscaler that would undo the temporary scale-down. Do not uninstall the release or delete its PVC.

```bash
umask 077
NS=obot
RELEASE=obot
DEPLOYMENT=obot
POD='<current-obot-pod>'
PVC='<existing-data-pvc>'
OLD_IMAGE='<recorded-pg17-image-with-digest>'
NEW_IMAGE='ghcr.io/obot-platform/obot:<new-release-tag>'
DB_USER=obot

helm get values "$RELEASE" -n "$NS" -o yaml > obot-values-before-upgrade.yaml
kubectl get deployment "$DEPLOYMENT" -n "$NS" -o yaml > obot-deployment-before-upgrade.yaml
kubectl exec -n "$NS" "$POD" -c obot -- sh -c 'printf "POSTGRES_USER=%s\nPOSTGRES_PASSWORD=%s\nPGDATA=%s\n" "$POSTGRES_USER" "$POSTGRES_PASSWORD" "$PGDATA"' > postgres.env
kubectl scale deployment "$DEPLOYMENT" -n "$NS" --replicas=0
kubectl wait -n "$NS" --for=delete "pod/$POD" --timeout=180s
kubectl create secret generic obot-pg-migration -n "$NS" --from-env-file=postgres.env
```

Obtain the PVC name from the deployment's `data` volume, including when `persistence.existingClaim` is configured. Confirm all Obot pods have stopped before mounting it elsewhere. The environment file records the actual `PGDATA`. Mount the volume at the same path used by the original deployment if it differs from `/data`.

### Dump, replace the data directory, and restore

Define this helper to create a temporary pod. It has no Obot service labels or HTTP probes. For a private image, add your existing `imagePullSecrets` under `spec`; carry over any node scheduling requirements needed to mount the PVC.

```bash
pg_pod() {
  kubectl create -n "$NS" -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: obot-pg-maintenance
spec:
  restartPolicy: Never
  terminationGracePeriodSeconds: 60
  containers:
    - name: postgres
      image: "$1"
      command: ["/bin/sh", "-ec"]
      args:
        - |
          $2
      envFrom:
        - secretRef:
            name: obot-pg-migration
      env:
        - name: POSTGRES_DB
          value: postgres
      volumeMounts:
        - name: data
          mountPath: /data
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: "$PVC"
EOF
}

pg_pod "$OLD_IMAGE" 'exec /usr/bin/docker-entrypoint.sh postgres -c listen_addresses='
kubectl wait -n "$NS" --for=condition=Ready pod/obot-pg-maintenance --timeout=180s
kubectl exec -n "$NS" obot-pg-maintenance -- pg_isready -U "$DB_USER" -d postgres
```

Wait for PostgreSQL readiness, then take the dump and copy it off the pod. A pod-local dump alone is not a backup.

```bash
kubectl exec -n "$NS" obot-pg-maintenance -- sh -ec \
  'umask 077; pg_dumpall -U "$POSTGRES_USER" --quote-all-identifiers -f /tmp/obot-backup.sql'
kubectl cp -n "$NS" obot-pg-maintenance:/tmp/obot-backup.sql ./obot-backup.sql
test -s obot-backup.sql
kubectl delete pod obot-pg-maintenance -n "$NS" --wait=true

pg_pod "$NEW_IMAGE" 'test "$(cat "$PGDATA/PG_VERSION")" = 17; test ! -e "$PGDATA-17-backup"; mv "$PGDATA" "$PGDATA-17-backup"'
kubectl wait -n "$NS" --for=jsonpath='{.status.phase}'=Succeeded pod/obot-pg-maintenance --timeout=180s
kubectl delete pod obot-pg-maintenance -n "$NS" --wait=true

pg_pod "$NEW_IMAGE" 'exec /usr/bin/docker-entrypoint.sh postgres -c listen_addresses='
kubectl wait -n "$NS" --for=condition=Ready pod/obot-pg-maintenance --timeout=180s
kubectl exec -n "$NS" obot-pg-maintenance -- sh -c \
  'test "$(head -n 1 "$PGDATA/postmaster.pid")" = 1 && pg_isready -U "$POSTGRES_USER" -d postgres'
```

Wait until the final command succeeds, then restore:

```bash
kubectl exec -i -n "$NS" obot-pg-maintenance -- psql -X -U "$DB_USER" -d postgres \
  -v ON_ERROR_STOP=off -v VERBOSITY=terse \
  < obot-backup.sql > restore.out 2> restore.log
```

Complete [Check the restore](#check-the-restore), then:

```bash
kubectl exec -n "$NS" obot-pg-maintenance -- vacuumdb -U "$DB_USER" --all --analyze
kubectl exec -n "$NS" obot-pg-maintenance -- psql -X -U "$DB_USER" -d postgres -c '\l'
kubectl delete pod obot-pg-maintenance -n "$NS" --wait=true
kubectl delete secret obot-pg-migration -n "$NS"
```

Upgrade the existing Helm release through your normal deployment process, preserving its values, secrets, PVC, and original database environment. Set the target chart/image versions, `dev.useEmbeddedDb: true`, and `replicaCount: 1`. Resume reconciliation only after the desired configuration uses the new image. Wait for the deployment rollout and perform the checks below.

## Check the restore

The new cluster already contains the bootstrap superuser. PostgreSQL documents the resulting `CREATE ROLE` error as [harmless](https://www.postgresql.org/docs/18/app-pg-dumpall.html#APP-PG-DUMPALL-NOTES). This is the only SQL error these instructions permit. The later `ALTER ROLE` statements restore that user's attributes and password.

`psql` can exit successfully after SQL errors when `ON_ERROR_STOP` is off. Require both a successful restore command and this diagnostic check:

```bash
grep -E '(ERROR|FATAL|PANIC):' restore.log > restore-errors.txt
printf 'ERROR:  role "%s" already exists\n' "$DB_USER" > expected-restore-error.txt
cmp restore-errors.txt expected-restore-error.txt
```

Read `restore.log` as well. If the comparison fails or the restore command fails, keep Obot stopped and investigate. Do not restore again into a partially populated cluster; retain that failed directory separately and initialize another empty cluster before retrying. Never continue merely because the last SQL statement succeeded.

## Verify Obot

- Confirm PostgreSQL reports version 18 and both the application and gateway databases exist with their expected owners and records.
- Sign in with an existing user. Check existing MCP servers, then make a harmless call through a virtual MCP server.
- Confirm saved credentials still decrypt, gateway configuration is present, a representative model request succeeds, and retained MCP/LLM audit records are visible.
- Restart Obot once and repeat the health and data checks.

Keep the dump, previous image, original deployment configuration, and `PGDATA-17-backup` until these checks pass. Retain backups according to your normal policy; protect or remove temporary credential files when finished.

## Recovery and rollback

If you upgraded before taking a dump, the startup check leaves the PG17 directory intact. Recreate the Docker container or roll back the Helm workload to the previous image with its original configuration and volume. Once it is running, begin the procedure above to record its settings, stop Obot, and take the PostgreSQL-only dump. If the old image is no longer local, retrieve the exact previous tag/digest from your recorded configuration.

To roll back after initializing PG18, stop Obot and all maintenance database processes. Using a temporary container/pod with the volume mounted and a shell entrypoint, rename the new `$PGDATA` to a separate failed-upgrade directory, then move `$PGDATA-17-backup` back to `$PGDATA`. Start the previous Obot image with its original configuration. Do not run PG17 against the PG18 directory. A Helm rollback alone does not restore the database.

Rollback returns to the backup's recovery point and loses subsequent writes. Preserve the new directory for investigation rather than deleting it.
