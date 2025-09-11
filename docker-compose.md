# Obot Docker Compose Configuration

This Docker Compose configuration runs Obot with external PostgreSQL, plus development and database management tools.

## What's Included

The `docker-compose.yml` provides:
- **PostgreSQL**: pgvector-enabled PostgreSQL 17 database
- **Obot**: Connected to external PostgreSQL with debug logging
- **pgAdmin**: Web-based database management interface
- **Features**:
  - PostgreSQL with query logging enabled
  - Obot with debug mode and gateway debugging
  - pgAdmin for database management (accessible at http://localhost:8081)
  - Volume mounts for agents directory
- **Credentials**:
  - pgAdmin: admin@obot.local / admin

## Quick Start

```bash
# Start all services (PostgreSQL + Obot + pgAdmin)
docker compose up -d

# View logs
docker compose logs -f

# View just Obot logs to see database connection logging
docker compose logs -f obot

# Stop the services
docker compose down
```

## Access Points

- **Obot**: http://localhost:8080
- **pgAdmin**: http://localhost:8081 (admin@obot.local / admin)
- **PostgreSQL**: localhost:5432 (if you need direct database access)

## Environment Variables

### Obot Configuration
- `OBOT_SERVER_DSN`: Database connection string
- `OBOT_SERVER_HTTP_LISTEN_PORT`: HTTP port for Obot server
- `OBOT_SERVER_DEV_MODE`: Enable development mode
- `OBOT_SERVER_ENABLE_AUTHENTICATION`: Enable authentication (production)
- `OBOT_SERVER_GATEWAY_DEBUG`: Enable gateway debugging

### PostgreSQL Configuration
- `POSTGRES_USER`: Database user
- `POSTGRES_PASSWORD`: Database password
- `POSTGRES_DB`: Database name
- `PGDATA`: Data directory path

## Networking

### Container Communication
- Obot connects to PostgreSQL using hostname `postgres`
- Both containers are on the same Docker network
- No external PostgreSQL access needed

### Port Mapping
- **Obot**: http://localhost:8080
- **PostgreSQL** (dev only): localhost:5432
- **pgAdmin** (dev only): http://localhost:8081

## Data Persistence

Docker volumes are used for data persistence:
- `postgres_dev_data`: PostgreSQL database data
- `obot_dev_data`: Obot application data
- `pgadmin_dev_data`: pgAdmin settings and data

## Health Checks

All services include health checks:
- **PostgreSQL**: Uses `pg_isready` command
- **Obot**: HTTP request to `/api/version` endpoint

## Database Connection Logging

The recent database connection logging improvements will show:
```
INFO Connecting to database dsn=postgresql://obot:obot@postgres:5432/obot
INFO Successfully connected to database dsn=postgresql://obot:obot@postgres:5432/obot  
INFO Initializing gateway database connection
INFO Running database migrations
INFO Database migrations completed successfully
```

This helps diagnose connection issues when connecting to remote PostgreSQL instances.

## Troubleshooting

### Connection Issues
1. Check if PostgreSQL is healthy: `docker compose ps`
2. View PostgreSQL logs: `docker compose logs postgres`
3. Verify network connectivity: `docker compose exec obot pg_isready -h postgres -U obot`

### Permission Issues
- Ensure Docker has permissions to bind to specified ports
- Check that volume mount paths exist and are accessible

### Database Issues
- Check PostgreSQL logs for connection errors
- Verify pgvector extension is installed: `docker compose exec postgres psql -U obot -c "SELECT * FROM pg_extension WHERE extname = 'vector';"`

## Migration from All-in-One

To migrate from the all-in-one container:
1. Backup your data: `docker exec <container> pg_dump -U obot obot > backup.sql`
2. Start the new setup: `docker compose up -d`
3. Wait for services to be healthy
4. Restore data: `docker compose exec postgres psql -U obot obot < backup.sql`

## Security Notes

For production:
- Change default passwords
- Use Docker secrets for sensitive data
- Restrict network access to PostgreSQL
- Enable SSL/TLS connections
- Regular security updates
- Monitor database access logs