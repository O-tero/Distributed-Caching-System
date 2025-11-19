# Warming Service

Proactive cache warming service that prevents cold misses and cache stampedes by intelligently pre-populating cache entries before they're accessed.

## 🏗️ Architecture
```
┌─────────────────────────────────────────────────────┐
│              Warming Service                        │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────┐ │
│  │  Strategies  │  │  Predictor   │  │  Cron    │ │
│  │              │  │              │  │  Jobs    │ │
│  │  • Selective │  │  • Heuristic │  │          │ │
│  │  • Breadth   │  │  • ML-ready  │  │  • Daily │ │
│  │  • Priority  │  │              │  │  • Hourly│ │
│  └──────┬───────┘  └──────┬───────┘  └────┬─────┘ │
│         │                 │                │       │
│         └─────────┬───────┴────────────────┘       │
│                   ▼                                 │
│         ┌─────────────────────┐                    │
│         │   Worker Pool       │                    │
│         │   (Concurrent       │                    │
│         │    Warming)         │                    │
│         └─────────┬───────────┘                    │
│                   │                                 │
│                   ▼                                 │
│         ┌─────────────────────┐                    │
│         │  Origin Fetcher     │                    │
│         │  + Rate Limiter     │                    │
│         └─────────┬───────────┘                    │
│                   │                                 │
│                   ▼                                 │
│         ┌─────────────────────┐                    │
│         │  Cache Manager      │                    │
│         │  (Write Cache)      │                    │
│         └─────────────────────┘                    │
└─────────────────────────────────────────────────────┘
```

## ✨ Features

- **Multiple Warming Strategies**: Selective, breadth-first, priority-based
- **Predictive Warming**: ML-ready predictor interface with heuristic implementation
- **Scheduled Warming**: Cron jobs for daily, hourly, and peak-hour warming
- **Rate Limiting**: Protects origin services from overload
- **Worker Pool**: Concurrent warming with configurable concurrency
- **Deduplication**: Prevents redundant warming of same keys
- **Retry Logic**: Exponential backoff for failed warming attempts
- **Emergency Stop**: Automatic throttling on high origin latency
- **Observable**: Real-time metrics and status endpoints

## 🚀 Quick Start

### Running Locally
```bash
# Start the service
cd distributed-cache-system
encore run

# Service endpoints available at:
# http://localhost:4000
```

### Configuration

Set environment variables or use runtime config API:
```bash
# Core Configuration
export WARMING_MAX_ORIGIN_RPS=100          # Max requests/sec to origin
export WARMING_CONCURRENT_WARMERS=10       # Number of worker goroutines
export WARMING_MAX_BATCH_SIZE=50           # Max keys per batch
export WARMING_DEFAULT_TTL=3600            # Default cache TTL (seconds)

# Performance Tuning
export WARMING_ORIGIN_TIMEOUT=5            # Origin fetch timeout (seconds)
export WARMING_RETRY_ATTEMPTS=3            # Number of retry attempts
export WARMING_BACKOFF_BASE=100            # Base backoff duration (ms)
export WARMING_EMERGENCY_THRESHOLD=2000    # Emergency stop threshold (ms)

# Strategy Selection
export WARMING_DEFAULT_STRATEGY=priority   # Default: selective, breadth, priority
```

## 📡 API Endpoints

### 1. Warm Specific Keys

Warm exact cache keys immediately.
```bash
curl -X POST http://localhost:4000/warm/key \
  -H "Content-Type: application/json" \
  -d '{
    "keys": ["user:123:profile", "user:123:settings"],
    "priority": 80,
    "strategy": "priority"
  }'
```

**Response:**
```json
{
  "success": true,
  "queued": 2,
  "keys": ["user:123:profile", "user:123:settings"],
  "job_id": "warm-1736938200000-123",
  "estimated_time_ms": 100
}
```

### 2. Warm by Pattern

Warm keys matching a wildcard pattern.
```bash
curl -X POST http://localhost:4000/warm/pattern \
  -H "Content-Type: application/json" \
  -d '{
    "pattern": "user:123:*",
    "limit": 50,
    "priority": 70,
    "strategy": "breadth"
  }'
```

**Response:**
```json
{
  "success": true,
  "pattern": "user:123:*",
  "queued": 15,
  "matched_keys": ["user:123:profile", "user:123:settings", ...],
  "job_id": "warm-1736938200001-456",
  "estimated_time_ms": 300
}
```

### 3. Trigger Predictive Warming

Manually trigger ML/heuristic-based prediction and warming.
```bash
curl -X POST http://localhost:4000/warm/trigger-predictive
```

**Response:**
```json
{
  "success": true,
  "queued": 87,
  "keys": ["hot:key1", "hot:key2", ...],
  "job_id": "warm-1736938200002-789",
  "estimated_time_ms": 870
}
```

### 4. Get Status

Retrieve current warming service status and metrics.
```bash
curl http://localhost:4000/warm/status
```

**Response:**
```json
{
  "active_jobs": 3,
  "queued_tasks": 45,
  "worker_status": [
    {
      "id": 0,
      "state": "busy",
      "current_key": "user:456:profile",
      "started_at": "2025-01-15T10:30:00Z"
    },
    ...
  ],
  "emergency_stop": false,
  "metrics": {
    "jobs_total": 1523,
    "success_total": 1487,
    "failure_total": 36,
    "success_rate": 0.976,
    "origin_requests": 1487,
    "cache_writes": 1487,
    "rate_limit_hits": 12,
    "emergency_stops": 0,
    "avg_duration_ms": 47.3
  }
}
```

### 5. Get Configuration

Retrieve current service configuration.
```bash
curl http://localhost:4000/warm/config
```

**Response:**
```json
{
  "config": {
    "max_origin_rps": 100,
    "max_batch_size": 50,
    "concurrent_warmers": 10,
    "default_ttl": 3600000000000,
    "origin_timeout": 5000000000,
    "retry_attempts": 3,
    "backoff_base": 100000000,
    "emergency_threshold": 2000000000,
    "default_strategy": "priority"
  }
}
```

### 6. Update Configuration

Update service configuration at runtime.
```bash
curl -X POST http://localhost:4000/warm/config \
  -H "Content-Type: application/json" \
  -d '{
    "max_origin_rps": 200,
    "max_batch_size": 100,
    "default_strategy": "selective"
  }'
```

## 🎯 Warming Strategies

### Selective Hot Keys

Warms only the hottest keys based on access frequency.

**Best for:**
- High-traffic scenarios
- Pareto-distributed access patterns (80-20 rule)
- Limited warming budget

**Algorithm:**
- Takes top N most frequently accessed keys
- Priority decreases linearly for less hot keys
```bash
curl -X POST http://localhost:4000/warm/pattern \
  -d '{"pattern":"*","limit":100,"strategy":"selective"}'
```

### Breadth-First

Warms keys based on hierarchical dependencies.

**Best for:**
- Related data warming (user → posts → comments)
- Preventing cascading cache misses
- Hierarchical key structures

**Algorithm:**
- Sorts keys by depth (fewer colons = higher priority)
- Warms parent keys before children
```bash
curl -X POST http://localhost:4000/warm/pattern \
  -d '{"pattern":"user:123:*","strategy":"breadth"}'
```

### Priority-Based

Warms keys based on calculated priority score.

**Best for:**
- Balancing multiple factors (importance, cost, hotness)
- Cost-aware warming
- General-purpose warming

**Algorithm:**
- Score = (importance * hotness) / cost
- Highest scores warmed first
```bash
curl -X POST http://localhost:4000/warm/key \
  -d '{"keys":["key1","key2"],"strategy":"priority"}'
```

## 🔮 Predictive Warming

### Default Predictor (Heuristic)

Uses access patterns to predict future hot keys.

**Features:**
- Tracks access frequency and recency
- Calculates growth rates
- Applies recency bonus

**Algorithm:**
```
score = frequency * (1 + growth_rate) * recency_bonus

where:
  frequency = accesses_per_hour
  growth_rate = (recent_freq - historical_freq) / historical_freq
  recency_bonus = 2.0 (if last_access < 5min)
                = 1.5 (if last_access < 30min)
                = 1.0 (otherwise)
```

### ML Predictor (TODO)

Placeholder for ML-based prediction.

**Integration Steps:**
1. Train model offline using historical access logs
2. Features: time of day, day of week, trends, key metadata
3. Model: LSTM for time series or gradient boosting
4. Load model at startup
5. Run inference in `PredictHotKeys()` method
```go
// Example ML integration
type MLPredictor struct {
    model *tensorflow.SavedModel
}

func (p *MLPredictor) PredictHotKeys(ctx context.Context, window time.Duration, limit int) ([]string, error) {
    features := extractFeatures(ctx, window)
    predictions := p.model.Predict(features)
    return topKKeys(predictions, limit), nil
}
```

## ⏰ Scheduled Warming (Cron Jobs)

### Pre-configured Schedules

**Daily Warmup** (`0 2 * * *`)
- Runs at 2 AM daily
- Warms critical cache keys
- Uses predictive warming

**Hourly Refresh** (`0 * * * *`)
- Runs every hour
- Refreshes frequently accessed keys
- Top 50 predicted keys

**Peak Hours Warmup** (`0 7,11,17 * * *`)
- Runs 1 hour before peak times (8 AM, 12 PM, 6 PM)
- Aggressive warming (top 100 keys)
- High priority

### Custom Scheduling

Register custom warming jobs via API (TODO: implement job registration endpoint).

## 🔌 Integration Examples

### From Cache Manager

Warm cache after data updates:
```go
import "github.com/yourusername/distributed-cache-system/warming"

// After updating user profile
_, err := warming.WarmKey(ctx, &warming.WarmKeyRequest{
    Keys:     []string{fmt.Sprintf("user:%d:profile", userID)},
    Priority: 90,
})
```

### From Application Code

Proactive warming before high-load events:
```go
// Before launching new feature
_, err := warming.WarmPattern(ctx, &warming.WarmPatternRequest{
    Pattern:  "new-feature:*",
    Limit:    1000,
    Priority: 100,
    Strategy: "priority",
})
```

### Subscribing to Warming Events

Listen for warming completion:
```go
var _ = pubsub.NewSubscription(
    warming.WarmCompletedTopic,
    "my-service-warm-completed",
    pubsub.SubscriptionConfig[*warming.WarmCompletedEvent]{
        Handler: HandleWarmCompleted,
    },
)

func HandleWarmCompleted(ctx context.Context, event *warming.WarmCompletedEvent) error {
    log.Printf("Warmed %s in %dms (status: %s)", 
        event.Key, event.DurationMs, event.Status)
    return nil
}
```

## 📊 Monitoring & Metrics

### Key Metrics
```
# Warming operations
warming_jobs_total{strategy="selective|breadth|priority"}
warming_success_total
warming_failure_total
warming_duration_seconds{quantile="0.5|0.95|0.99"}

# Origin protection
warming_origin_requests_total
warming_rate_limit_hits_total
warming_emergency_stops_total

# Cache operations
warming_cache_writes_total
```

### Alerting Recommendations
```yaml
# High failure rate
- alert: WarmingHighFailureRate
  expr: rate(warming_failure_total[5m]) / rate(warming_jobs_total[5m]) > 0.1
  severity: warning

# Emergency stop triggered
- alert: WarmingEmergencyStop
  expr: warming_emergency_stops_total > 0
  severity: critical

# Low success rate
- alert: WarmingLowSuccessRate
  expr: warming_success_total / warming_jobs_total < 0.9
severity: warning

# Rate limit saturation
- alert: WarmingRateLimitSaturated
  expr: rate(warming_rate_limit_hits_total[5m]) > 10
  severity: info