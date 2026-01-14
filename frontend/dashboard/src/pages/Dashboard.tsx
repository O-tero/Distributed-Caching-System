import { useState, useEffect, useMemo } from 'react'
import { 
  Activity, 
  Zap, 
  Database, 
  HardDrive, 
  Clock, 
  Trash2 
} from 'lucide-react'
import { 
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../components/ui/select'
import { MetricCard } from '../components/dashboard/MetricCard'
import { MetricsChart } from '../components/dashboard/MetricsChart'
import { RecentActivity } from '../components/dashboard/RecentActivity'
import { 
  generateMetrics, 
  generateTimeSeriesData, 
  generateEvents,
  type CacheMetrics,
  type CacheEvent
} from '../lib/mock-data'
import { formatNumber, formatBytes, formatPercentage, formatDuration } from '../lib/utils'
type TimeWindow = '1m' | '5m' | '1h' | '24h'

const timeWindowOptions: { value: TimeWindow; label: string; points: number }[] = [
  { value: '1m', label: 'Last 1 minute', points: 12 },
  { value: '5m', label: 'Last 5 minutes', points: 30 },
  { value: '1h', label: 'Last 1 hour', points: 60 },
  { value: '24h', label: 'Last 24 hours', points: 96 },
]

export default function Dashboard() {
  const [timeWindow, setTimeWindow] = useState<TimeWindow>('5m')
  const [metrics, setMetrics] = useState<CacheMetrics | null>(null)
  const [prevMetrics, setPrevMetrics] = useState<CacheMetrics | null>(null)
  const [events, setEvents] = useState<CacheEvent[]>([])
  const [loading, setLoading] = useState(true)
  
  const selectedTimeOption = timeWindowOptions.find(t => t.value === timeWindow)!
  
  const chartData = useMemo(() => {
    return generateTimeSeriesData(selectedTimeOption.points)
  }, [selectedTimeOption.points, metrics?.timestamp])
  
  // Initial load
  useEffect(() => {
    const loadData = () => {
      setMetrics(prev => {
        setPrevMetrics(prev)
        return generateMetrics()
      })
      setEvents(generateEvents(10))
      setLoading(false)
    }
    
    loadData()
    
    // Poll every 5 seconds
    const interval = setInterval(loadData, 5000)
    return () => clearInterval(interval)
  }, [])
  
  const getHitRateTrend = () => {
    if (!metrics || !prevMetrics) return undefined
    const diff = metrics.hitRate - prevMetrics.hitRate
    if (Math.abs(diff) < 0.001) return undefined
    return {
      direction: diff > 0 ? 'up' as const : 'down' as const,
      value: `${Math.abs(diff * 100).toFixed(1)}%`,
      isPositive: diff > 0,
    }
  }
  
  const getMissRateTrend = () => {
    if (!metrics || !prevMetrics) return undefined
    const diff = metrics.missRate - prevMetrics.missRate
    if (Math.abs(diff) < 0.001) return undefined
    return {
      direction: diff > 0 ? 'up' as const : 'down' as const,
      value: `${Math.abs(diff * 100).toFixed(1)}%`,
      isPositive: diff < 0, // Lower miss rate is better
    }
  }
  
  const getHitRateColor = () => {
    if (!metrics) return 'default' as const
    if (metrics.hitRate >= 0.8) return 'success' as const
    if (metrics.hitRate >= 0.6) return 'warning' as const
    return 'destructive' as const
  }
  
  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Dashboard</h1>
          <p className="text-sm text-muted-foreground">
            Real-time cache performance metrics
          </p>
        </div>
        
        <Select value={timeWindow} onValueChange={(v) => setTimeWindow(v as TimeWindow)}>
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="Select time window" />
          </SelectTrigger>
          <SelectContent>
            {timeWindowOptions.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      
      {/* Metrics grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 stagger-children">
        <MetricCard
          title="Hit Rate"
          value={metrics ? formatPercentage(metrics.hitRate) : '—'}
          subtitle="Cache hit percentage"
          icon={Activity}
          loading={loading}
          trend={getHitRateTrend()}
          color={getHitRateColor()}
        />
        
        <MetricCard
          title="Miss Rate"
          value={metrics ? formatPercentage(metrics.missRate) : '—'}
          subtitle="Cache miss percentage"
          icon={Zap}
          loading={loading}
          trend={getMissRateTrend()}
          color={metrics && metrics.missRate > 0.2 ? 'warning' : 'default'}
        />
        
        <MetricCard
          title="L1 Cache Size"
          value={metrics ? formatNumber(metrics.l1Size) : '—'}
          subtitle="In-memory entries"
          icon={Database}
          loading={loading}
          color="primary"
        />
        
        <MetricCard
          title="L2 Cache Size"
          value={metrics ? formatBytes(metrics.l2Size) : '—'}
          subtitle="Distributed cache"
          icon={HardDrive}
          loading={loading}
          color="primary"
        />
        
        <MetricCard
          title="P95 Latency"
          value={metrics ? formatDuration(metrics.l1Latency.p95) : '—'}
          subtitle="L1 response time"
          icon={Clock}
          loading={loading}
          color={metrics && metrics.l1Latency.p95 > 10 ? 'warning' : 'default'}
        />
        
        <MetricCard
          title="Invalidation Rate"
          value={metrics ? `${formatNumber(metrics.invalidationRate)}/s` : '—'}
          subtitle="Keys invalidated per second"
          icon={Trash2}
          loading={loading}
        />
      </div>
      
      {/* Charts and activity */}
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <MetricsChart data={chartData} loading={loading} />
        </div>
        
        <div className="lg:col-span-1">
          <RecentActivity events={events} loading={loading} />
        </div>
      </div>
    </div>
  )
}
