# OpenAPI Workers local Docker Compose stack

This stack is for a local, isolated demo. It starts PostgreSQL, two stateless OpenAPI Workers, and Obot with the fixed OpenAPI Worker backend. It does not create Kubernetes resources and does not mount the Docker socket.

Generate synthetic certificates and start:

```bash
cd deploy/compose
openssl rand -hex 16 > /tmp/openapi-compose-password
./generate-local-certs.sh
OBOT_POSTGRES_PASSWORD="$(cat /tmp/openapi-compose-password)" docker compose up -d --build
``+

Open `http://127.0.0.1:18080`. The generated files under `runtime/` and `.env` are ignored; delete them after the demo if desired.
