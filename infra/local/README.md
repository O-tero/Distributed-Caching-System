# Local Infrastructure Guide

Production-grade local development environment for the Distributed Caching & Cache Invalidation System.

## 📦 What's Included

This directory provides Docker Compose configurations and supporting files for local development:

- **PostgreSQL 15** - Audit logs, metrics storage, warming schedules
- **Redis 7** - L2 cache storage with LRU eviction
- **Prometheus** - Metrics collection and monitoring
- **Configuration files** - Optimized for local development

---

## 🚀 Quick Start

### Prerequisites

- **Docker Engine 20.10+** with Compose V2
- **Encore CLI** (install from https://encore.dev/docs/install)
- **Git** (for version control)
- **curl** (for API testing)

#### Install Docker (Ubuntu/Debian)

```bash
# Add Docker's official GPG key
sudo apt-get update
sudo apt-get install ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg

# Add Docker repository
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker Engine
sudo apt-get update
sudo apt-get install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Add your user to docker group (logout/login required)
sudo usermod -aG docker $USER
```

#### Install Encore CLI

```bash
curl -L https://encore.dev/install.sh | bash
```

### Step 1: Configure Environment

```bash
# Copy environment template
cp .env.example .env

# Edit .env with your preferred settings (optional)
nano .env
```

### Step 2: Start Infrastructure

```bash
# Start all infrastructure services
cd infra/local
docker compose up -d

# Check service health
docker compose ps

# View logs
docker compose logs -f
```

### Step 3: Run Application

```bash
# Return to project root
cd ../..

# Start Encore services (from project root)
./scripts/run_local.sh

# Or run Encore manually
encore run
```

### Step 4: Verify Services

```bash
# Check Encore dashboard
# Open: http://localhost:9400

# Check Prometheus metrics
# Open: http://localhost:9090

# Test cache-manager API
curl http://localhost:9400/api/cache/test-key

# Check PostgreSQL
docker compose exec postgres psql -U cache_user -d distributed_cache -c "\dt cache_system.*"

# Check Redis
docker compose exec redis redis-cli ping
```

---

## 🔧 Service Details

### PostgreSQL

- **Port**: 5432 (bound to localhost only)
- **User**: `cache_user` (default)
- **Password**: `changeme_dev_only` (default)
- **Database**: `distributed_cache`
- **Schema**: `cache_system`

**Tables**:
- `invalidation_audit` - Cache invalidation event log
- `metrics_events` - Service metrics storage
- `warming_schedules` - Scheduled cache warming jobs
- `cache_key_metadata` - Key statistics and ownership

**Connect**:
```bash
# From host
psql -h localhost -U cache_user -d distributed_cache

# From Docker
docker compose exec postgres psql -U cache_user -d distributed_cache

# Query examples
SELECT * FROM cache_system.recent_invalidations;
SELECT * FROM cache_system.warming_schedules;
```

### Redis

- **Port**: 6379 (bound to localhost only)
- **Max Memory**: 512MB (configurable in `redis.conf`)
- **Eviction Policy**: `volatile-lru`
- **Persistence**: Disabled (dev mode)

**Configuration**:
- Edit `redis.conf` for memory limits, eviction policies
- Restart required: `docker compose restart redis`

**CLI Access**:
```bash
# Connect to Redis CLI
docker compose exec redis redis-cli

# Useful commands
INFO memory
INFO stats
KEYS *
GET user:123
TTL user:123
```

### Prometheus

- **Port**: 9090 (bound to localhost only)
- **UI**: http://localhost:9090
- **Retention**: 7 days (local storage)

**Scrape Targets**:
- cache-manager (port 9400)
- invalidation (port 9401)
- warming (port 9402)
- monitoring (port 9403)

**Useful Queries**:
```promql
# Cache hit rate
rate(cache_hits_total[5m]) / (rate(cache_hits_total[5m]) + rate(cache_misses_total[5m]))

# P95 latency
histogram_quantile(0.95, rate(cache_latency_bucket[5m]))

# Invalidation rate
rate(invalidations_total[5m])
```

---

## 📜 Available Scripts

All scripts are located in `../../scripts/` directory:

### `run_local.sh` - Start Everything

```bash
# Start infra + Encore services
./scripts/run_local.sh

# Start without infra (infra already running)
./scripts/run_local.sh --no-infra

# Run Encore in foreground (default: background)
./scripts/run_local.sh --foreground

# Stop everything
./scripts/run_local.sh --stop
```

### `seed_data.sh` - Populate Cache

```bash
# Seed 100 sample cache entries
./scripts/seed_data.sh

# Seed specific count
./scripts/seed_data.sh --count 500

# Custom cache-manager URL
./scripts/seed_data.sh --cache-url http://localhost:9400
```

### `load_test.sh` - Performance Testing

```bash
# Run vegeta load test (if installed)
./scripts/load_test.sh --mode vegeta --rate 200 --duration 30s

# Fallback curl loop test
./scripts/load_test.sh --mode curl --rate 50 --duration 10s

# Hotspot pattern (80% requests to 20% keys)
./scripts/load_test.sh --pattern hotspot --rate 300

# Uniform distribution
./scripts/load_test.sh --pattern uniform --rate 300
```

**Install vegeta** (optional but recommended):
```bash
# Ubuntu/Debian
wget https://github.com/tsenart/vegeta/releases/download/v12.11.0/vegeta_12.11.0_linux_amd64.tar.gz
tar xzf vegeta_12.11.0_linux_amd64.tar.gz
sudo mv vegeta /usr/local/bin/
```

### `backup_db.sh` - Backup PostgreSQL

```bash
# Create timestamped backup
./scripts/backup_db.sh

# Backups stored in: infra/local/backups/
```

### `deploy.sh` - Deploy to Staging

```bash
# Deploy to local staging environment
./scripts/deploy.sh --env staging

# Build and deploy
./scripts/deploy.sh --env staging --build
```

---

## 🧪 Development Workflow

### 1. Start Clean Environment

```bash
# Stop and remove all containers
docker compose down -v

# Start fresh
docker compose up -d
./scripts/run_local.sh
```

### 2. Develop and Test

```bash
# Make code changes
# Encore automatically reloads on save

# Test API endpoints
curl -X PUT http://localhost:9400/api/cache/user:123 \
  -H "Content-Type: application/json" \
  -d '{"value": "test data"}'

curl http://localhost:9400/api/cache/user:123
```

### 3. Run Load Tests

```bash
# Seed cache with data
./scripts/seed_data.sh --count 1000

# Run load test
./scripts/load_test.sh --mode vegeta --rate 500 --duration 60s

# Check metrics in Prometheus
open http://localhost:9090
```

### 4. Debug Issues

```bash
# View service logs
docker compose logs -f postgres
docker compose logs -f redis

# Check Encore logs
encore logs

# Inspect Redis data
docker compose exec redis redis-cli
> KEYS *
> GET user:123

# Query audit logs
docker compose exec postgres psql -U cache_user -d distributed_cache
> SELECT * FROM cache_system.invalidation_audit ORDER BY triggered_at DESC LIMIT 10;
```

---

## 🛠️ Common Tasks

### Reset Everything

```bash
# Stop and remove all data
docker compose down -v

# Restart from scratch
docker compose up -d
./scripts/run_local.sh
./scripts/seed_data.sh
```

### Update Configuration

```bash
# Edit Redis config
nano redis.conf

# Edit Postgres init script
nano postgres-init/init.sql

# Edit Prometheus scrape config
nano prometheus.yml

# Restart services
docker compose down
docker compose up -d
```

### View Metrics

```bash
# Prometheus UI
open http://localhost:9090

# Check scrape targets
open http://localhost:9090/targets

# Example query: cache hit rate
# Navigate to: Graph > Execute
# Query: rate(cache_hits_total[5m])
```

### Backup and Restore

```bash
# Backup database
./scripts/backup_db.sh

# Restore from backup
docker compose exec -T postgres psql -U cache_user -d distributed_cache < infra/local/backups/backup_20240115_120000.sql
```

---

## 🔒 Security Notes

### Development Only

This configuration is **NOT production-ready**:

- ❌ No authentication on Redis
- ❌ Weak PostgreSQL password
- ❌ No TLS/SSL encryption
- ❌ Ports bound to localhost (not firewalled)
- ❌ No resource limits
- ❌ No backup strategy

### For Production

Required changes for production deployment:

1. **Secrets Management**
   - Use Vault, AWS Secrets Manager, or similar
   - Never commit passwords to git

2. **Network Security**
   - Enable TLS for all connections
   - Use internal networks, not exposed ports
   - Configure firewall rules

3. **Authentication**
   - Enable Redis AUTH
   - Use strong PostgreSQL passwords
   - Implement RBAC for services

4. **Resource Limits**
   - Set memory/CPU limits in docker-compose
   - Configure disk space alerts

5. **Monitoring**
   - Enable Alertmanager
   - Configure alerts for critical metrics
   - Set up on-call rotations

6. **Backup & Recovery**
   - Automated daily backups
   - Test restore procedures
   - Replicate across regions

---

## 🐛 Troubleshooting

### Services Won't Start

```bash
# Check Docker is running
docker ps

# Check port conflicts
sudo lsof -i :5432
sudo lsof -i :6379
sudo lsof -i :9090

# View container logs
docker compose logs postgres
docker compose logs redis

# Remove and recreate
docker compose down -v
docker compose up -d
```

### Connection Refused Errors

```bash
# Wait for health checks
docker compose ps

# Check service is healthy
docker compose exec postgres pg_isready -U cache_user

# Test connectivity from host
nc -zv localhost 5432
nc -zv localhost 6379
```

### Disk Space Issues

```bash
# Check Docker disk usage
docker system df

# Clean up unused resources
docker system prune -a --volumes

# Check available space
df -h
```

### Encore Service Errors

```bash
# Check Encore logs
encore logs

# Restart Encore dev server
encore daemon restart

# Check service endpoints
curl http://localhost:9400/health
```

### Redis Memory Issues

```bash
# Check Redis memory usage
docker compose exec redis redis-cli INFO memory

# Increase maxmemory in redis.conf
nano redis.conf
# Change: maxmemory 512mb to maxmemory 2gb

# Restart Redis
docker compose restart redis
```

---

## 📚 Additional Resources

- [Encore Documentation](https://encore.dev/docs)
- [Docker Compose Reference](https://docs.docker.com/compose/compose-file/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Redis Documentation](https://redis.io/docs/)
- [Prometheus Documentation](https://prometheus.io/docs/)

---

## 💡 Tips

1. **Use Docker Dashboard** (if available) for visual management
2. **Enable shell aliases** for common commands:
   ```bash
   alias dc='docker compose'
   alias dcl='docker compose logs -f'
   alias dce='docker compose exec'
   ```
3. **Monitor resource usage**: `docker stats`
4. **Keep services updated**: `docker compose pull`
5. **Use .env for customization** - never commit secrets

---
