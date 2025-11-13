package cachemanager

import (
	"time"
)

// EvictionPolicy defines the interface for cache eviction strategies.
type EvictionPolicy interface {
	// ShouldEvict returns true if an entry should be evicted based on policy.
	ShouldEvict(entry *CacheEntry, now time.Time) bool
	// OnAccess is called when an entry is accessed (for LRU ordering).
	OnAccess(key string)
	// OnSet is called when an entry is created/updated.
	OnSet(key string, value interface{}, ttl time.Duration)
}

// TTLPolicy implements time-to-live based eviction.
type TTLPolicy struct{}

// NewTTLPolicy creates a new TTL-based eviction policy.
func NewTTLPolicy() *TTLPolicy {
	return &TTLPolicy{}
}

func (p *TTLPolicy) ShouldEvict(entry *CacheEntry, now time.Time) bool {
	return now.After(entry.ExpiresAt)
}

func (p *TTLPolicy) OnAccess(key string) {
}

// OnSet is a no-op for TTL policy.
func (p *TTLPolicy) OnSet(key string, value interface{}, ttl time.Duration) {
}

// LRUPolicy implements least-recently-used eviction.
// This is implicitly handled by the L1Cache's internal LRU list,
// but this interface allows for future policy extensions.
type LRUPolicy struct {
}

func NewLRUPolicy() *LRUPolicy {
	return &LRUPolicy{}
}

func (p *LRUPolicy) ShouldEvict(entry *CacheEntry, now time.Time) bool {
	// This method exists for interface compliance
	return false
}

// OnAccess updates LRU ordering (handled by L1Cache.Get moving to front).
func (p *LRUPolicy) OnAccess(key string) {
	// Handled by L1Cache internal list operations
}

// OnSet updates LRU ordering (handled by L1Cache.Set).
func (p *LRUPolicy) OnSet(key string, value interface{}, ttl time.Duration) {
	// Handled by L1Cache internal list operations
}

// CombinedPolicy applies both TTL and LRU eviction.
// Entries are evicted if TTL expires OR if LRU eviction is needed at capacity.
type CombinedPolicy struct {
	ttl *TTLPolicy
	lru *LRUPolicy
}

// NewCombinedPolicy creates a policy that combines TTL and LRU.
func NewCombinedPolicy() *CombinedPolicy {
	return &CombinedPolicy{
		ttl: NewTTLPolicy(),
		lru: NewLRUPolicy(),
	}
}

// ShouldEvict returns true if either TTL expired or LRU eviction needed.
func (p *CombinedPolicy) ShouldEvict(entry *CacheEntry, now time.Time) bool {
	return p.ttl.ShouldEvict(entry, now)
}

// OnAccess updates both policies.
func (p *CombinedPolicy) OnAccess(key string) {
	p.lru.OnAccess(key)
}

// OnSet updates both policies.
func (p *CombinedPolicy) OnSet(key string, value interface{}, ttl time.Duration) {
	p.ttl.OnSet(key, value, ttl)
	p.lru.OnSet(key, value, ttl)
}

// PolicyEngine manages eviction policy application.
type PolicyEngine struct {
	policy EvictionPolicy
}

// NewPolicyEngine creates an engine with the specified policy.
func NewPolicyEngine(policy EvictionPolicy) *PolicyEngine {
	return &PolicyEngine{policy: policy}
}

// DefaultPolicyEngine returns an engine with combined TTL+LRU policy.
func DefaultPolicyEngine() *PolicyEngine {
	return &PolicyEngine{policy: NewCombinedPolicy()}
}

// ShouldEvict evaluates whether an entry should be evicted.
func (e *PolicyEngine) ShouldEvict(entry *CacheEntry) bool {
	return e.policy.ShouldEvict(entry, time.Now())
}

// RecordAccess notifies policy of entry access.
func (e *PolicyEngine) RecordAccess(key string) {
	e.policy.OnAccess(key)
}

// RecordSet notifies policy of entry creation/update.
func (e *PolicyEngine) RecordSet(key string, value interface{}, ttl time.Duration) {
	e.policy.OnSet(key, value, ttl)
}