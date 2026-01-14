import { NavLink, useLocation } from 'react-router-dom'
import { 
  LayoutDashboard, 
  Search, 
  Trash2, 
  Flame, 
  Settings,
  X,
  ChevronLeft
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface SidebarProps {
  isOpen: boolean
  onClose: () => void
  isCollapsed: boolean
  onToggleCollapse: () => void
}

const navItems = [
  { path: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { path: '/cache', icon: Search, label: 'Cache Explorer' },
  { path: '/invalidate', icon: Trash2, label: 'Invalidation' },
  { path: '/warming', icon: Flame, label: 'Warming Jobs' },
  { path: '/settings', icon: Settings, label: 'Settings' },
]

export function Sidebar({ isOpen, onClose, isCollapsed, onToggleCollapse }: SidebarProps) {
  const location = useLocation()
  
  return (
    <>
      {/* Mobile overlay */}
      {isOpen && (
        <div 
          className="fixed inset-0 z-40 bg-foreground/20 backdrop-blur-sm lg:hidden"
          onClick={onClose}
        />
      )}
      
      {/* Sidebar */}
      <aside
        className={cn(
          "fixed left-0 top-14 z-50 h-[calc(100vh-3.5rem)] bg-sidebar transition-all duration-300 ease-in-out lg:sticky",
          isOpen ? "translate-x-0" : "-translate-x-full lg:translate-x-0",
          isCollapsed ? "w-16" : "w-64"
        )}
      >
        <div className="flex h-full flex-col">
          {/* Mobile close button */}
          <div className="flex items-center justify-between p-4 lg:hidden">
            <span className="text-sm font-medium text-sidebar-foreground">Menu</span>
            <Button 
              variant="ghost" 
              size="icon" 
              onClick={onClose}
              className="text-sidebar-foreground hover:bg-sidebar-accent"
            >
              <X className="h-5 w-5" />
            </Button>
          </div>
          
          {/* Navigation */}
          <nav className="flex-1 space-y-1 px-3 py-4">
            {navItems.map((item) => {
              const isActive = location.pathname === item.path
              
              return (
                <NavLink
                  key={item.path}
                  to={item.path}
                  onClick={onClose}
                  className={cn(
                    "flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors",
                    isActive 
                      ? "bg-sidebar-accent text-sidebar-primary" 
                      : "text-sidebar-muted hover:bg-sidebar-accent hover:text-sidebar-foreground",
                    isCollapsed && "justify-center px-2"
                  )}
                  title={isCollapsed ? item.label : undefined}
                >
                  <item.icon className={cn("h-5 w-5 flex-shrink-0", isActive && "text-sidebar-primary")} />
                  {!isCollapsed && <span>{item.label}</span>}
                </NavLink>
              )
            })}
          </nav>
          
          {/* Footer */}
          <div className="border-t border-sidebar-border p-4">
            {!isCollapsed && (
              <div className="mb-3 text-xs text-sidebar-muted">
                Cache Dashboard v1.0.0
              </div>
            )}
            
            {/* Collapse toggle - desktop only */}
            <Button
              variant="ghost"
              size="sm"
              onClick={onToggleCollapse}
              className={cn(
                "hidden w-full lg:flex",
                "text-sidebar-muted hover:bg-sidebar-accent hover:text-sidebar-foreground",
                isCollapsed && "justify-center px-0"
              )}
            >
              <ChevronLeft className={cn(
                "h-4 w-4 transition-transform",
                isCollapsed && "rotate-180"
              )} />
              {!isCollapsed && <span className="ml-2">Collapse</span>}
            </Button>
          </div>
        </div>
      </aside>
    </>
  )
}
