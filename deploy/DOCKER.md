# Sub2API Docker Image

Sub2API is an AI API Gateway Platform for distributing and managing AI product subscription API quotas.

This is an internal build: the in-app update checker, online upgrade and rollback
have been removed, and the payment and third-party login surfaces are trimmed.
Upgrades are performed by swapping the image tag.

## Image Coordinates

The release pipeline always publishes to GHCR, and additionally to Docker Hub when
a Docker Hub account is configured. GHCR is the canonical coordinate:

```
ghcr.io/cherrylover/sub2api:latest
```

## Quick Start

Requires a reachable PostgreSQL 14+ and Redis 6+. With `AUTO_SETUP=true` the
container applies migrations, generates a JWT secret and creates the admin
account on first start (the generated password is printed in the logs).

```bash
docker run -d \
  --name sub2api \
  -p 8080:8080 \
  -e AUTO_SETUP=true \
  -e DATABASE_HOST=postgres-host \
  -e DATABASE_USER=sub2api \
  -e DATABASE_PASSWORD=your-password \
  -e DATABASE_DBNAME=sub2api \
  -e REDIS_HOST=redis-host \
  ghcr.io/cherrylover/sub2api:latest
```

## Docker Compose

Use the maintained compose files in [`deploy/`](./README.md) rather than copying a
snippet from this page: `docker-compose.local.yml` (local directories) and
`docker-compose.yml` (named volumes) ship the full environment, healthchecks and
resource limits. `docker-compose.standalone.yml` runs the app alone against an
external PostgreSQL and Redis.

## Key Environment Variables

Defaults below are the application's own; they assume a local PostgreSQL and
Redis, so a container almost always has to set the host and credential values.

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_HOST` | PostgreSQL host | `localhost` |
| `DATABASE_PORT` | PostgreSQL port | `5432` |
| `DATABASE_USER` | PostgreSQL user | `postgres` |
| `DATABASE_PASSWORD` | PostgreSQL password | `postgres` |
| `DATABASE_DBNAME` | PostgreSQL database name | `sub2api` |
| `DATABASE_SSLMODE` | PostgreSQL SSL mode | `prefer` |
| `REDIS_HOST` | Redis host | `localhost` |
| `REDIS_PORT` | Redis port | `6379` |
| `REDIS_PASSWORD` | Redis password | *(empty)* |
| `AUTO_SETUP` | `true`/`1`/`yes` applies migrations and bootstraps the admin on first start | *(off)* |
| `SERVER_PORT` | Server port | `8080` |
| `SERVER_MODE` | `release` or `debug` | `release` |
| `JWT_SECRET` | JWT secret (set it to keep sessions across restarts) | *(auto-generated)* |
| `TZ` | Timezone | `Asia/Shanghai` |

See [`.env.example`](./.env.example) for the complete list.

## Supported Architectures

- `linux/amd64`
- `linux/arm64`

## Tags

- `latest` - Latest stable release
- `x.y.z` - Specific version
- `x.y` - Latest patch of minor version
- `x` - Latest minor of major version

## Links

- [GitHub Repository](https://github.com/CherryLover/sub2api)
- [Deployment Guide](./README.md)
