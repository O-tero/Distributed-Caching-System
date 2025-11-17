# Invalidation Service

Distributed cache invalidation service that coordinates cache invalidation across multiple cache-manager instances using event-driven architecture.

## 🏗️ Architecture
```
┌─────────────────────────────────────────────────────┐
│           Invalidation Service                      │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌──────────────────┐    ┌──────────────────┐     │
│  │  Pattern Matcher │    │  Audit Logger    │     │
│  │                  │    │  (PostgreSQL)    │     │
│  │  • Exact         │    │                  │     │
│  │  • Prefix (*)    │    │  • Immutable Log │     │
│  │  • Suffix (*)    │    │  • Compliance    │     │
│  │  • Regex         │    │  • Tracing       │     │
│  └──────────────────┘    └──────────────────┘     │
│           │                        │               │
│           └────────┬───────────────┘               │
│                    ▼                                │
│         ┌─────────────────────┐                    │
│         │  Pub/Sub Publisher  │                    │
│         │  (cache-invalidate) │                    │
│         └─────────────────────┘                    │
└─────────────────────────────────────────────────────┘
                      │
                      │ Broadcast
                      ▼
      ┌───────────────────────────────────┐
      │   All Cache Manager Instances     │
      │   (Receive InvalidationEvent)     │
      └───────────────────────────────────┘
```

## ✨ Features

- **Multi-Pattern Invalidation**: Exact keys, prefix wildcards, regex patterns
- **Distributed Coordination**: Pub/Sub broadcast ensures all cache nodes are synchronized
- **Audit Trail**: Immutable PostgreSQL log for compliance and debugging
- **Performance Optimized**: Regex caching, O(1) prefix matching, sub-millisecond latency
- **Observability**: Real-time metrics on invalidation patterns and performance
- **Idempotent**: Duplicate invalidations are safely handled

## 🚀 Quick Start

### Running Locally
```bash
# Start the service
cd distributed-cache-system
encore run

# Service endpoints available at:
# http://localhost:4000
```

### Database Setup

The service automatically creates the required PostgreSQL schema on startup:
```sql
CREATE TABLE invalidation_audit (
    id BIGSERIAL PRIMARY KEY,
    pattern TEXT NOT NULL,
    keys JSONB,
    triggered_by TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    request_id TEXT NOT NULL,
    latency_ms BIGINT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

## 📡 API Endpoints

### 1. Invalidate by Key

Invalidate specific cache keys.
```bash
curl -X POST http://localhost:4000/invalidate/key \
  -H "Content-Type: application/json" \
  -d '{
    "keys": ["user:123", "user:456"],
    "triggered_by": "admin",
    "request_id": "req-001"
  }'
```

**Response:**
```json
{
  "success": true,
  "invalidated_count": 2,
  "keys": ["user:123", "user:456"],
  "request_id": "req-001",
  "published_at": "2025-01-15T10:30:00Z"
}
```

### 2. Invalidate by Pattern

Invalidate keys matching a wildcard pattern.
```bash
curl -X POST http://localhost:4000/invalidate/pattern \
  -H "Content-Type: application/json" \
  -d '{
    "pattern": "user:123:*",
    "triggered_by": "cache_manager",
    "cache_keys": [
      "user:123:profile",
      "user:123:settings",
      "user:456:profile"
    ]
  }'
```

**Response:**
```json
{
  "success": true,
  "pattern": "user:123:*",
  "matched_keys": ["user:123:profile", "user:123:settings"],
  "invalidated_count": 2,
  "request_id": "inv-1736938200000-123",
  "published_at": "2025-01-15T10:30:00Z"
}
```

### 3. Get Audit Logs

Retrieve invalidation history with pagination.
```bash
curl "http://localhost:4000/audit/logs?limit=50&offset=0"
```

**Response:**
```json
{
  "logs": [
    {
      "id": 123,
      "pattern": "user:123:*",
      "keys": ["user:123:profile", "user:123:settings"],
      "triggered_by": "cache_manager",
      "timestamp": "2025-01-15T10:30:00Z",
      "request_id": "req-001",
      "latency": 5
    }
  ],
  "total_count": 1250,
  "has_more": true
}
```

### 4. Get Metrics

Retrieve invalidation service metrics.
```bash
curl http://localhost:4000/invalidate/metrics