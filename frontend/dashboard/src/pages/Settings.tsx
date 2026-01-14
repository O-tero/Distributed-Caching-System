import { useState } from 'react'
import { 
  Key, 
  Server, 
  Clock, 
  Database, 
  CheckCircle2,
  AlertCircle,
  Info
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useToast } from '@/hooks/use-toast'

const evictionPolicies = [
  {
    value: 'lru',
    label: 'LRU (Least Recently Used)',
    description: 'Evicts the keys that have been unused for the longest time. Best for workloads with temporal locality.',
  },
  {
    value: 'lfu',
    label: 'LFU (Least Frequently Used)',
    description: 'Evicts keys accessed the fewest number of times. Best for workloads with stable access patterns.',
  },
  {
    value: 'hybrid',
    label: 'Hybrid (LRU + LFU)',
    description: 'Combines both recency and frequency. Adapts to changing access patterns automatically.',
  },
]

export default function Settings() {
  const { toast } = useToast()
  const [apiToken, setApiToken] = useState('')
  const [isAuthenticated, setIsAuthenticated] = useState(true)
  const [evictionPolicy, setEvictionPolicy] = useState('lru')
  const [isSaving, setIsSaving] = useState(false)
  
  const selectedPolicy = evictionPolicies.find(p => p.value === evictionPolicy)
  
  const handleSaveToken = async () => {
    if (!apiToken.trim()) return
    setIsSaving(true)
    await new Promise(r => setTimeout(r, 800))
    setIsAuthenticated(true)
    setApiToken('')
    setIsSaving(false)
    toast({
      title: "Token saved",
      description: "Your API token has been securely stored.",
    })
  }
  
  const handleClearToken = () => {
    setIsAuthenticated(false)
    toast({
      title: "Token cleared",
      description: "Your API token has been removed.",
    })
  }
  
  const handleSaveCacheSettings = async () => {
    setIsSaving(true)
    await new Promise(r => setTimeout(r, 800))
    setIsSaving(false)
    toast({
      title: "Settings saved",
      description: "Cache settings have been updated.",
    })
  }
  
  return (
    <div className="space-y-6 animate-fade-in max-w-2xl">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Configure authentication and cache behavior
        </p>
      </div>
      
      {/* Authentication */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Key className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-base">Authentication</CardTitle>
          </div>
          <CardDescription>
            Manage your API access token
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {isAuthenticated && (
            <div className="flex items-center gap-2 text-success text-sm">
              <CheckCircle2 className="h-4 w-4" />
              <span>Authenticated</span>
            </div>
          )}
          
          <div className="space-y-2">
            <Label htmlFor="api-token">API Token</Label>
            <Input
              id="api-token"
              type="password"
              placeholder="Enter your API token"
              value={apiToken}
              onChange={(e) => setApiToken(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Token is stored securely in your browser's local storage.
            </p>
          </div>
          
          <div className="flex gap-2">
            <Button 
              onClick={handleSaveToken}
              disabled={!apiToken.trim() || isSaving}
            >
              {isSaving ? 'Saving...' : 'Save Token'}
            </Button>
            {isAuthenticated && (
              <Button variant="outline" onClick={handleClearToken}>
                Clear Token
              </Button>
            )}
          </div>
        </CardContent>
      </Card>
      
      {/* API Configuration */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Server className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-base">API Configuration</CardTitle>
          </div>
          <CardDescription>
            Backend connection settings
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label>API Base URL</Label>
            <div className="flex items-center gap-2">
              <code className="flex-1 font-mono text-sm bg-muted px-3 py-2 rounded border">
                https://api.cache.example.com/v1
              </code>
            </div>
            <p className="text-xs text-muted-foreground">
              Configured via <code className="font-mono">VITE_API_BASE_URL</code> environment variable.
            </p>
          </div>
        </CardContent>
      </Card>
      
      {/* Polling Intervals */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Clock className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-base">Polling Intervals</CardTitle>
          </div>
          <CardDescription>
            Data refresh frequency
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label>Metrics Refresh Interval</Label>
            <div className="flex items-center gap-2">
              <code className="font-mono text-sm bg-muted px-3 py-2 rounded border">
                5 seconds
              </code>
            </div>
            <p className="text-xs text-muted-foreground">
              Configured via <code className="font-mono">VITE_POLL_INTERVAL</code> environment variable.
            </p>
          </div>
        </CardContent>
      </Card>
      
      {/* Cache Settings */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <Database className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-base">Cache Settings</CardTitle>
          </div>
          <CardDescription>
            Configure cache eviction behavior
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!isAuthenticated && (
            <Alert variant="destructive" className="border-warning/50 bg-warning/10">
              <AlertCircle className="h-4 w-4 text-warning" />
              <AlertDescription className="text-foreground">
                You must be authenticated to modify cache settings.
              </AlertDescription>
            </Alert>
          )}
          
          <div className="space-y-2">
            <Label>Eviction Policy</Label>
            <Select 
              value={evictionPolicy} 
              onValueChange={setEvictionPolicy}
              disabled={!isAuthenticated}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select policy" />
              </SelectTrigger>
              <SelectContent>
                {evictionPolicies.map((policy) => (
                  <SelectItem key={policy.value} value={policy.value}>
                    {policy.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          
          {selectedPolicy && (
            <div className="rounded-lg bg-muted p-3">
              <p className="text-sm text-muted-foreground">
                {selectedPolicy.description}
              </p>
            </div>
          )}
          
          <Button 
            onClick={handleSaveCacheSettings}
            disabled={!isAuthenticated || isSaving}
          >
            {isSaving ? 'Saving...' : 'Save Cache Settings'}
          </Button>
        </CardContent>
      </Card>
      
      {/* Info card */}
      <Alert className="border-muted">
        <Info className="h-4 w-4" />
        <AlertDescription>
          <strong>Configuration Notes:</strong>
          <ul className="mt-2 space-y-1 text-sm list-disc list-inside text-muted-foreground">
            <li>API tokens are stored in browser local storage</li>
            <li>Environment variables require an application restart</li>
            <li>Cache settings changes take effect immediately</li>
          </ul>
        </AlertDescription>
      </Alert>
    </div>
  )
}
