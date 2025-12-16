// Package utils provides utility functions for the distributed caching system.
//
// This file implements a consistent hashing ring with virtual nodes.
// Consistent hashing minimizes key redistribution when nodes are added/removed.
//
// Design Notes:
//   - Uses FNV-1a 64-bit hash (stdlib, fast, good distribution)
//   - Virtual nodes (replicas) improve load distribution
//   - Thread-safe via sync.RWMutex
//   - O(log M) lookup complexity where M = total virtual nodes
//   - Sorted ring positions for binary search
//
// Trade-offs:
//   - Memory: O(N * replicas) where N = number of nodes
//   - CPU: AddNode/RemoveNode O(replicas * log M), GetNode O(log M)
//   - Distribution uniformity improves with more replicas (default: 150)
//
// Production extensions:
//   - Consider xxhash for 2x faster hashing (requires external dep)
//   - Add weighted nodes via replica count per node
//   - Implement bounded jump consistent hashing for cache-only use cases
package utils

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

// DefaultReplicas is the default number of virtual nodes per physical node.
// More replicas = better distribution but more memory and slower add/remove.
const DefaultReplicas = 150

// HashRing implements a consistent hashing ring with virtual nodes.
//
// Example usage:
//
//	ring := NewHashRing(150)
//	ring.AddNode("cache-1", 1)
//	ring.AddNode("cache-2", 1)
//	ring.AddNode("cache-3", 1)
//
//	node := ring.GetNode("user:12345")  // Returns "cache-2"
//	nodes := ring.GetNNodes("user:12345", 2)  // Returns ["cache-2", "cache-1"]
type HashRing struct {
	mu       sync.RWMutex
	replicas int
	keys     []uint64          // Sorted ring positions
	ring     map[uint64]string // Hash -> node ID mapping
	nodes    map[string]int    // Node ID -> weight mapping
}

// NewHashRing creates a new consistent hash ring.
// replicas determines the number of virtual nodes per physical node.
// Use 0 for default (150 replicas).
func NewHashRing(replicas int) *HashRing {
	if replicas <= 0 {
		replicas = DefaultReplicas
	}

	return &HashRing{
		replicas: replicas,
		ring:     make(map[uint64]string),
		nodes:    make(map[string]int),
	}
}

// AddNode adds a node to the ring with the given weight.
// Weight determines the number of virtual nodes (replicas * weight).
// Weight must be > 0 (default: 1).
//
// Complexity: O(replicas * weight * log M) where M = total virtual nodes
func (h *HashRing) AddNode(nodeID string, weight int) error {
	if nodeID == "" {
		return fmt.Errorf("nodeID cannot be empty")
	}
	if weight <= 0 {
		weight = 1
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Update or insert node
	h.nodes[nodeID] = weight

	// Add virtual nodes to ring
	virtualNodes := h.replicas * weight
	for i := 0; i < virtualNodes; i++ {
		hash := h.hashKey(fmt.Sprintf("%s:%d", nodeID, i))
		h.ring[hash] = nodeID
		h.keys = append(h.keys, hash)
	}

	// Re-sort ring positions
	sort.Slice(h.keys, func(i, j int) bool {
		return h.keys[i] < h.keys[j]
	})

	return nil
}

// RemoveNode removes a node from the ring.
// Returns error if node doesn't exist.
//
// Complexity: O(replicas * weight * log M)
func (h *HashRing) RemoveNode(nodeID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	weight, exists := h.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// Remove virtual nodes from ring
	virtualNodes := h.replicas * weight
	for i := 0; i < virtualNodes; i++ {
		hash := h.hashKey(fmt.Sprintf("%s:%d", nodeID, i))
		delete(h.ring, hash)
	}

	// Rebuild keys slice (remove deleted hashes)
	newKeys := make([]uint64, 0, len(h.ring))
	for hash := range h.ring {
		newKeys = append(newKeys, hash)
	}
	sort.Slice(newKeys, func(i, j int) bool {
		return newKeys[i] < newKeys[j]
	})
	h.keys = newKeys

	delete(h.nodes, nodeID)
	return nil
}

// GetNode returns the node responsible for the given key.
// Returns empty string if ring is empty.
//
// Complexity: O(log M) where M = total virtual nodes
func (h *HashRing) GetNode(key string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.keys) == 0 {
		return ""
	}

	hash := h.hashKey(key)

	// Binary search for first node >= hash
	idx := sort.Search(len(h.keys), func(i int) bool {
		return h.keys[i] >= hash
	})

	// Wrap around if we're past the end
	if idx == len(h.keys) {
		idx = 0
	}

	return h.ring[h.keys[idx]]
}

// GetNNodes returns the N nodes responsible for the given key.
// Useful for replication (primary + replicas).
// Returns up to N unique nodes. May return fewer if N > node count.
//
// Complexity: O(N * log M)
func (h *HashRing) GetNNodes(key string, n int) []string {
	if n <= 0 {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.keys) == 0 {
		return nil
	}

	hash := h.hashKey(key)

	// Find starting position
	idx := sort.Search(len(h.keys), func(i int) bool {
		return h.keys[i] >= hash
	})
	if idx == len(h.keys) {
		idx = 0
	}

	// Collect unique nodes
	seen := make(map[string]bool)
	result := make([]string, 0, n)

	for i := 0; i < len(h.keys) && len(result) < n; i++ {
		pos := (idx + i) % len(h.keys)
		nodeID := h.ring[h.keys[pos]]

		if !seen[nodeID] {
			seen[nodeID] = true
			result = append(result, nodeID)
		}
	}

	return result
}

// Nodes returns all node IDs currently in the ring.
func (h *HashRing) Nodes() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nodes := make([]string, 0, len(h.nodes))
	for nodeID := range h.nodes {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

// Size returns the number of physical nodes in the ring.
func (h *HashRing) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodes)
}

// hashKey computes FNV-1a 64-bit hash of the key.
// FNV-1a is fast and provides good distribution for hash tables.
//
// Performance: ~150ns per key on modern CPUs
// Alternative: xxhash is 2x faster but requires external dependency
func (h *HashRing) hashKey(key string) uint64 {
	hasher := fnv.New64a()
	hasher.Write([]byte(key))
	return hasher.Sum64()
}