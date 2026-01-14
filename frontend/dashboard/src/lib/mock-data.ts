// Mock data for the cache dashboard

export interface CacheMetrics {
  hitRate: number
  missRate: number
  l1Size: number
  l2Size: number
  l1Latency: { p50: number; p95: number }
  l2Latency: { p50: number; p95: number }
  invalidationRate: number
  evictionRate: number
  timestamp: string
}

export interface CacheKey {
  key: string
  size: number
  ttl: number
  lastAccess: string
  accessCount: number
  createdAt: string
}

export interface CacheEvent {
  id: string
  type: 'invalidation' | 'warning' | 'error' | 'info'
  message: string
  service: string
  timestamp: string
  keysAffected?: number
}

export interface WarmingJob {
  id: string
  name: string
  pattern: string
  cron: string
  enabled: boolean
  lastRun?: {
    status: 'success' | 'failed' | 'partial'
    keysWarmed: number
    keysFailed: number
    duration: number
    timestamp: string
  }
}

export interface InvalidationRecord {
  id: string
  pattern: string
  keysInvalidated: number
  timestamp: string
  isDryRun: boolean
}

// Generate time series data
export function generateTimeSeriesData(points: number = 60): Array<{
  time: string
  hitRate: number
  missRate: number
  p95Latency: number
}> {
  const data = []
  const now = Date.now()
  
  for (let i = points - 1; i >= 0; i--) {
    const timestamp = new Date(now - i * 60000)
    const baseHitRate = 0.85 + Math.random() * 0.1
    const hitRate = Math.min(0.99, Math.max(0.7, baseHitRate + (Math.sin(i / 10) * 0.05)))
    
    data.push({
      time: timestamp.toISOString(),
      hitRate: parseFloat(hitRate.toFixed(3)),
      missRate: parseFloat((1 - hitRate).toFixed(3)),
      p95Latency: parseFloat((3 + Math.random() * 4 + (Math.sin(i / 8) * 2)).toFixed(1)),
    })
  }
  
  return data
}

// Generate current metrics
export function generateMetrics(): CacheMetrics {
  const hitRate = 0.85 + Math.random() * 0.1
  return {
    hitRate,
    missRate: 1 - hitRate,
    l1Size: Math.floor(8000 + Math.random() * 4000),
    l2Size: Math.floor(40000000 + Math.random() * 20000000),
    l1Latency: {
      p50: parseFloat((0.8 + Math.random() * 0.5).toFixed(2)),
      p95: parseFloat((3 + Math.random() * 3).toFixed(2)),
    },
    l2Latency: {
      p50: parseFloat((10 + Math.random() * 8).toFixed(1)),
      p95: parseFloat((40 + Math.random() * 20).toFixed(1)),
    },
    invalidationRate: Math.floor(100 + Math.random() * 100),
    evictionRate: Math.floor(30 + Math.random() * 40),
    timestamp: new Date().toISOString(),
  }
}

// Generate cache keys
export function generateCacheKeys(count: number = 50): CacheKey[] {
  const prefixes = ['users', 'products', 'sessions', 'orders', 'inventory', 'config']
  const keys: CacheKey[] = []
  
  for (let i = 0; i < count; i++) {
    const prefix = prefixes[Math.floor(Math.random() * prefixes.length)]
    const id = Math.floor(Math.random() * 10000)
    const suffix = ['profile', 'details', 'metadata', 'cache', 'data'][Math.floor(Math.random() * 5)]
    
    keys.push({
      key: `${prefix}:${id}:${suffix}`,
      size: Math.floor(256 + Math.random() * 4096),
      ttl: Math.floor(1800 + Math.random() * 5400),
      lastAccess: new Date(Date.now() - Math.random() * 3600000).toISOString(),
      accessCount: Math.floor(1 + Math.random() * 200),
      createdAt: new Date(Date.now() - Math.random() * 86400000).toISOString(),
    })
  }
  
  return keys.sort((a, b) => b.accessCount - a.accessCount)
}

// Generate recent events
export function generateEvents(count: number = 10): CacheEvent[] {
  const eventTypes: Array<{ type: CacheEvent['type']; messages: string[] }> = [
    {
      type: 'invalidation',
      messages: [
        'Bulk invalidation completed',
        'Pattern-based cache clear',
        'Scheduled cache refresh',
      ],
    },
    {
      type: 'warning',
      messages: [
        'High miss rate detected',
        'Memory threshold approaching',
        'Slow cache response times',
      ],
    },
    {
      type: 'error',
      messages: [
        'Cache connection timeout',
        'Failed to warm cache',
        'Invalidation request failed',
      ],
    },
    {
      type: 'info',
      messages: [
        'Cache warming completed',
        'Configuration updated',
        'New node joined cluster',
      ],
    },
  ]
  
  const services = ['api-gateway', 'user-service', 'product-service', 'order-service', 'auth-service']
  const events: CacheEvent[] = []
  
  for (let i = 0; i < count; i++) {
    const eventType = eventTypes[Math.floor(Math.random() * eventTypes.length)]
    events.push({
      id: `evt_${Math.random().toString(36).substring(2, 9)}`,
      type: eventType.type,
      message: eventType.messages[Math.floor(Math.random() * eventType.messages.length)],
      service: services[Math.floor(Math.random() * services.length)],
      timestamp: new Date(Date.now() - Math.random() * 3600000).toISOString(),
      keysAffected: eventType.type === 'invalidation' ? Math.floor(10 + Math.random() * 1000) : undefined,
    })
  }
  
  return events.sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
}

// Generate warming jobs
export function generateWarmingJobs(): WarmingJob[] {
  return [
    {
      id: 'job_1',
      name: 'User Profiles Warm',
      pattern: 'users:*:profile',
      cron: '0 */6 * * *',
      enabled: true,
      lastRun: {
        status: 'success',
        keysWarmed: 2543,
        keysFailed: 0,
        duration: 45200,
        timestamp: new Date(Date.now() - 3600000).toISOString(),
      },
    },
    {
      id: 'job_2',
      name: 'Product Catalog',
      pattern: 'products:*',
      cron: '0 2 * * *',
      enabled: true,
      lastRun: {
        status: 'partial',
        keysWarmed: 8921,
        keysFailed: 234,
        duration: 120500,
        timestamp: new Date(Date.now() - 7200000).toISOString(),
      },
    },
    {
      id: 'job_3',
      name: 'Config Cache',
      pattern: 'config:*',
      cron: '*/30 * * * *',
      enabled: true,
      lastRun: {
        status: 'success',
        keysWarmed: 156,
        keysFailed: 0,
        duration: 2100,
        timestamp: new Date(Date.now() - 1800000).toISOString(),
      },
    },
    {
      id: 'job_4',
      name: 'Session Preload',
      pattern: 'sessions:active:*',
      cron: '0 0 * * *',
      enabled: false,
      lastRun: {
        status: 'failed',
        keysWarmed: 0,
        keysFailed: 1523,
        duration: 5000,
        timestamp: new Date(Date.now() - 86400000).toISOString(),
      },
    },
  ]
}

// Generate invalidation records
export function generateInvalidationRecords(): InvalidationRecord[] {
  return [
    {
      id: 'inv_1',
      pattern: 'users:*:session',
      keysInvalidated: 1847,
      timestamp: new Date(Date.now() - 300000).toISOString(),
      isDryRun: false,
    },
    {
      id: 'inv_2',
      pattern: 'products:*:price',
      keysInvalidated: 523,
      timestamp: new Date(Date.now() - 1800000).toISOString(),
      isDryRun: false,
    },
    {
      id: 'inv_3',
      pattern: 'orders:pending:*',
      keysInvalidated: 89,
      timestamp: new Date(Date.now() - 3600000).toISOString(),
      isDryRun: true,
    },
    {
      id: 'inv_4',
      pattern: 'inventory:*',
      keysInvalidated: 2156,
      timestamp: new Date(Date.now() - 7200000).toISOString(),
      isDryRun: false,
    },
  ]
}

// Simulate matching keys for a pattern
export function matchKeysForPattern(pattern: string, allKeys: CacheKey[]): CacheKey[] {
  const regex = new RegExp('^' + pattern.replace(/\*/g, '.*') + '$')
  return allKeys.filter(key => regex.test(key.key))
}
