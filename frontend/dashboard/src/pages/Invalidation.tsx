import { useState, useMemo } from 'react'
import { AlertTriangle, Trash2, Clock, CheckCircle2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ScrollArea } from '@/components/ui/scroll-area'
import { 
  generateCacheKeys, 
  generateInvalidationRecords, 
  matchKeysForPattern,
  type CacheKey,
  type InvalidationRecord 
} from '@/lib/mock-data'
import { formatRelativeTime, cn } from '@/lib/utils'

const commonPatterns = [
  { label: 'All Users', pattern: 'users:*' },
  { label: 'All Products', pattern: 'products:*' },
  { label: 'All Sessions', pattern: 'sessions:*' },
  { label: 'User Profiles', pattern: 'users:*:profile' },
  { label: 'Product Cache', pattern: 'products:*:details' },
]

export default function Invalidation() {
  const [pattern, setPattern] = useState('')
  const [showPreview, setShowPreview] = useState(false)
  const [isDryRun, setIsDryRun] = useState(false)
  const [isInvalidating, setIsInvalidating] = useState(false)
  const [recentInvalidations] = useState<InvalidationRecord[]>(generateInvalidationRecords())
  const [allKeys] = useState<CacheKey[]>(generateCacheKeys(200))
  
  const matchingKeys = useMemo(() => {
    if (!pattern.trim()) return []
    return matchKeysForPattern(pattern, allKeys)
  }, [pattern, allKeys])
  
  const handlePreview = () => {
    if (!pattern.trim()) return
    setShowPreview(true)
  }
  
  const handleInvalidate = async () => {
    setIsInvalidating(true)
    await new Promise(r => setTimeout(r, 1500))
    setIsInvalidating(false)
    setShowPreview(false)
    setPattern('')
  }
  
  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Invalidation Console</h1>
        <p className="text-sm text-muted-foreground">
          Bulk invalidate cache keys by pattern
        </p>
      </div>
      
      {/* Warning banner */}
      <Alert variant="destructive" className="border-warning/50 bg-warning/10 text-warning">
        <AlertTriangle className="h-4 w-4" />
        <AlertDescription className="text-foreground">
          <strong>Caution: Mass Invalidation</strong> — Invalidating large numbers of keys can 
          impact cache performance and increase load on your data sources. Always preview 
          matches before confirming.
        </AlertDescription>
      </Alert>
      
      {/* Pattern input */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Invalidation Pattern</CardTitle>
          <CardDescription>
            Enter a pattern to match cache keys. Use * as wildcard.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-3">
            <Input
              placeholder="e.g., users:* or products:*:price"
              value={pattern}
              onChange={(e) => setPattern(e.target.value)}
              className="font-mono flex-1"
            />
            <Button 
              variant="destructive"
              onClick={handlePreview}
              disabled={!pattern.trim()}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Preview & Invalidate
            </Button>
          </div>
          
          <p className="text-xs text-muted-foreground">
            Use * as wildcard. Example: <code className="font-mono bg-muted px-1 rounded">users:*</code> matches all user keys.
          </p>
        </CardContent>
      </Card>
      
      {/* Common patterns */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Common Patterns</CardTitle>
          <CardDescription>Quick access to frequently used patterns</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2">
            {commonPatterns.map((item) => (
              <Button
                key={item.pattern}
                variant="secondary"
                size="sm"
                onClick={() => setPattern(item.pattern)}
                className="font-mono"
              >
                {item.label}
                <span className="ml-2 text-muted-foreground text-xs">{item.pattern}</span>
              </Button>
            ))}
          </div>
        </CardContent>
      </Card>
      
      {/* Pattern syntax guide */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Pattern Syntax Guide</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3 text-sm">
            <div className="flex items-start gap-3">
              <code className="font-mono bg-muted px-2 py-0.5 rounded shrink-0">*</code>
              <span className="text-muted-foreground">Matches any sequence of characters</span>
            </div>
            <div className="flex items-start gap-3">
              <code className="font-mono bg-muted px-2 py-0.5 rounded shrink-0">prefix:*</code>
              <span className="text-muted-foreground">Matches all keys starting with prefix</span>
            </div>
            <div className="flex items-start gap-3">
              <code className="font-mono bg-muted px-2 py-0.5 rounded shrink-0">*:suffix</code>
              <span className="text-muted-foreground">Matches all keys ending with suffix</span>
            </div>
            <div className="flex items-start gap-3">
              <code className="font-mono bg-muted px-2 py-0.5 rounded shrink-0">a:*:b</code>
              <span className="text-muted-foreground">Matches keys with prefix a and suffix b</span>
            </div>
          </div>
        </CardContent>
      </Card>
      
      {/* Recent invalidations */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Recent Invalidations</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {recentInvalidations.map((record) => (
              <div 
                key={record.id}
                className="flex items-center justify-between rounded-lg border p-3"
              >
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <code className="font-mono text-sm bg-muted px-2 py-0.5 rounded">
                      {record.pattern}
                    </code>
                    {record.isDryRun && (
                      <Badge variant="secondary" className="text-xs">Dry Run</Badge>
                    )}
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground">
                    <span>{record.keysInvalidated.toLocaleString()} keys invalidated</span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      {formatRelativeTime(record.timestamp)}
                    </span>
                  </div>
                </div>
                <CheckCircle2 className="h-5 w-5 text-success" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
      
      {/* Preview modal */}
      <Dialog open={showPreview} onOpenChange={setShowPreview}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Confirm Invalidation</DialogTitle>
            <DialogDescription>
              Review the keys that will be invalidated
            </DialogDescription>
          </DialogHeader>
          
          <Alert variant="destructive" className="border-destructive/50">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>
              This will invalidate <strong>{matchingKeys.length.toLocaleString()}</strong> cache keys
            </AlertDescription>
          </Alert>
          
          <div className="space-y-2">
            <p className="text-sm font-medium">Pattern</p>
            <code className="block font-mono text-sm bg-muted p-2 rounded">
              {pattern}
            </code>
          </div>
          
          <div className="space-y-2">
            <p className="text-sm font-medium">Matching Keys</p>
            <ScrollArea className="h-48 rounded border">
              <div className="p-3 space-y-1">
                {matchingKeys.slice(0, 100).map((key) => (
                  <div key={key.key} className="font-mono text-xs text-muted-foreground">
                    {key.key}
                  </div>
                ))}
                {matchingKeys.length > 100 && (
                  <div className="text-xs text-muted-foreground pt-2 border-t mt-2">
                    ... and {(matchingKeys.length - 100).toLocaleString()} more
                  </div>
                )}
                {matchingKeys.length === 0 && (
                  <div className="text-xs text-muted-foreground text-center py-4">
                    No matching keys found
                  </div>
                )}
              </div>
            </ScrollArea>
          </div>
          
          <div className="flex items-center space-x-2">
            <Checkbox
              id="dry-run"
              checked={isDryRun}
              onCheckedChange={(c) => setIsDryRun(c === true)}
            />
            <Label htmlFor="dry-run" className="text-sm cursor-pointer">
              Dry run (preview only, don't actually invalidate)
            </Label>
          </div>
          
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowPreview(false)}>
              Cancel
            </Button>
            <Button 
              variant="destructive" 
              onClick={handleInvalidate}
              disabled={isInvalidating || matchingKeys.length === 0}
            >
              {isInvalidating ? (
                <>
                  <span className="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
                  Invalidating...
                </>
              ) : (
                <>
                  <Trash2 className="mr-2 h-4 w-4" />
                  {isDryRun ? 'Run Dry Run' : 'Invalidate'}
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
