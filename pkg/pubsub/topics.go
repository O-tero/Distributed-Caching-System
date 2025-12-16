// Package pubsub provides topic names and event type definitions for the
// distributed caching system's event-driven architecture.
//
// Topic Naming Convention:
//   - cache.invalidate: Cache invalidation events (key/pattern-based)
//   - cache.refresh: Cache refresh/reload events
//   - cache.warm.completed: Cache warming completion events
//
// Design Notes:
//   - Topics are defined as constants to avoid typos and enable compile-time checks
//   - Version field in events enables schema evolution without breaking consumers
//   - No direct Encore dependencies to keep pkg/ reusable across services
package pubsub

// Topic name constants for Encore Pub/Sub integration.
// These should be used when defining pubsub.Topic[T] in service code.
const (
	// TopicCacheInvalidate is published when cache entries need invalidation.
	// Event type: InvalidationEvent
	// Publishers: cache-manager, invalidation service, external triggers
	// Subscribers: All cache-manager instances
	TopicCacheInvalidate = "cache.invalidate"

	// TopicCacheRefresh is published when cache entries should be refreshed.
	// Event type: RefreshEvent
	// Publishers: warming service, scheduled jobs
	// Subscribers: cache-manager instances
	TopicCacheRefresh = "cache.refresh"

	// TopicCacheWarmCompleted is published when cache warming completes.
	// Event type: WarmCompletedEvent
	// Publishers: warming service
	// Subscribers: monitoring service, admin dashboard
	TopicCacheWarmCompleted = "cache.warm.completed"
)

// AllTopics returns all defined topic names.
// Useful for validation, testing, and administrative tools.
func AllTopics() []string {
	return []string{
		TopicCacheInvalidate,
		TopicCacheRefresh,
		TopicCacheWarmCompleted,
	}
}

// IsValidTopic checks if the given topic name is recognized.
func IsValidTopic(topic string) bool {
	for _, t := range AllTopics() {
		if t == topic {
			return true
		}
	}
	return false
}

// TopicMetadata provides descriptive information about topics.
type TopicMetadata struct {
	Name        string
	Description string
	EventType   string
}

// GetTopicMetadata returns metadata for all topics.
// Useful for documentation generation and admin UIs.
func GetTopicMetadata() []TopicMetadata {
	return []TopicMetadata{
		{
			Name:        TopicCacheInvalidate,
			Description: "Cache invalidation events for key or pattern-based clearing",
			EventType:   "InvalidationEvent",
		},
		{
			Name:        TopicCacheRefresh,
			Description: "Cache refresh events to reload specific entries",
			EventType:   "RefreshEvent",
		},
		{
			Name:        TopicCacheWarmCompleted,
			Description: "Cache warming completion notifications with status",
			EventType:   "WarmCompletedEvent",
		},
	}
}