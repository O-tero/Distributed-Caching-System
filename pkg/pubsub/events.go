package pubsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Event versioning strategy:
// - Version 1: Initial schema
// - Future versions: Add fields, never remove (backward compatible)
// - Consumers should check Version and handle appropriately

const (
	// EventVersion1 is the current event schema version
	EventVersion1 = 1
)

// InvalidationEvent represents a cache invalidation request.
// This event is published to TopicCacheInvalidate.
//
// Invalidation modes:
//   - Exact keys: Provide Keys slice
//   - Pattern-based: Provide Pattern (e.g., "users:*")
//   - Combination: Both Keys and Pattern can be set
//
// Design notes:
//   - Keys and Pattern are optional but at least one must be set
//   - Service field enables audit trail and debugging
//   - RequestID enables distributed tracing
type InvalidationEvent struct {
	// Version of the event schema (for backward compatibility)
	Version int `json:"version"`

	// Service that triggered the invalidation (e.g., "cache-manager", "api-gateway")
	Service string `json:"service"`

	// Keys to invalidate (exact match). Can be empty if Pattern is set.
	Keys []string `json:"keys,omitempty"`

	// Pattern for wildcard invalidation (e.g., "users:*"). Optional.
	Pattern string `json:"pattern,omitempty"`

	// TriggeredAt is the time the invalidation was requested
	TriggeredAt time.Time `json:"triggered_at"`

	// Meta contains optional metadata (e.g., reason, user_id)
	Meta map[string]string `json:"meta,omitempty"`

	// RequestID for distributed tracing and correlation
	RequestID string `json:"request_id"`
}

// Validate checks if the InvalidationEvent is well-formed.
func (e *InvalidationEvent) Validate() error {
	if e.Version != EventVersion1 {
		return fmt.Errorf("unsupported event version: %d", e.Version)
	}

	if e.Service == "" {
		return errors.New("service field is required")
	}

	if len(e.Keys) == 0 && e.Pattern == "" {
		return errors.New("at least one of keys or pattern must be set")
	}

	if e.TriggeredAt.IsZero() {
		return errors.New("triggered_at cannot be zero")
	}

	if e.RequestID == "" {
		return errors.New("request_id is required for tracing")
	}

	return nil
}

// ToJSON serializes the event to JSON.
func (e *InvalidationEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// InvalidationEventFromJSON deserializes an InvalidationEvent from JSON.
func InvalidationEventFromJSON(data []byte) (*InvalidationEvent, error) {
	var e InvalidationEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("failed to unmarshal InvalidationEvent: %w", err)
	}
	return &e, nil
}

// RefreshEvent represents a cache refresh request.
// This event is published to TopicCacheRefresh.
//
// Use cases:
//   - Scheduled refresh of hot keys
//   - Pre-warming after deployment
//   - Proactive reload of expiring entries
type RefreshEvent struct {
	// Version of the event schema
	Version int `json:"version"`

	// Service that triggered the refresh
	Service string `json:"service"`

	// Keys to refresh. Cannot be empty.
	Keys []string `json:"keys"`

	// Priority of the refresh (higher = more urgent). Default: 0
	Priority int `json:"priority"`

	// TriggeredAt is the time the refresh was requested
	TriggeredAt time.Time `json:"triggered_at"`

	// Meta contains optional metadata (e.g., "source=cron", "batch_id=123")
	Meta map[string]string `json:"meta,omitempty"`

	// RequestID for distributed tracing
	RequestID string `json:"request_id"`
}

// Validate checks if the RefreshEvent is well-formed.
func (e *RefreshEvent) Validate() error {
	if e.Version != EventVersion1 {
		return fmt.Errorf("unsupported event version: %d", e.Version)
	}

	if e.Service == "" {
		return errors.New("service field is required")
	}

	if len(e.Keys) == 0 {
		return errors.New("keys cannot be empty")
	}

	if e.TriggeredAt.IsZero() {
		return errors.New("triggered_at cannot be zero")
	}

	if e.RequestID == "" {
		return errors.New("request_id is required for tracing")
	}

	return nil
}

// ToJSON serializes the event to JSON.
func (e *RefreshEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// RefreshEventFromJSON deserializes a RefreshEvent from JSON.
func RefreshEventFromJSON(data []byte) (*RefreshEvent, error) {
	var e RefreshEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RefreshEvent: %w", err)
	}
	return &e, nil
}

// WarmCompletedEvent represents the completion of a cache warming operation.
// This event is published to TopicCacheWarmCompleted.
//
// Use cases:
//   - Notify monitoring of warming completion
//   - Trigger downstream processes after cache is ready
//   - Track warming performance and failures
type WarmCompletedEvent struct {
	// Version of the event schema
	Version int `json:"version"`

	// Service that performed the warming (typically "warming")
	Service string `json:"service"`

	// Status of the warming operation ("success", "partial", "failed")
	Status string `json:"status"`

	// Duration of the warming operation
	Duration time.Duration `json:"duration"`

	// KeysWarmed is the number of keys successfully warmed
	KeysWarmed int `json:"keys_warmed"`

	// KeysFailed is the number of keys that failed to warm
	KeysFailed int `json:"keys_failed"`

	// Error message if Status is "failed" or "partial"
	Error string `json:"error,omitempty"`

	// CompletedAt is the time the warming completed
	CompletedAt time.Time `json:"completed_at"`

	// Meta contains optional metadata (e.g., "batch_id", "source")
	Meta map[string]string `json:"meta,omitempty"`

	// RequestID for distributed tracing
	RequestID string `json:"request_id"`
}

// Validate checks if the WarmCompletedEvent is well-formed.
func (e *WarmCompletedEvent) Validate() error {
	if e.Version != EventVersion1 {
		return fmt.Errorf("unsupported event version: %d", e.Version)
	}

	if e.Service == "" {
		return errors.New("service field is required")
	}

	validStatuses := map[string]bool{"success": true, "partial": true, "failed": true}
	if !validStatuses[e.Status] {
		return fmt.Errorf("invalid status: %s (must be success, partial, or failed)", e.Status)
	}

	if e.Duration < 0 {
		return errors.New("duration cannot be negative")
	}

	if e.KeysWarmed < 0 || e.KeysFailed < 0 {
		return errors.New("keys_warmed and keys_failed cannot be negative")
	}

	if e.CompletedAt.IsZero() {
		return errors.New("completed_at cannot be zero")
	}

	if e.RequestID == "" {
		return errors.New("request_id is required for tracing")
	}

	return nil
}

// ToJSON serializes the event to JSON.
func (e *WarmCompletedEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// WarmCompletedEventFromJSON deserializes a WarmCompletedEvent from JSON.
func WarmCompletedEventFromJSON(data []byte) (*WarmCompletedEvent, error) {
	var e WarmCompletedEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("failed to unmarshal WarmCompletedEvent: %w", err)
	}
	return &e, nil
}