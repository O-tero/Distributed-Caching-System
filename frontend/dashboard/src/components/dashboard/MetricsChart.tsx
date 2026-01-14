import { useMemo, useState } from 'react'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn, formatPercentage } from '@/lib/utils'

interface ChartDataPoint {
  time: string
  hitRate: number
  missRate: number
  p95Latency: number
}

interface MetricsChartProps {
  data: ChartDataPoint[]
  loading?: boolean
}

const chartColors = {
  hitRate: 'hsl(160, 84%, 39%)',
  missRate: 'hsl(38, 92%, 50%)',
  p95Latency: 'hsl(199, 89%, 48%)',
}

export function MetricsChart({ data, loading = false }: MetricsChartProps) {
  const [hiddenSeries, setHiddenSeries] = useState<Set<string>>(new Set())
  
  const formattedData = useMemo(() => {
    return data.map(d => ({
      ...d,
      time: new Date(d.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      hitRatePercent: d.hitRate * 100,
      missRatePercent: d.missRate * 100,
    }))
  }, [data])
  
  const toggleSeries = (dataKey: string) => {
    setHiddenSeries(prev => {
      const next = new Set(prev)
      if (next.has(dataKey)) {
        next.delete(dataKey)
      } else {
        next.add(dataKey)
      }
      return next
    })
  }
  
  if (loading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className="h-6 w-48" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-[400px] w-full" />
        </CardContent>
      </Card>
    )
  }
  
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base font-medium">Performance Metrics</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="h-[400px] w-full">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={formattedData} margin={{ top: 5, right: 30, left: 0, bottom: 5 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" opacity={0.5} />
              <XAxis 
                dataKey="time" 
                stroke="hsl(var(--muted-foreground))"
                fontSize={12}
                tickLine={false}
                axisLine={false}
              />
              <YAxis 
                yAxisId="left"
                stroke="hsl(var(--muted-foreground))"
                fontSize={12}
                tickLine={false}
                axisLine={false}
                tickFormatter={(value) => `${value}%`}
                domain={[0, 100]}
              />
              <YAxis 
                yAxisId="right"
                orientation="right"
                stroke="hsl(var(--muted-foreground))"
                fontSize={12}
                tickLine={false}
                axisLine={false}
                tickFormatter={(value) => `${value}ms`}
                domain={[0, 'auto']}
              />
              <Tooltip 
                contentStyle={{
                  backgroundColor: 'hsl(var(--card))',
                  border: '1px solid hsl(var(--border))',
                  borderRadius: '8px',
                  boxShadow: 'var(--shadow-md)',
                }}
                labelStyle={{ color: 'hsl(var(--foreground))', fontWeight: 500 }}
                formatter={(value: number, name: string) => {
                  if (name === 'Hit Rate' || name === 'Miss Rate') {
                    return [`${value.toFixed(1)}%`, name]
                  }
                  return [`${value.toFixed(1)}ms`, name]
                }}
              />
              <Legend 
                onClick={(e) => toggleSeries(e.dataKey as string)}
                wrapperStyle={{ cursor: 'pointer' }}
              />
              {!hiddenSeries.has('hitRatePercent') && (
                <Line
                  yAxisId="left"
                  type="monotone"
                  dataKey="hitRatePercent"
                  name="Hit Rate"
                  stroke={chartColors.hitRate}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4, strokeWidth: 0 }}
                />
              )}
              {!hiddenSeries.has('missRatePercent') && (
                <Line
                  yAxisId="left"
                  type="monotone"
                  dataKey="missRatePercent"
                  name="Miss Rate"
                  stroke={chartColors.missRate}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4, strokeWidth: 0 }}
                />
              )}
              {!hiddenSeries.has('p95Latency') && (
                <Line
                  yAxisId="right"
                  type="monotone"
                  dataKey="p95Latency"
                  name="P95 Latency"
                  stroke={chartColors.p95Latency}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4, strokeWidth: 0 }}
                />
              )}
            </LineChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  )
}
