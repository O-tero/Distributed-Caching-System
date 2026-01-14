import { useState, useEffect, useMemo, useCallback } from 'react'
import { 
  Search, 
  RefreshCw, 
  Download, 
  Trash2, 
  Eye,
  ChevronLeft,
  ChevronRight,
  X
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { generateCacheKeys, type CacheKey } from '@/lib/mock-data'
import { formatBytes, formatRelativeTime, formatDuration, cn, debounce } from '@/lib/utils'

const PAGE_SIZE = 10

export default function CacheExplorer() {
  const [allKeys, setAllKeys] = useState<CacheKey[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set())
  const [currentPage, setCurrentPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [selectedKeyDetail, setSelectedKeyDetail] = useState<CacheKey | null>(null)
  
  // Load initial data
  useEffect(() => {
    loadKeys()
  }, [])
  
  // Debounce search
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(searchQuery)
      setCurrentPage(1)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])
  
  const loadKeys = async () => {
    setLoading(true)
    // Simulate API delay
    await new Promise(r => setTimeout(r, 500))
    setAllKeys(generateCacheKeys(100))
    setLoading(false)
  }
  
  const handleRefresh = async () => {
    setRefreshing(true)
    await new Promise(r => setTimeout(r, 800))
    setAllKeys(generateCacheKeys(100))
    setSelectedKeys(new Set())
    setRefreshing(false)
  }
  
  // Filter and paginate keys
  const filteredKeys = useMemo(() => {
    if (!debouncedQuery) return allKeys
    
    const pattern = debouncedQuery.replace(/\*/g, '.*')
    const regex = new RegExp(`^${pattern}`, 'i')
    return allKeys.filter(key => regex.test(key.key))
  }, [allKeys, debouncedQuery])
  
  const totalPages = Math.ceil(filteredKeys.length / PAGE_SIZE)
  const paginatedKeys = filteredKeys.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE)
  
  // Selection handlers
  const toggleSelectAll = () => {
    const pageKeys = paginatedKeys.map(k => k.key)
    const allSelected = pageKeys.every(k => selectedKeys.has(k))
    
    if (allSelected) {
      setSelectedKeys(prev => {
        const next = new Set(prev)
        pageKeys.forEach(k => next.delete(k))
        return next
      })
    } else {
      setSelectedKeys(prev => new Set([...prev, ...pageKeys]))
    }
  }
  
  const toggleSelect = (key: string) => {
    setSelectedKeys(prev => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }
  
  const isAllSelected = paginatedKeys.length > 0 && 
    paginatedKeys.every(k => selectedKeys.has(k.key))
  
  const handleExportCSV = () => {
    const headers = ['Key', 'Size (bytes)', 'TTL (s)', 'Last Access', 'Access Count']
    const rows = filteredKeys.map(k => [
      k.key,
      k.size,
      k.ttl,
      k.lastAccess,
      k.accessCount
    ])
    
    const csv = [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
    const blob = new Blob([csv], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'cache-keys.csv'
    a.click()
    URL.revokeObjectURL(url)
  }
  
  return (
    <div className="space-y-6 animate-fade-in">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Cache Explorer</h1>
        <p className="text-sm text-muted-foreground">
          Browse and manage cached keys
        </p>
      </div>
      
      {/* Search and actions */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search by prefix (e.g., users:*)"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 font-mono text-sm"
          />
        </div>
        
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={refreshing}
          >
            <RefreshCw className={cn("mr-2 h-4 w-4", refreshing && "animate-spin")} />
            Refresh
          </Button>
          
          <Button
            variant="outline"
            size="sm"
            onClick={handleExportCSV}
            disabled={filteredKeys.length === 0}
          >
            <Download className="mr-2 h-4 w-4" />
            Export
          </Button>
          
          {selectedKeys.size > 0 && (
            <Button
              variant="destructive"
              size="sm"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Invalidate ({selectedKeys.size})
            </Button>
          )}
        </div>
      </div>
      
      {/* Stats cards */}
      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{filteredKeys.length.toLocaleString()}</div>
            <p className="text-xs text-muted-foreground">Total Keys</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">
              {currentPage} / {Math.max(1, totalPages)}
            </div>
            <p className="text-xs text-muted-foreground">Current Page</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{selectedKeys.size}</div>
            <p className="text-xs text-muted-foreground">Selected Keys</p>
          </CardContent>
        </Card>
      </div>
      
      {/* Keys table */}
      <Card>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-12">
                    <Checkbox 
                      checked={isAllSelected}
                      onCheckedChange={toggleSelectAll}
                      aria-label="Select all"
                    />
                  </TableHead>
                  <TableHead>Key</TableHead>
                  <TableHead className="w-24">Size</TableHead>
                  <TableHead className="w-24">TTL</TableHead>
                  <TableHead className="w-32">Last Access</TableHead>
                  <TableHead className="w-24 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  Array.from({ length: PAGE_SIZE }).map((_, i) => (
                    <TableRow key={i}>
                      <TableCell><Skeleton className="h-4 w-4" /></TableCell>
                      <TableCell><Skeleton className="h-4 w-48" /></TableCell>
                      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
                      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
                      <TableCell><Skeleton className="h-4 w-20" /></TableCell>
                      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
                    </TableRow>
                  ))
                ) : paginatedKeys.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="h-32 text-center">
                      <div className="text-muted-foreground">
                        {debouncedQuery ? 'No keys match your search' : 'No cache keys found'}
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  paginatedKeys.map((key) => (
                    <TableRow key={key.key} className="table-row-hover">
                      <TableCell>
                        <Checkbox
                          checked={selectedKeys.has(key.key)}
                          onCheckedChange={() => toggleSelect(key.key)}
                          aria-label={`Select ${key.key}`}
                        />
                      </TableCell>
                      <TableCell className="font-mono text-sm">{key.key}</TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatBytes(key.size)}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatDuration(key.ttl * 1000)}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatRelativeTime(key.lastAccess)}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8"
                            onClick={() => setSelectedKeyDetail(key)}
                            aria-label="View details"
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-destructive hover:text-destructive"
                            aria-label="Invalidate"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          
          {/* Pagination */}
          {!loading && totalPages > 1 && (
            <div className="flex items-center justify-between border-t px-4 py-3">
              <div className="text-sm text-muted-foreground">
                Showing {((currentPage - 1) * PAGE_SIZE) + 1} to{' '}
                {Math.min(currentPage * PAGE_SIZE, filteredKeys.length)} of{' '}
                {filteredKeys.length} keys
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCurrentPage(p => p - 1)}
                  disabled={currentPage === 1}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Previous
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCurrentPage(p => p + 1)}
                  disabled={currentPage >= totalPages}
                >
                  Next
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
      
      {/* Key detail modal */}
      <Dialog open={!!selectedKeyDetail} onOpenChange={() => setSelectedKeyDetail(null)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Key Details</DialogTitle>
          </DialogHeader>
          
          {selectedKeyDetail && (
            <div className="space-y-4">
              <div className="rounded-lg bg-muted p-3">
                <p className="font-mono text-sm break-all">{selectedKeyDetail.key}</p>
              </div>
              
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <p className="text-sm font-medium text-muted-foreground">Size</p>
                  <p className="text-lg font-semibold">{formatBytes(selectedKeyDetail.size)}</p>
                </div>
                <div>
                  <p className="text-sm font-medium text-muted-foreground">TTL Remaining</p>
                  <p className="text-lg font-semibold">{formatDuration(selectedKeyDetail.ttl * 1000)}</p>
                </div>
                <div>
                  <p className="text-sm font-medium text-muted-foreground">Access Count</p>
                  <p className="text-lg font-semibold">{selectedKeyDetail.accessCount.toLocaleString()}</p>
                </div>
                <div>
                  <p className="text-sm font-medium text-muted-foreground">Last Accessed</p>
                  <p className="text-lg font-semibold">{formatRelativeTime(selectedKeyDetail.lastAccess)}</p>
                </div>
              </div>
              
              <div>
                <p className="text-sm font-medium text-muted-foreground">Created</p>
                <p className="text-sm">{new Date(selectedKeyDetail.createdAt).toLocaleString()}</p>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
