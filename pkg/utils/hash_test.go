package utils

import (
	"fmt"
	"sync"
	"testing"
)

func TestHashRing_AddNode(t *testing.T) {
	ring := NewHashRing(10)

	err := ring.AddNode("node1", 1)
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	if ring.Size() != 1 {
		t.Errorf("Size() = %v, want 1", ring.Size())
	}

	// Add another node
	err = ring.AddNode("node2", 1)
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	if ring.Size() != 2 {
		t.Errorf("Size() = %v, want 2", ring.Size())
	}

	// Add with weight
	err = ring.AddNode("node3", 3)
	if err != nil {
		t.Fatalf("AddNode() error = %v", err)
	}

	if ring.Size() != 3 {
		t.Errorf("Size() = %v, want 3", ring.Size())
	}

	// Verify total virtual nodes: (10*1) + (10*1) + (10*3) = 50
	if len(ring.keys) != 50 {
		t.Errorf("Virtual nodes count = %v, want 50", len(ring.keys))
	}
}

func TestHashRing_AddNodeErrors(t *testing.T) {
	ring := NewHashRing(10)

	err := ring.AddNode("", 1)
	if err == nil {
		t.Error("AddNode() with empty nodeID should return error")
	}
}

func TestHashRing_RemoveNode(t *testing.T) {
	ring := NewHashRing(10)

	ring.AddNode("node1", 1)
	ring.AddNode("node2", 1)
	ring.AddNode("node3", 1)

	if ring.Size() != 3 {
		t.Fatalf("Size() = %v, want 3", ring.Size())
	}

	err := ring.RemoveNode("node2")
	if err != nil {
		t.Fatalf("RemoveNode() error = %v", err)
	}

	if ring.Size() != 2 {
		t.Errorf("Size() = %v, want 2", ring.Size())
	}

	// Try to remove non-existent node
	err = ring.RemoveNode("nonexistent")
	if err == nil {
		t.Error("RemoveNode() with non-existent node should return error")
	}

	// Verify virtual nodes count: 2 nodes * 10 replicas = 20
	if len(ring.keys) != 20 {
		t.Errorf("Virtual nodes count = %v, want 20", len(ring.keys))
	}
}

func TestHashRing_GetNode(t *testing.T) {
	ring := NewHashRing(100)

	// Empty ring
	node := ring.GetNode("key1")
	if node != "" {
		t.Errorf("GetNode() on empty ring = %v, want empty", node)
	}

	// Add nodes
	ring.AddNode("node1", 1)
	ring.AddNode("node2", 1)
	ring.AddNode("node3", 1)

	// Same key should always map to same node
	node1 := ring.GetNode("user:12345")
	node2 := ring.GetNode("user:12345")
	if node1 != node2 {
		t.Errorf("GetNode() inconsistent: %v != %v", node1, node2)
	}

	// Different keys should distribute across nodes
	keys := []string{"key1", "key2", "key3", "key4", "key5", "key6", "key7", "key8", "key9", "key10"}
	nodeCount := make(map[string]int)

	for _, key := range keys {
		node := ring.GetNode(key)
		nodeCount[node]++
	}

	// All nodes should get at least one key (probabilistic test)
	if len(nodeCount) < 2 {
		t.Errorf("Distribution too uneven: %v nodes used out of 3", len(nodeCount))
	}
}

func TestHashRing_GetNNodes(t *testing.T) {
	ring := NewHashRing(100)
	ring.AddNode("node1", 1)
	ring.AddNode("node2", 1)
	ring.AddNode("node3", 1)

	tests := []struct {
		name string
		key  string
		n    int
		want int // Expected number of unique nodes
	}{
		{"single replica", "key1", 1, 1},
		{"two replicas", "key1", 2, 2},
		{"three replicas", "key1", 3, 3},
		{"more than available", "key1", 5, 3}, // Only 3 nodes exist
		{"zero replicas", "key1", 0, 0},
		{"negative replicas", "key1", -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := ring.GetNNodes(tt.key, tt.n)
			if len(nodes) != tt.want {
				t.Errorf("GetNNodes() returned %v nodes, want %v", len(nodes), tt.want)
			}

			// Verify uniqueness
			seen := make(map[string]bool)
			for _, node := range nodes {
				if seen[node] {
					t.Errorf("GetNNodes() returned duplicate node: %v", node)
				}
				seen[node] = true
			}

			// Same key should return same ordered list
			nodes2 := ring.GetNNodes(tt.key, tt.n)
			if len(nodes) != len(nodes2) {
				t.Errorf("GetNNodes() inconsistent length")
			}
			for i := range nodes {
				if nodes[i] != nodes2[i] {
					t.Errorf("GetNNodes() inconsistent order at index %d", i)
				}
			}
		})
	}
}

func TestHashRing_Distribution(t *testing.T) {
	ring := NewHashRing(150) // More replicas = better distribution

	// Add 4 nodes
	nodes := []string{"node1", "node2", "node3", "node4"}
	for _, node := range nodes {
		ring.AddNode(node, 1)
	}

	// Generate 10000 keys and check distribution
	keyCount := 10000
	nodeCount := make(map[string]int)

	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("key:%d", i)
		node := ring.GetNode(key)
		nodeCount[node]++
	}

	// Calculate standard deviation of distribution
	avg := float64(keyCount) / float64(len(nodes))
	variance := 0.0
	for _, count := range nodeCount {
		diff := float64(count) - avg
		variance += diff * diff
	}
	variance /= float64(len(nodes))
	stddev := variance // Simplified

	// Each node should get roughly 25% ± 5% with 150 replicas
	for node, count := range nodeCount {
		percentage := float64(count) / float64(keyCount) * 100
		t.Logf("Node %s: %d keys (%.2f%%)", node, count, percentage)

		if percentage < 20 || percentage > 30 {
			t.Errorf("Node %s distribution %.2f%% is outside acceptable range [20%%, 30%%]", node, percentage)
		}
	}

	t.Logf("Distribution stddev: %.2f", stddev)
}

func TestHashRing_KeyRedistribution(t *testing.T) {
	ring := NewHashRing(100)

	// Add initial nodes
	ring.AddNode("node1", 1)
	ring.AddNode("node2", 1)

	// Map keys before adding new node
	keyCount := 1000
	beforeMapping := make(map[string]string)
	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("key:%d", i)
		beforeMapping[key] = ring.GetNode(key)
	}

	// Add new node
	ring.AddNode("node3", 1)

	// Map keys after adding new node
	movedKeys := 0
	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("key:%d", i)
		afterNode := ring.GetNode(key)
		if beforeMapping[key] != afterNode {
			movedKeys++
		}
	}

	// With consistent hashing, only ~33% of keys should move (1/3 to new node)
	movePercentage := float64(movedKeys) / float64(keyCount) * 100
	t.Logf("Keys moved after adding node: %d (%.2f%%)", movedKeys, movePercentage)

	// Should be between 25% and 40% (with some variance)
	if movePercentage < 20 || movePercentage > 45 {
		t.Errorf("Key redistribution %.2f%% is outside expected range [20%%, 45%%]", movePercentage)
	}
}

func TestHashRing_Concurrency(t *testing.T) {
	ring := NewHashRing(50)

	// Pre-populate
	for i := 0; i < 5; i++ {
		ring.AddNode(fmt.Sprintf("node%d", i), 1)
	}

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("key:%d:%d", id, j)
				node := ring.GetNode(key)
				if node == "" {
					t.Errorf("GetNode() returned empty string")
				}
			}
		}(i)
	}

	// Concurrent writes (add/remove)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := fmt.Sprintf("temp-node%d", id)
			ring.AddNode(nodeID, 1)
			ring.RemoveNode(nodeID)
		}(i)
	}

	wg.Wait()
}

func BenchmarkHashRing_GetNode(b *testing.B) {
	ring := NewHashRing(150)
	ring.AddNode("node1", 1)
	ring.AddNode("node2", 1)
	ring.AddNode("node3", 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key:%d", i%1000)
		ring.GetNode(key)
	}
}

func BenchmarkHashRing_GetNNodes(b *testing.B) {
	ring := NewHashRing(150)
	ring.AddNode("node1", 1)
	ring.AddNode("node2", 1)
	ring.AddNode("node3", 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key:%d", i%1000)
		ring.GetNNodes(key, 2)
	}
}

func BenchmarkHashRing_AddNode(b *testing.B) {
	ring := NewHashRing(150)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		ring.AddNode(nodeID, 1)
	}
}