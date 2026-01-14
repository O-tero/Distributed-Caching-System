import { AlertCircle, Info, AlertTriangle, Trash2 } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn, formatRelativeTime } from '@/lib/utils'
import type { CacheEvent } from '@/lib/mock-data'

interface RecentActivityProps {
  events: CacheEvent[]
  loading?: boolean
}

const eventTypeConfig = {
  invalidation: {
    icon: Trash2,
    color: 'text-primary',
    bgColor: 'bg-primary/10',
  },
  warning: {
    icon: AlertTriangle,
    color: 'text-warning',
    bgColor: 'bg-warning/10',
  },
  error: {
    icon: AlertCircle,
    color: 'text-destructive',
    bgColor: 'bg-destructive/10',
  },
  info: {
    icon: Info,
    color: 'text-muted-foreground',
    bgColor: 'bg-muted',
  },
}

export function RecentActivity({ events, loading = false }: RecentActivityProps) {
  if (loading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className="h-6 w-36" />
        </CardHeader>
        <CardContent className="space-y-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="flex items-start gap-3">
              <Skeleton className="h-8 w-8 rounded-lg" />
              <div className="flex-1 space-y-2">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-3 w-24" />
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
    )
  }
  
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base font-medium">Recent Activity</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {events.slice(0, 5).map((event, index) => {
            const config = eventTypeConfig[event.type]
            const Icon = config.icon
            
            return (
              <div 
                key={event.id}
                className={cn(
                  "flex items-start gap-3 animate-fade-in",
                )}
                style={{ animationDelay: `${index * 50}ms` }}
              >
                <div className={cn(
                  "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg",
                  config.bgColor
                )}>
                  <Icon className={cn("h-4 w-4", config.color)} />
                </div>
                
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium leading-tight">{event.message}</p>
                  <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                    <span>{event.service}</span>
                    <span>•</span>
                    <span>{formatRelativeTime(event.timestamp)}</span>
                    {event.keysAffected !== undefined && (
                      <>
                        <span>•</span>
                        <span>{event.keysAffected.toLocaleString()} keys</span>
                      </>
                    )}
                  </div>
                </div>
              </div>
            )
          })}
          
          {events.length === 0 && (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No recent activity
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
