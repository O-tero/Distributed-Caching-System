package warming

import (
	"context"
	"sort"
	"time"
)

// Strategy defines the interface for cache warming strategies.
// Different strategies determine which keys to warm and in what order.
type Strategy interface {
	Name() string
	Plan(ctx context.Context, opts PlanOptions) ([]WarmTask, error)
}

// PlanOptions provides input parameters for warming strategy planning.
type PlanOptions struct {
	Keys     []string          // Keys to consider for warming
	Priority int               // Base priority level
	Limit    int               // Maximum number of tasks to generate
	Metadata map[string]string // Additional strategy-specific metadata
}

// WarmTask represents a single cache warming task.
type WarmTask struct {
	Key           string        // Cache key to warm
	Priority      int           // Task priority (higher = more important)
	EstimatedCost int           // Estimated cost in milliseconds
	TTL           time.Duration // Cache TTL for this key
	Strategy      string        // Strategy that created this task
	Metadata      map[string]interface{} // Additional task metadata
}

// SelectiveHotKeysStrategy warms only the hottest keys based on access frequency.
// This strategy is efficient for high-traffic scenarios where most requests
// target a small subset of keys (Pareto principle / 80-20 rule).
type SelectiveHotKeysStrategy struct {
	name string
}

// NewSelectiveHotKeysStrategy creates a new selective hot keys strategy.
func NewSelectiveHotKeysStrategy() Strategy {
	return &SelectiveHotKeysStrategy{
		name: "selective",
	}
}

func (s *SelectiveHotKeysStrategy) Name() string {
	return s.name
}

// Plan generates warming tasks for the hottest keys.
// Assumes keys are already sorted by hotness (most frequent first).
// Complexity: O(n) where n = min(len(keys), limit)
func (s *SelectiveHotKeysStrategy) Plan(ctx context.Context, opts PlanOptions) ([]WarmTask, error) {
	limit := opts.Limit
	if limit <= 0 || limit > len(opts.Keys) {
		limit = len(opts.Keys)
	}

	// Apply a reasonable cap to prevent runaway warming
	if limit > 1000 {
		limit = 1000
	}

	tasks := make([]WarmTask, 0, limit)
	
	// Take top N hottest keys
	for i := 0; i < limit && i < len(opts.Keys); i++ {
		key := opts.Keys[i]
		
		// Priority decreases for less hot keys
		priority := opts.Priority
		if opts.Priority == 0 {
			priority = 100 - (i * 100 / limit) // Linear decrease from 100 to 0
		}

		tasks = append(tasks, WarmTask{
			Key:           key,
			Priority:      priority,
			EstimatedCost: estimateFetchCost(key),
			TTL:           1 * time.Hour,
			Strategy:      s.name,
		})
	}

	return tasks, nil
}

// BreadthFirstStrategy warms keys based on dependency relationships.
// Useful when cache keys have hierarchical relationships (e.g., user -> posts -> comments).
// Ensures parent keys are warmed before children to prevent cascading misses.
type BreadthFirstStrategy struct {
	name string
}

// NewBreadthFirstStrategy creates a new breadth-first strategy.
func NewBreadthFirstStrategy() Strategy {
	return &BreadthFirstStrategy{
		name: "breadth",
	}
}

func (s *BreadthFirstStrategy) Name() string {
	return s.name
}

// Plan generates warming tasks in breadth-first order.
// This assumes keys are structured hierarchically (e.g., "user:123", "user:123:posts", "user:123:posts:456").
// Complexity: O(n log n) for sorting + O(n) for task generation
func (s *BreadthFirstStrategy) Plan(ctx context.Context, opts PlanOptions) ([]WarmTask, error) {
	if len(opts.Keys) == 0 {
		return []WarmTask{}, nil
	}

	// Sort keys by depth (fewer colons = higher in hierarchy)
	sortedKeys := make([]string, len(opts.Keys))
	copy(sortedKeys, opts.Keys)
	
	sort.Slice(sortedKeys, func(i, j int) bool {
		depthI := keyDepth(sortedKeys[i])
		depthJ := keyDepth(sortedKeys[j])
		if depthI == depthJ {
			return sortedKeys[i] < sortedKeys[j] // Alphabetical for same depth
		}
		return depthI < depthJ // Shallower keys first
	})

	limit := opts.Limit
	if limit <= 0 || limit > len(sortedKeys) {
		limit = len(sortedKeys)
	}

	tasks := make([]WarmTask, 0, limit)
	
	for i := 0; i < limit && i < len(sortedKeys); i++ {
		key := sortedKeys[i]
		depth := keyDepth(key)
		
		// Higher priority for shallower (parent) keys
		priority := opts.Priority
		if priority == 0 {
			priority = 100 - (depth * 10)
			if priority < 0 {
				priority = 0
			}
		}

		tasks = append(tasks, WarmTask{
			Key:           key,
			Priority:      priority,
			EstimatedCost: estimateFetchCost(key),
			TTL:           1 * time.Hour,
			Strategy:      s.name,
			Metadata: map[string]interface{}{
				"depth": depth,
			},
		})
	}

	return tasks, nil
}

// keyDepth calculates the hierarchical depth of a key based on separator count.
func keyDepth(key string) int {
	depth := 0
	for _, ch := range key {
		if ch == ':' {
			depth++
		}
	}
	return depth
}

// PriorityBasedStrategy warms keys based on a calculated priority score.
// Score = (importance * hotness) / cost
// This balances multiple factors to optimize warming efficiency.
type PriorityBasedStrategy struct {
	name string
}

// NewPriorityBasedStrategy creates a new priority-based strategy.
func NewPriorityBasedStrategy() Strategy {
	return &PriorityBasedStrategy{
		name: "priority",
	}
}

func (s *PriorityBasedStrategy) Name() string {
	return s.name
}

// Plan generates warming tasks sorted by calculated priority score.
// Complexity: O(n log n) for sorting
func (s *PriorityBasedStrategy) Plan(ctx context.Context, opts PlanOptions) ([]WarmTask, error) {
	if len(opts.Keys) == 0 {
		return []WarmTask{}, nil
	}

	// Create tasks with calculated priorities
	tasks := make([]WarmTask, 0, len(opts.Keys))
	
	for i, key := range opts.Keys {
		cost := estimateFetchCost(key)
		
		// Calculate importance (decreases with position in list)
		importance := float64(len(opts.Keys)-i) / float64(len(opts.Keys))
		
		// Calculate hotness (assume keys are ordered by access frequency)
		hotness := 1.0
		if i < len(opts.Keys)/10 {
			hotness = 2.0 // Top 10% get double weight
		}
		
		// Priority score: higher importance and hotness, lower cost = higher priority
		score := (importance * hotness * 100) / float64(cost)
		priority := int(score)
		
		// Clamp to 0-100 range
		if priority > 100 {
			priority = 100
		}
		if priority < 0 {
			priority = 0
		}

		tasks = append(tasks, WarmTask{
			Key:           key,
			Priority:      priority,
			EstimatedCost: cost,
			TTL:           1 * time.Hour,
			Strategy:      s.name,
			Metadata: map[string]interface{}{
				"importance": importance,
				"hotness":    hotness,
				"score":      score,
			},
		})
	}

	// Sort by priority (highest first)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Priority > tasks[j].Priority
	})

	// Apply limit
	limit := opts.Limit
	if limit > 0 && limit < len(tasks) {
		tasks = tasks[:limit]
	}

	return tasks, nil
}

// estimateFetchCost estimates the cost (in milliseconds) to fetch a key from origin.
// This is a heuristic based on key patterns and can be refined with actual metrics.
func estimateFetchCost(key string) int {
	// Base cost
	cost := 50

	// Longer keys might indicate more complex data
	if len(key) > 50 {
		cost += 20
	}

	// Keys with multiple segments might require joins/aggregations
	depth := keyDepth(key)
	cost += depth * 10

	// Special cases (can be extended based on patterns)
	// Example: user profiles are fast, reports are slow
	if containsPattern(key, "report") {
		cost += 100
	}
	if containsPattern(key, "analytics") {
		cost += 150
	}

	return cost
}

// containsPattern checks if a key contains a specific pattern.
func containsPattern(key, pattern string) bool {
	return len(key) >= len(pattern) && contains(key, pattern)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}