package cachemanager

import (
	"context"
	"time"

	"encore.dev/pubsub"
)

// InvalidateEvent represents a cache invalidation broadcast to all instances.
type InvalidateEvent struct {
	Keys      []string  `json:"keys"`      // Specific keys to invalidate
	Pattern   string    `json:"pattern"`   // Pattern for wildcard invalidation (e.g., "user:*")
	Timestamp time.Time `json:"timestamp"` // When invalidation was triggered
	Source    string    `json:"source"`    // Which instance triggered invalidation
}

// RefreshEvent represents a cache refresh command broadcast to all instances.
type RefreshEvent struct {
	Key       string      `json:"key"`        // Key to refresh
	Value     interface{} `json:"value"`      // New value to cache
	TTL       int         `json:"ttl"`        // TTL in seconds
	Timestamp time.Time   `json:"timestamp"`  // When refresh was triggered
	Priority  string      `json:"priority"`   // "critical", "high", "normal"
}

// Pub/Sub topic definitions for cache coordination.
var CacheInvalidateTopic = pubsub.NewTopic[*InvalidateEvent](
	"cache-invalidate",
	pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	},
)

var CacheRefreshTopic = pubsub.NewTopic[*RefreshEvent](
	"cache-refresh",
	pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	},
)

// Subscribe to cache invalidation events from other instances.
// This ensures eventual consistency across all cache-manager instances.
var _ = pubsub.NewSubscription(
	CacheInvalidateTopic,
	"cache-manager-invalidate",
	pubsub.SubscriptionConfig[*InvalidateEvent]{
		Handler: HandleInvalidateEvent,
	},
)

// HandleInvalidateEvent processes invalidation events from other cache instances.
// This handler is triggered when any instance publishes an invalidation event.
func HandleInvalidateEvent(ctx context.Context, event *InvalidateEvent) error {
	if svc == nil {
		return nil // Service not initialized yet
	}

	// Invalidate specific keys
	for _, key := range event.Keys {
		svc.l1Cache.Delete(key)
		svc.metrics.Deletes.Add(1)
	}

	// Invalidate by pattern
	if event.Pattern != "" {
		deleted := svc.l1Cache.DeletePattern(event.Pattern)
		svc.metrics.Deletes.Add(int64(deleted))
	}

	return nil
}

// Subscribe to cache refresh events from warming service.
var _ = pubsub.NewSubscription(
	CacheRefreshTopic,
	"cache-manager-refresh",
	pubsub.SubscriptionConfig[*RefreshEvent]{
		Handler: HandleRefreshEvent,
	},
)

// HandleRefreshEvent processes cache refresh events from warming service.
// This proactively populates the cache with fresh data.
func HandleRefreshEvent(ctx context.Context, event *RefreshEvent) error {
	if svc == nil {
		return nil 
	}

	ttl := time.Duration(event.TTL) * time.Second
	if ttl == 0 {
		ttl = svc.config.DefaultTTL
	}

	svc.l1Cache.Set(event.Key, event.Value, ttl)

	if svc.config.L2Enabled && svc.l2Cache != nil {
		go func() {
			entry := CacheEntry{
				Value:     event.Value,
				CachedAt:  time.Now(),
				ExpiresAt: time.Now().Add(ttl),
			}
			// Serialize and store (implementation depends on L2 provider)
			_ = entry // TODO: implement L2 storage
		}()
	}

	return nil
}

// PublishInvalidation publishes an invalidation event to all instances.
// This is called internally after local invalidation to coordinate with other nodes.
func (s *Service) PublishInvalidation(ctx context.Context, keys []string, pattern string) error {
	event := &InvalidateEvent{
		Keys:      keys,
		Pattern:   pattern,
		Timestamp: time.Now(),
		Source:    "cache-manager", // Could be instance ID in production
	}
	_, err := CacheInvalidateTopic.Publish(ctx, event)
	return err
}
// PublishRefresh publishes a refresh event to all instances.
// This is called by warming service to proactively populate caches.
func (s *Service) PublishRefresh(ctx context.Context, key string, value interface{}, ttl int) error {
	event := &RefreshEvent{
		Key:       key,
		Value:     value,
		TTL:       ttl,
		Timestamp: time.Now(),
		Priority:  "normal",
	}
	_, err := CacheRefreshTopic.Publish(ctx, event)
	return err
}