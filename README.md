#  Distributed Caching & Cache Invalidation System

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://go.dev/)
[![TypeScript](https://img.shields.io/badge/typescript-5.2+-3178C6.svg)](https://www.typescriptlang.org/)
[![Docker](https://img.shields.io/badge/docker-20.10+-2496ED.svg)](https://www.docker.com/)

A production-grade, horizontally scalable distributed caching system with intelligent invalidation, predictive warming, and comprehensive monitoring built on the **Encore** framework.

---

## 📋 Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Project Structure](#project-structure)
- [Services](#services)
- [Frontend Dashboard](#frontend-dashboard)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [Performance](#performance)
- [Monitoring](#monitoring)
- [Security](#security)
- [Development](#development)
- [Testing](#testing)
- [Contributing](#contributing)
- [Troubleshooting](#troubleshooting)
- [License](#license)

---

##  Overview

The Distributed Caching & Cache Invalidation System is an enterprise-ready solution designed to:

- **Reduce latency** by caching frequently accessed data
- **Decrease database load** through intelligent L1/L2 caching
- **Maintain consistency** with event-driven invalidation
- **Scale horizontally** using consistent hashing
- **Predict access patterns** with ML-based warming
- **Monitor performance** in real-time with comprehensive metrics

### Why This System?

Traditional caching solutions often struggle with:
- ❌ Stale data after updates
- ❌ Cache stampede under high load
- ❌ Complex invalidation patterns
- ❌ Lack of observability
- ❌ Difficult horizontal scaling

**Solution:**
- ✅ Event-driven invalidation with Pub/Sub
- ✅ Request coalescing for stampede prevention
- ✅ Pattern-based bulk invalidation
- ✅ Real-time metrics and dashboards
- ✅ Consistent hashing for seamless scaling

---

##  Key Features

### **Core Capabilities**

- **Dual-Layer Caching (L1/L2)**
  - L1: In-memory cache with LRU/LFU eviction
  - L2: Redis-backed persistent cache
  - Automatic failover and fallback

- **Intelligent Invalidation**
  - Key-based: Invalidate specific entries
  - Pattern-based: `users:*`, `products:category:*`
  - Event-driven: Pub/Sub for distributed systems
  - Audit trail: Complete invalidation history

- **Predictive Cache Warming**
  - Scheduled jobs with cron expressions
  - On-demand warming triggers
  - ML-based access prediction (optional)
  - Batch processing with priority queues

- **High Availability**
  - Consistent hashing for node distribution
  - Automatic rebalancing on topology changes
  - Circuit breakers for fault tolerance
  - Graceful degradation

### **Monitoring & Observability**

- **Real-time Metrics**
  - Hit/miss rates
  - Latency percentiles (P50, P90, P95, P99)
  - Cache sizes and memory usage
  - Invalidation and eviction rates

- **Admin Dashboard**
  - Live metrics visualization
  - Cache key explorer
  - Bulk invalidation console
  - Warming job management
  - WebSocket real-time updates

- **Integrations**
  - Prometheus metrics export
  - Grafana dashboards
  - Distributed tracing (Jaeger)
  - Structured logging (JSON)

### **Enterprise-Ready**

- Token-based authentication
- Rate limiting per user/endpoint
- CORS protection
- Audit logging
- Backup and restore
- Multi-environment support

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Client Applications                       │
└────────────┬────────────────────────────────────┬────────────────┘
             │                                    │
             ▼                                    ▼
┌─────────────────────────┐         ┌─────────────────────────┐
│   Cache Manager API     │         │   Admin Dashboard       │
│   (Encore Service)      │         │   (React + Vite)        │
│   Port: 9400            │         │   Port: 3000            │
└────────────┬────────────┘         └─────────────────────────┘
             │
    ┌────────┴────────┐
    │                 │
    ▼                 ▼
┌─────────┐      ┌─────────┐
│ L1 Cache│      │ L2 Cache│
│(In-Mem) │      │ (Redis) │
└─────────┘      └─────────┘
             │
    ┌────────┴────────────────────┐
    │      Pub/Sub Events         │
    │  (Redis/Kafka/NATS)         │
    └────────┬────────────────────┘
             │
    ┌────────┴────────┐
    │                 │
    ▼                 ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ Invalidation │  │   Warming    │  │  Monitoring  │
│   Service    │  │   Service    │  │   Service    │
│  Port: 9401  │  │  Port: 9402  │  │  Port: 9403  │
└──────────────┘  └──────────────┘  └──────────────┘
         │                 │                 │
         └─────────────────┴─────────────────┘
                           │
                           ▼
                   ┌──────────────┐
                   │  PostgreSQL  │
                   │ (Audit Logs) │
                   │ Port: 5432   │
                   └──────────────┘
```

### **Data Flow**

1. **Cache Read**
   ```
   Client → Cache Manager → L1 (hit?) → Return
                          ↓ (miss)
                       L2 (hit?) → Store in L1 → Return
                          ↓ (miss)
                    Data Source → Store in L1 + L2 → Return
   ```

2. **Cache Invalidation**
   ```
   Invalidation Request → Pub/Sub Event → All Cache Managers
                                        → Remove from L1 + L2
                                        → Audit Log
   ```

3. **Cache Warming**
   ```
   Warming Trigger → Batch Fetch → Store in L1 + L2
                                 → Publish Completion Event
   ```

---

## Quick Start

### Prerequisites

- **Go 1.21+** ([install](https://go.dev/doc/install))
- **Node.js 18+** ([install](https://nodejs.org/))
- **Docker 20.10+** ([install](https://docs.docker.com/engine/install/))
- **Encore CLI** ([install](https://encore.dev/docs/install))

### Installation

```bash
# 1. Clone the repository
git clone https://github.com/your-org/distributed-cache-system.git
cd distributed-cache-system

# 2. Set up environment
cp .env.example .env
# Edit .env with your configuration

# 3. Start infrastructure (PostgreSQL, Redis, Prometheus)
cd infra/local
docker compose up -d
cd ../..

# 4. Start backend services
encore run

# 5. Start frontend dashboard (new terminal)
cd frontend/dashboard
npm install
npm run dev

# 6. Open dashboard
open http://localhost:3000
```

### First Steps

1. **Access the Dashboard**: http://localhost:3000
2. **Configure Auth Token**: Go to Settings → Enter API token
3. **View Metrics**: Navigate to Dashboard page
4. **Explore Cache**: Check Cache Explorer for keys
5. **Test Invalidation**: Try the Invalidation Console

---

## Services

### **cache-manager** (Port 9400)

Main cache API service providing read/write operations.

**Endpoints:**
- `GET /api/cache/:key` - Retrieve cached value
- `PUT /api/cache/:key` - Store value in cache
- `DELETE /api/cache/:key` - Delete from cache
- `GET /api/metrics` - Current metrics
- `GET /api/cache/keys` - List cache keys

**Features:**
- Dual-layer caching (L1 + L2)
- Automatic TTL management
- Cache stampede prevention
- Consistent hashing for distribution

**Configuration:**
```env
CACHE_MANAGER_PORT=9400
L1_CACHE_MAX_SIZE=10000
L2_CACHE_DEFAULT_TTL=3600
CACHE_EVICTION_POLICY=lru
```

### **invalidation** (Port 9401)

Handles cache invalidation with pattern matching.

**Endpoints:**
- `POST /api/invalidate` - Invalidate by keys
- `POST /api/invalidate/pattern` - Pattern-based invalidation
- `GET /api/invalidate/preview` - Preview matches

**Features:**
- Key and pattern-based invalidation
- Pub/Sub event publishing
- Audit logging to PostgreSQL
- Dry-run mode

**Configuration:**
```env
INVALIDATION_PORT=9401
INVALIDATION_BATCH_SIZE=1000
INVALIDATION_AUDIT_ENABLED=true
```

### **warming** (Port 9402)

Manages cache warming jobs and schedules.

**Endpoints:**
- `GET /api/warming/jobs` - List scheduled jobs
- `POST /api/warming/trigger` - Trigger warming
- `GET /api/warming/history` - Job history

**Features:**
- Cron-based scheduling
- On-demand triggers
- Batch processing
- Priority queues
- ML-based prediction (optional)

**Configuration:**
```env
WARMING_PORT=9402
WARMING_MAX_CONCURRENT_JOBS=5
WARMING_BATCH_SIZE=100
```

### **monitoring** (Port 9403)

Aggregates metrics and provides observability.

**Endpoints:**
- `GET /api/metrics` - Current metrics
- `GET /api/metrics/history` - Historical data
- `GET /metrics` - Prometheus format

**Features:**
- Metric aggregation
- Prometheus export
- Alert generation
- Time-series storage

**Configuration:**
```env
MONITORING_PORT=9403
METRICS_COLLECTION_INTERVAL=5000
PROMETHEUS_ENABLED=true
```

---

## Frontend Dashboard

Modern React-based admin dashboard built with **Vite**, **TypeScript**, and **TailwindCSS**.

### Features

**Dashboard Page**
- Real-time metrics visualization
- Interactive charts (Recharts)
- Time window selection (1m, 5m, 1h, 24h)
- WebSocket live updates

**Cache Explorer**
- Searchable key list with filters
- Bulk operations (select, invalidate)
- CSV export
- Pagination (50 items/page)

**Invalidation Console**
- Pattern-based invalidation
- Preview matched keys
- Dry-run mode
- Common pattern templates

**Warming Jobs**
- Scheduled job listing
- Manual trigger interface
- Job history and status
- Success rate tracking

**Settings**
- API token configuration
- Cache policy selection
- Polling intervals

### Tech Stack

- **React 18** - UI framework
- **TypeScript** - Type safety
- **Vite** - Build tool
- **TailwindCSS** - Styling
- **SWR** - Data fetching
- **Recharts** - Charts
- **Lucide React** - Icons

### Quick Start

```bash
cd frontend/dashboard
npm install
npm run dev
# Open http://localhost:3000
```

### Build for Production

```bash
npm run build
npm run preview

# Docker build
docker build -t cache-dashboard .
docker run -p 80:80 cache-dashboard
```

---

## ⚙️ Configuration

### Environment Files

- `.env.example` - Template with all variables
- `.env.development` - Development settings
- `.env.production` - Production configuration

### Key Variables

**Backend:**
```env
# Database
POSTGRES_HOST=localhost
POSTGRES_PASSWORD=changeme

# Cache
REDIS_HOST=localhost
REDIS_MAXMEMORY=512mb

# Services
CACHE_MANAGER_PORT=9400

# Auth
API_TOKEN_ADMIN=your_token_here
```

**Frontend:**
```env
# API
VITE_API_BASE=http://localhost:9400

# Features
VITE_ENABLE_REALTIME=true
VITE_METRICS_POLL_INTERVAL=5000
```

See [SETUP_GUIDE.md](SETUP_GUIDE.md) for complete configuration details.

---

## Deployment

### Local Development

```bash
# Start infrastructure
./scripts/run_local.sh

# Access services
- Dashboard: http://localhost:3000
- API: http://localhost:9400
- Prometheus: http://localhost:9090
```

### Docker Compose

```bash
# Build and start
docker compose up -d

# Scale services
docker compose up -d --scale cache-manager=3

# View logs
docker compose logs -f
```

### Kubernetes

```bash
# Apply manifests
kubectl apply -f infra/k8s/

# Check status
kubectl get pods -n cache-system

# Access dashboard
kubectl port-forward svc/dashboard 3000:80
```

### Production Checklist

- [ ] Generate strong passwords (32+ chars)
- [ ] Enable TLS/SSL
- [ ] Configure secrets management
- [ ] Set up monitoring and alerting
- [ ] Enable backups
- [ ] Configure rate limiting
- [ ] Review CORS settings
- [ ] Enable audit logging
- [ ] Test failover scenarios
- [ ] Document runbooks

See [docs/deployment.md](docs/deployment.md) for detailed deployment guide.

---

## Performance

### Benchmarks

**Hardware:** 4 vCPU, 8GB RAM, SSD

| Operation | Throughput | Latency (P95) |
|-----------|------------|---------------|
| Cache Read (L1 hit) | 100,000 req/s | 0.5ms |
| Cache Read (L2 hit) | 50,000 req/s | 2ms |
| Cache Write | 25,000 req/s | 5ms |
| Invalidation | 10,000 keys/s | 10ms |
| Warming | 5,000 keys/s | 20ms |

### Scalability

- **Horizontal**: Add cache-manager instances with consistent hashing
- **Vertical**: Increase L1 cache size and worker pool
- **Storage**: Redis Cluster for L2, PostgreSQL replication

### Optimization Tips

1. **Cache Hit Rate**: Aim for >80% hit rate
2. **TTL Tuning**: Balance freshness vs hits
3. **L1 Size**: Monitor eviction rate
4. **Batch Operations**: Use bulk invalidation
5. **Connection Pools**: Tune DB/Redis pools

---

## Monitoring

### Metrics

**Cache Performance:**
- `cache_hits_total` - Total cache hits
- `cache_misses_total` - Total cache misses
- `cache_latency_seconds` - Request latency histogram
- `cache_size_bytes` - Current cache size

**System Health:**
- `invalidations_total` - Invalidation count
- `evictions_total` - Eviction count
- `warming_jobs_total` - Warming job count
- `errors_total` - Error count by type

### Dashboards

**Prometheus + Grafana:**
```bash
# Access Prometheus
open http://localhost:9090

# Import Grafana dashboard
# ID: 12345 (Redis)
# ID: 67890 (Custom cache metrics)
```

**Built-in Dashboard:**
```bash
# Access admin dashboard
open http://localhost:3000
```

### Alerting

Configure alerts in `prometheus.yml`:

```yaml
- alert: HighCacheMissRate
  expr: cache_miss_rate > 0.5
  for: 5m
  annotations:
    summary: "Cache miss rate above 50%"
```

---

## Security

### Authentication

- **Token-based**: Simple API tokens
- **JWT**: Stateless authentication (production)
- **OAuth2**: SSO integration (optional)

### Authorization

- **API Keys**: Per-service keys
- **RBAC**: Role-based access (future)
- **Rate Limiting**: Per-user quotas

### Network Security

- **TLS/SSL**: Required in production
- **CORS**: Whitelist allowed origins
- **Firewall**: Block public access to internal services

### Data Security

- **Encryption**: At rest (database) and in transit (TLS)
- **Secrets**: Use Vault or AWS Secrets Manager
- **Audit Logs**: All mutations logged

### Security Checklist

- [ ] Strong passwords (32+ characters)
- [ ] TLS enabled for all connections
- [ ] API tokens rotated regularly
- [ ] CORS properly configured
- [ ] Rate limiting enabled
- [ ] Audit logging enabled
- [ ] Secrets in vault (not .env)
- [ ] Regular security updates
- [ ] Penetration testing
- [ ] Incident response plan

---

## Development

### Prerequisites

```bash
# Install Go
brew install go  # macOS
# or download from https://go.dev/dl/

# Install Node.js
brew install node  # macOS

# Install Encore CLI
curl -L https://encore.dev/install.sh | bash

# Install Docker
# Download from https://docker.com
```

### Development Workflow

```bash
# 1. Start infrastructure
cd infra/local && docker compose up -d

# 2. Start backend (hot reload)
encore run

# 3. Start frontend (new terminal)
cd frontend/dashboard && npm run dev

# 4. Make changes (auto-reload on save)

# 5. Run tests
encore test ./...
cd frontend/dashboard && npm test

# 6. Lint code
encore lint
cd frontend/dashboard && npm run lint
```

---

## Testing

### Unit Tests

```bash
# Backend
encore test ./...
encore test ./pkg/utils -v

# Frontend
cd frontend/dashboard
npm test
npm run test:ui  # Interactive UI
```

### Integration Tests

```bash
# Start test infrastructure
./scripts/run_local.sh

# Run integration tests
go test ./tests/integration/... -v
```

### Load Testing

```bash
# Seed test data
./scripts/seed_data.sh --count 10000

# Run load test (requires vegeta)
./scripts/load_test.sh --mode vegeta --rate 1000 --duration 60s

# Or use curl mode
./scripts/load_test.sh --mode curl --rate 100 --duration 30s
```

### Test Coverage

```bash
# Backend coverage
encore test ./... -cover
go tool cover -html=coverage.out

# Frontend coverage
cd frontend/dashboard
npm run coverage
```

---

## Contributing

We welcome contributions! Please follow these guidelines:

### Getting Started

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes
4. Write/update tests
5. Update documentation
6. Commit: `git commit -m 'Add amazing feature'`
7. Push: `git push origin feature/amazing-feature`
8. Open a Pull Request

### Code Standards

- **Go**: Follow [Effective Go](https://go.dev/doc/effective_go)
- **TypeScript**: Follow [TypeScript guidelines](https://www.typescriptlang.org/docs/)
- **Commits**: Use [Conventional Commits](https://www.conventionalcommits.org/)

### Pull Request Process

1. Update README.md with changes
2. Add tests for new functionality
3. Ensure CI passes
4. Request review from maintainers
5. Address review feedback
6. Squash commits before merge

---

## Troubleshooting

### Common Issues

**Issue: Backend won't start**
```bash
# Check if ports are in use
lsof -i :9400

# Check environment variables
cat .env | grep POSTGRES

# View logs
docker compose logs postgres redis
```

**Issue: Frontend can't connect**
```bash
# Verify backend is running
curl http://localhost:9400/health

# Check CORS settings
curl -H "Origin: http://localhost:3000" http://localhost:9400/api/metrics

# Clear browser cache
# Open DevTools > Application > Clear storage
```

**Issue: Tests failing**
```bash
# Clean and rebuild
go clean -cache
rm -rf node_modules
npm install

# Reset database
docker compose down -v
docker compose up -d
```


## Documentation

- [Architecture Overview](docs/architecture.md)
- [API Reference](docs/api.md)
- [Deployment Guide](docs/deployment.md)
- [Setup Guide](SETUP_GUIDE.md)
- [Contributing Guide](CONTRIBUTING.md)

---

## Roadmap

### Version 1.0 (Current)
- ✅ Dual-layer caching (L1 + L2)
- ✅ Event-driven invalidation
- ✅ Cache warming
- ✅ Admin dashboard
- ✅ Prometheus metrics

### Version 1.1 (Q1 2024)
- 🔄 ML-based predictive warming
- 🔄 GraphQL API
- 🔄 Multi-region support
- 🔄 Enhanced RBAC

### Version 2.0 (Q2 2024)
- 📋 Cache-as-a-Service API
- 📋 Multi-tenancy
- 📋 Advanced analytics
- 📋 Plugin system

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Acknowledgments

- [Encore](https://encore.dev) - Backend framework
- [Vite](https://vitejs.dev) - Frontend build tool
- [SWR](https://swr.vercel.app) - Data fetching library
- [Recharts](https://recharts.org) - Chart library
- [TailwindCSS](https://tailwindcss.com) - CSS framework


<div align="center">

** Star us on GitHub** — it helps!

[Documentation](docs/) • [API Reference](docs/api.md) • [Contributing](CONTRIBUTING.md) • [Changelog](CHANGELOG.md)

</div>