# Cache Manager Service

High-performance distributed cache manager with multi-level storage (L1 in-memory, L2 distributed), intelligent eviction policies, and event-driven coordination.

## 🏗️ Architecture
```
┌─────────────────────────────────────────┐
│         Cache Manager Service           │
├─────────────────────────────────────────┤
│                                         │
│  ┌─────────────┐      ┌──────────────┐ │
│  │  L1 Cache   │──►   │  L2 Cache    │ │
│  │  (Memory)   │      │  (Redis)     │ │
│  └─────────────┘      └──────────────┘ │
│         │                     │         │
│         │      ┌──────────────┘         │
│         ▼      ▼                        │
│  ┌──────────────────┐                  │
│  │  Request         │                  │
│  │  Coalescer       │                  │
│  │  (Singleflight)  │                  │
│  └──────────────────┘                  │
│                                         │
│  Pub/Sub: Invalidate, Refresh Events   │
└─────────────────────────────────────────┘
```

## ✨ Features

- **Multi-Level Caching**: L1 (in-memory) + L2 (distributed Redis)
- **LRU + TTL Eviction**: Combined eviction policies for optimal memory usage
- **Cache Stampede Prevention**: Request coalescing via singleflight pattern
- **Distributed Coordination**: Pub/Sub for cross-instance invalidation
- **Read-Through/Write-Through**: Automatic cache population from origin
- **Pattern Invalidation**: Wildcard support (e.g., `user:*`)
- **Real-Time Metrics**: Hit/miss rates, latency, eviction stats

## 🚀 Quick Start

### Prerequisites
```bash
# Install Encore CLI
curl -L https://encore.dev/install.sh | bash

# Verify installation
encore version
```

### Running Locally
```bash
# Clone repository
cd distributed-cache-system/cache-manager

# Run service
encore run

# Service will be available at:
# http://localhost:4000
```

### Running Tests
```bash
# Run all tests
go test ./... -v

# Run tests with race detector
go test ./... -race -v

# Run benchmarks
go test -bench=. -benchmem

# Run specific test
go test -run TestL1Cache_BasicOperations -v
```

## 🔧 Configuration

### Environment Variables
```bash
# L1 Cache Configuration
export CACHE_L1_MAX_ENTRIES=10000        # Max L1 entries (default: 10000)
export CACHE_DEFAULT_TTL=3600            # Default TTL in seconds (default: 3600)
export CACHE_CLEANUP_INTERVAL=60         # Cleanup interval in seconds (default: 60)

# L2 Cache Configuration (Redis)
export REDIS_URL="redis://localhost:6379"
export REDIS_PASSWORD=""
export REDIS_DB=0

# Monitoring
export METRICS_ENABLED=true
export METRICS_INTERVAL=10               # Metrics collection interval (seconds)
```

### Programmatic Configuration
```go
config := Config{
    L1MaxEntries:    10000,
    DefaultTTL:      1 * time.Hour,
    CleanupInterval: 1 * time.Minute,
    L2Enabled:       true,
}
```

## 📡 API Endpoints

### Get Cache Entry
```bash
# Get value from cache
curl http://localhost:4000/api/cache/user:123

# Response
{
  "value": {"id": 123, "name": "John"},
  "hit": true,
  "source": "l1",
  "cached_at": "2024-01-15T10:30:00Z",
  "expires_at": "2024-01-15T11:30:00Z"
}
```

### Set Cache Entry
```bash
# Set value with custom TTL
curl -X PUT http://localhost:4000/api/cache/user:123 \
  -H "Content-Type: application/json" \
  -d '{
    "key": "user:123",
    "value": {"id": 123, "name": "John"},
    "ttl": 3600
  }'

# Response
{
  "success": true,
  "expires_at": "2024-01-15T11:30:00Z"
}
```

### Invalidate Cache
```bash
# Invalidate specific keys
curl -X POST http://localhost:4000/api/cache/invalidate \
  -H "Content-Type: application/json" \
  -d '{
    "keys": ["user:123", "user:456"]
  }'

# Invalidate by pattern
curl -X POST http://localhost:4000/api/cache/invalidate \
  -H "Content-Type: application/json" \
  -d '{
    "pattern": "user:*"
  }'

# Response
{
  "invalidated": 2,
  "success": true
}
```

### Get Metrics
```bash
# Get cache performance metrics
curl http://localhost:4000/api/cache/metrics

# Response
{
  "hits": 8542,
  "misses": 1234,
  "hit_rate": 0.873,
  "sets": 2345,
  "deletes": 123,
  "evictions": 456,
  "l1_size": 7890,
  "l2_hits": 890,
  "l2_misses": 344,
  "l2_errors": 2
}
```

## 🔌 Integration Guide

### Using with Origin Fetcher
```go
// Implement origin fetcher interface
type UserService struct {
    db *sql.DB
}

func (s *UserService) Fetch(ctx context.Context, key string) (interface{}, error) {
    // Extract ID from key (e.g., "user:123" -> "123")
    userID := extractID(key)
    
    // Fetch from database
    var user User
    err := s.db.QueryRowContext(ctx, 
        "SELECT id, name, email FROM users WHERE id = $1", 
        userID,
    ).Scan(&user.ID, &user.Name, &user.Email)
    
    if err != nil {
        return nil, err
    }
    
    return user, nil
}

// Wire up to cache manager
svc.SetOriginFetcher(&UserService{db: database})
```

### Using with L2 Cache (Redis)
```go
import "github.com/go-redis/redis/v8"

// Implement RemoteCache interface for Redis
type RedisCache struct {
    client *redis.Client
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
    val, err := r.client.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, false, nil
    }
    if err != nil {
        return nil, false, err
    }
    return val, true, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
    return r.client.Del(ctx, key).Err()
}

func (r *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
    // Use Redis SCAN + DEL for pattern matching
    iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()
    for iter.Next(ctx) {
        r.client.Del(ctx, iter.Val())
    }
    return iter.Err()
}

// Wire up to cache manager
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})
svc.SetL2Cache(&RedisCache{client: redisClient})
```

## 📊 Performance Characteristics

### Operation Complexity

| Operation | L1 Hit | L1 Miss + L2 Hit | L1/L2 Miss + Origin |
|-----------|--------|------------------|---------------------|
| Get       | O(1) ~1μs | O(1) ~1-5ms | O(1) + origin latency |
| Set       | O(1) ~2μs | O(1) ~2-10ms | N/A |
| Delete    | O(1) ~1μs | O(1) ~1-5ms | N/A |
| Pattern   | O(n) | O(n) + network | N/A |

### Throughput Benchmarks
```
BenchmarkL1Cache_Get-8              50000000    25.3 ns/op
BenchmarkL1Cache_Set-8              10000000    142 ns/op
BenchmarkL1Cache_ConcurrentGet-8    30000000    41.2 ns/op
BenchmarkRequestCoalescer-8         20000000    67.8 ns/op
```

## 🐛 Troubleshooting

### High Memory Usage
```bash
# Check L1 cache size
curl http://localhost:4000/api/cache/metrics | jq '.l1_size'

# Reduce L1 max entries
export CACHE_L1_MAX_ENTRIES=5000

# Reduce default TTL
export CACHE_DEFAULT_TTL=1800
```

### Low Hit Rate
```bash
# Check hit rate
curl http://localhost:4000/api/cache/metrics | jq '.hit_rate'

# Increase TTL for stable data
# Implement warming service to pre-populate cache
# Review access patterns for optimization opportunities
```

### L2 Connection Errors
```bash
# Check L2 errors
curl http://localhost:4000/api/cache/metrics | jq '.l2_errors'

# Verify Redis connectivity
redis-cli ping

# Check Redis logs
redis-cli --latency
```

## 🔐 Security Considerations

- **Key Validation**: All keys are validated to prevent injection attacks
- **Value Size Limits**: Implement max value size to prevent memory exhaustion
- **Rate Limiting**: Apply rate limiting at API gateway level
- **Authentication**: Use Encore's built-in auth for production deployments

## 📈 Production Optimization Tips

1. **Shard L1 Cache**: For >1M keys, shard L1 across multiple sync.RWMutex instances
2. **Redis Pipelining**: Batch L2 operations to reduce network RTT by 5-10x
3. **Compression**: Enable compression for values >1KB to save memory/bandwidth
4. **Adaptive TTL**: Implement dynamic TTL based on access frequency
5. **Circuit Breaker**: Add circuit breaker for L2 to prevent cascading failures
6. **Monitoring**: Set up alerts for hit rate <70%, P95 latency >100ms

## 📚 Additional Resources

- [Encore Documentation](https://encore.dev/docs)
- [Cache Patterns Guide](https://encore.dev/docs/tutorials/caching)
- [Performance Tuning](https://encore.dev/docs/deploy/performance)

## 🤝 Contributing

See main project README for contribution guidelines.

## 📝 License

See main project LICENSE file.