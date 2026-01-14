import { useState } from 'react'
import { 
  Flame, 
  Clock, 
  CheckCircle2, 
  XCircle, 
  AlertCircle,
  Play,
  Power,
  PowerOff
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { generateWarmingJobs, type WarmingJob } from '@/lib/mock-data'
import { formatRelativeTime, formatDuration, cn } from '@/lib/utils'

const statusConfig = {
  success: {
    icon: CheckCircle2,
    color: 'text-success',
    bgColor: 'bg-success/10',
    label: 'Success',
  },
  failed: {
    icon: XCircle,
    color: 'text-destructive',
    bgColor: 'bg-destructive/10',
    label: 'Failed',
  },
  partial: {
    icon: AlertCircle,
    color: 'text-warning',
    bgColor: 'bg-warning/10',
    label: 'Partial',
  },
}

export default function WarmingJobs() {
  const [warmingPattern, setWarmingPattern] = useState('')
  const [isTriggering, setIsTriggering] = useState(false)
  const [jobs, setJobs] = useState<WarmingJob[]>(generateWarmingJobs())
  
  const activeJobs = jobs.filter(j => j.enabled).length
  const successfulJobs = jobs.filter(j => j.lastRun?.status === 'success').length
  const successRate = jobs.length > 0 ? (successfulJobs / jobs.length) * 100 : 0
  
  const handleTriggerWarming = async () => {
    if (!warmingPattern.trim()) return
    setIsTriggering(true)
    await new Promise(r => setTimeout(r, 1500))
    setIsTriggering(false)
    setWarmingPattern('')
  }
  
  const toggleJobEnabled = (jobId: string) => {
    setJobs(jobs.map(j => 
      j.id === jobId ? { ...j, enabled: !j.enabled } : j
    ))
  }
  
  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Warming Jobs</h1>
        <p className="text-sm text-muted-foreground">
          Schedule and manage cache warming operations
        </p>
      </div>
      
      {/* Stats grid */}
      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{jobs.length}</div>
            <p className="text-xs text-muted-foreground">Total Jobs</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-success">{activeJobs}</div>
            <p className="text-xs text-muted-foreground">Active Jobs</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{successRate.toFixed(0)}%</div>
            <p className="text-xs text-muted-foreground">Success Rate</p>
          </CardContent>
        </Card>
      </div>
      
      {/* Manual trigger */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Manual Trigger</CardTitle>
          <CardDescription>
            Trigger an immediate cache warming operation
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-3">
            <Input
              placeholder="Enter pattern (e.g., products:*)"
              value={warmingPattern}
              onChange={(e) => setWarmingPattern(e.target.value)}
              className="font-mono flex-1"
            />
            <Button 
              onClick={handleTriggerWarming}
              disabled={!warmingPattern.trim() || isTriggering}
            >
              {isTriggering ? (
                <>
                  <span className="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
                  Warming...
                </>
              ) : (
                <>
                  <Flame className="mr-2 h-4 w-4" />
                  Trigger Warming
                </>
              )}
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Cache warming pre-populates the cache with data to reduce miss rates during peak traffic.
          </p>
        </CardContent>
      </Card>
      
      {/* Info card */}
      <Alert className="border-primary/20 bg-primary/5">
        <Flame className="h-4 w-4 text-primary" />
        <AlertDescription>
          <strong>What is cache warming?</strong> Cache warming proactively loads data into the cache 
          before it's needed, reducing latency for users and preventing cache stampedes during 
          traffic spikes. Schedule warming jobs during off-peak hours for best results.
        </AlertDescription>
      </Alert>
      
      {/* Scheduled jobs */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Scheduled Jobs</CardTitle>
          <CardDescription>
            Automated cache warming schedules
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {jobs.map((job) => {
            const status = job.lastRun?.status ? statusConfig[job.lastRun.status] : null
            const StatusIcon = status?.icon
            const successPercent = job.lastRun 
              ? (job.lastRun.keysWarmed / (job.lastRun.keysWarmed + job.lastRun.keysFailed)) * 100 
              : 0
            
            return (
              <div 
                key={job.id}
                className={cn(
                  "rounded-lg border p-4 transition-colors",
                  !job.enabled && "opacity-60"
                )}
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="space-y-3 flex-1">
                    {/* Job header */}
                    <div className="flex items-center gap-2">
                      <h3 className="font-medium">{job.name}</h3>
                      <Badge variant={job.enabled ? 'default' : 'secondary'}>
                        {job.enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                    
                    {/* Pattern and cron */}
                    <div className="flex flex-wrap items-center gap-4 text-sm">
                      <code className="font-mono bg-muted px-2 py-0.5 rounded">
                        {job.pattern}
                      </code>
                      <span className="flex items-center gap-1 text-muted-foreground">
                        <Clock className="h-3 w-3" />
                        {job.cron}
                      </span>
                    </div>
                    
                    {/* Last run info */}
                    {job.lastRun && (
                      <div className="space-y-2">
                        <div className="flex items-center gap-3 text-sm">
                          {StatusIcon && (
                            <span className={cn("flex items-center gap-1", status.color)}>
                              <StatusIcon className="h-4 w-4" />
                              {status.label}
                            </span>
                          )}
                          <span className="text-muted-foreground">
                            {job.lastRun.keysWarmed.toLocaleString()} keys warmed
                          </span>
                          <span className="text-muted-foreground">
                            {formatDuration(job.lastRun.duration)}
                          </span>
                          <span className="text-muted-foreground">
                            {formatRelativeTime(job.lastRun.timestamp)}
                          </span>
                        </div>
                        
                        {/* Progress bar for partial */}
                        {job.lastRun.status === 'partial' && (
                          <div className="space-y-1">
                            <Progress value={successPercent} className="h-2" />
                            <div className="flex justify-between text-xs text-muted-foreground">
                              <span>{job.lastRun.keysWarmed.toLocaleString()} succeeded</span>
                              <span>{job.lastRun.keysFailed.toLocaleString()} failed</span>
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                  
                  {/* Actions */}
                  <div className="flex items-center gap-2">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8"
                      aria-label="Run now"
                    >
                      <Play className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className={cn(
                        "h-8 w-8",
                        job.enabled ? "text-success hover:text-success" : "text-muted-foreground"
                      )}
                      onClick={() => toggleJobEnabled(job.id)}
                      aria-label={job.enabled ? 'Disable' : 'Enable'}
                    >
                      {job.enabled ? (
                        <Power className="h-4 w-4" />
                      ) : (
                        <PowerOff className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                </div>
              </div>
            )
          })}
          
          {jobs.length === 0 && (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No warming jobs configured
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
