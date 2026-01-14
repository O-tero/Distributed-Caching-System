import { LucideIcon, TrendingUp, TrendingDown, Minus } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

interface MetricCardProps {
  title: string
  value: string | number
  subtitle?: string
  icon: LucideIcon
  loading?: boolean
  trend?: {
    direction: 'up' | 'down' | 'neutral'
    value: string
    isPositive?: boolean
  }
  color?: 'default' | 'success' | 'warning' | 'destructive' | 'primary'
}

const colorStyles = {
  default: {
    icon: 'bg-muted text-muted-foreground',
    value: 'text-foreground',
  },
  success: {
    icon: 'bg-success/10 text-success',
    value: 'text-success',
  },
  warning: {
    icon: 'bg-warning/10 text-warning',
    value: 'text-warning',
  },
  destructive: {
    icon: 'bg-destructive/10 text-destructive',
    value: 'text-destructive',
  },
  primary: {
    icon: 'bg-primary/10 text-primary',
    value: 'text-primary',
  },
}

export function MetricCard({
  title,
  value,
  subtitle,
  icon: Icon,
  loading = false,
  trend,
  color = 'default',
}: MetricCardProps) {
  const styles = colorStyles[color]
  
  if (loading) {
    return (
      <Card className="metric-card">
        <CardContent className="p-6">
          <div className="flex items-start justify-between">
            <div className="space-y-2">
              <Skeleton className="h-4 w-24" />
              <Skeleton className="h-8 w-20" />
              <Skeleton className="h-3 w-32" />
            </div>
            <Skeleton className="h-10 w-10 rounded-lg" />
          </div>
        </CardContent>
      </Card>
    )
  }
  
  const TrendIcon = trend?.direction === 'up' 
    ? TrendingUp 
    : trend?.direction === 'down' 
      ? TrendingDown 
      : Minus
  
  return (
    <Card className="metric-card">
      <CardContent className="p-6">
        <div className="flex items-start justify-between">
          <div className="space-y-1">
            <p className="text-sm font-medium text-muted-foreground">{title}</p>
            <div className="flex items-baseline gap-2">
              <span className={cn("text-2xl font-bold tracking-tight", styles.value)}>
                {value}
              </span>
              {trend && (
                <div className={cn(
                  "flex items-center gap-0.5 text-xs font-medium",
                  trend.isPositive !== undefined
                    ? trend.isPositive ? "text-success" : "text-destructive"
                    : trend.direction === 'up' ? "text-success" : trend.direction === 'down' ? "text-destructive" : "text-muted-foreground"
                )}>
                  <TrendIcon className="h-3 w-3" />
                  <span>{trend.value}</span>
                </div>
              )}
            </div>
            {subtitle && (
              <p className="text-xs text-muted-foreground">{subtitle}</p>
            )}
          </div>
          
          <div className={cn("flex h-10 w-10 items-center justify-center rounded-lg", styles.icon)}>
            <Icon className="h-5 w-5" />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
