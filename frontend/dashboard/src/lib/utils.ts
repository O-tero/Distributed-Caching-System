import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Format a number with K, M, B suffixes
 */
export function formatNumber(value: number): string {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(1)}B`
  }
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(1)}K`
  }
  return value.toString()
}

/**
 * Format bytes to human-readable format
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const k = 1024
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${units[i]}`
}

/**
 * Format milliseconds to readable duration
 */
export function formatDuration(ms: number): string {
  if (ms < 1) return `${(ms * 1000).toFixed(0)}μs`
  if (ms < 1000) return `${ms.toFixed(1)}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

/**
 * Format a decimal as percentage
 */
export function formatPercentage(value: number): string {
  return `${(value * 100).toFixed(1)}%`
}

/**
 * Format timestamp to relative time
 */
export function formatRelativeTime(timestamp: string | Date): string {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)
  const diffDays = Math.floor(diffHours / 24)
  
  if (diffSeconds < 60) return 'just now'
  if (diffMinutes < 60) return `${diffMinutes}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays < 7) return `${diffDays}d ago`
  
  return date.toLocaleDateString()
}

/**
 * Get trend direction and color
 */
export function getTrendInfo(current: number, previous: number): {
  direction: 'up' | 'down' | 'neutral'
  percentage: number
  color: string
} {
  const diff = current - previous
  const percentage = previous !== 0 ? Math.abs((diff / previous) * 100) : 0
  
  if (Math.abs(diff) < 0.001) {
    return { direction: 'neutral', percentage: 0, color: 'text-muted-foreground' }
  }
  
  return {
    direction: diff > 0 ? 'up' : 'down',
    percentage,
    color: diff > 0 ? 'text-success' : 'text-destructive'
  }
}

/**
 * Get status color based on value thresholds
 */
export function getStatusColor(value: number, thresholds: { good: number; warning: number }): {
  color: string
  bgColor: string
  status: 'good' | 'warning' | 'critical'
} {
  if (value >= thresholds.good) {
    return { color: 'text-success', bgColor: 'bg-success/10', status: 'good' }
  }
  if (value >= thresholds.warning) {
    return { color: 'text-warning', bgColor: 'bg-warning/10', status: 'warning' }
  }
  return { color: 'text-destructive', bgColor: 'bg-destructive/10', status: 'critical' }
}

/**
 * Debounce function
 */
export function debounce<T extends (...args: unknown[]) => unknown>(
  func: T,
  wait: number
): (...args: Parameters<T>) => void {
  let timeout: NodeJS.Timeout | null = null
  
  return (...args: Parameters<T>) => {
    if (timeout) clearTimeout(timeout)
    timeout = setTimeout(() => func(...args), wait)
  }
}

/**
 * Generate random ID
 */
export function generateId(): string {
  return Math.random().toString(36).substring(2, 9)
}
