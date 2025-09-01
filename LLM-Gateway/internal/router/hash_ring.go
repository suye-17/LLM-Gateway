// Package router implements consistent hashing for load balancing
package router

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"sync"
)

// HashRing implements a consistent hash ring
type HashRing struct {
	nodes        map[uint32]string // hash -> node name
	sortedKeys   []uint32          // sorted hash values
	virtualNodes int               // number of virtual nodes per physical node
	mu           sync.RWMutex
}

// NewHashRing creates a new hash ring
func NewHashRing() *HashRing {
	return &HashRing{
		nodes:        make(map[uint32]string),
		virtualNodes: 150, // Good balance between distribution and memory usage
	}
}

// NewHashRingWithVirtualNodes creates a new hash ring with specified virtual nodes
func NewHashRingWithVirtualNodes(virtualNodes int) *HashRing {
	return &HashRing{
		nodes:        make(map[uint32]string),
		virtualNodes: virtualNodes,
	}
}

// AddNode adds a node to the hash ring
func (hr *HashRing) AddNode(node string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	// Add virtual nodes
	for i := 0; i < hr.virtualNodes; i++ {
		virtualNodeKey := fmt.Sprintf("%s:%d", node, i)
		hash := hr.hashKey(virtualNodeKey)
		hr.nodes[hash] = node
	}

	hr.updateSortedKeys()
}

// RemoveNode removes a node from the hash ring
func (hr *HashRing) RemoveNode(node string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	// Remove virtual nodes
	for i := 0; i < hr.virtualNodes; i++ {
		virtualNodeKey := fmt.Sprintf("%s:%d", node, i)
		hash := hr.hashKey(virtualNodeKey)
		delete(hr.nodes, hash)
	}

	hr.updateSortedKeys()
}

// GetNode returns the node responsible for the given key
func (hr *HashRing) GetNode(key string) string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.sortedKeys) == 0 {
		return ""
	}

	hash := hr.hashKey(key)

	// Find the first node with hash >= key hash (clockwise)
	idx := sort.Search(len(hr.sortedKeys), func(i int) bool {
		return hr.sortedKeys[i] >= hash
	})

	// If no such node found, wrap around to the first node
	if idx == len(hr.sortedKeys) {
		idx = 0
	}

	return hr.nodes[hr.sortedKeys[idx]]
}

// GetNodes returns multiple nodes for the given key (for replication)
func (hr *HashRing) GetNodes(key string, count int) []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.sortedKeys) == 0 || count <= 0 {
		return nil
	}

	hash := hr.hashKey(key)

	// Find the first node with hash >= key hash
	idx := sort.Search(len(hr.sortedKeys), func(i int) bool {
		return hr.sortedKeys[i] >= hash
	})

	// If no such node found, wrap around to the first node
	if idx == len(hr.sortedKeys) {
		idx = 0
	}

	seen := make(map[string]bool)
	result := make([]string, 0, count)

	// Collect unique nodes (virtual nodes may map to same physical node)
	for len(result) < count && len(seen) < len(hr.getUniqueNodes()) {
		node := hr.nodes[hr.sortedKeys[idx]]
		if !seen[node] {
			seen[node] = true
			result = append(result, node)
		}
		idx = (idx + 1) % len(hr.sortedKeys)
	}

	return result
}

// UpdateNodes updates the hash ring with a new set of nodes
func (hr *HashRing) UpdateNodes(nodes []string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	// Clear existing nodes
	hr.nodes = make(map[uint32]string)

	// Add new nodes
	for _, node := range nodes {
		for i := 0; i < hr.virtualNodes; i++ {
			virtualNodeKey := fmt.Sprintf("%s:%d", node, i)
			hash := hr.hashKey(virtualNodeKey)
			hr.nodes[hash] = node
		}
	}

	hr.updateSortedKeys()
}

// GetNodeDistribution returns the distribution of keys across nodes
func (hr *HashRing) GetNodeDistribution(sampleSize int) map[string]int {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	distribution := make(map[string]int)

	// Generate sample keys and see which nodes they map to
	for i := 0; i < sampleSize; i++ {
		sampleKey := fmt.Sprintf("sample_key_%d", i)
		node := hr.GetNode(sampleKey)
		if node != "" {
			distribution[node]++
		}
	}

	return distribution
}

// GetStats returns statistics about the hash ring
func (hr *HashRing) GetStats() HashRingStats {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	uniqueNodes := hr.getUniqueNodes()
	return HashRingStats{
		TotalVirtualNodes:       len(hr.nodes),
		PhysicalNodes:           len(uniqueNodes),
		VirtualNodesPerPhysical: hr.virtualNodes,
		Nodes:                   uniqueNodes,
	}
}

// HashRingStats contains statistics about the hash ring
type HashRingStats struct {
	TotalVirtualNodes       int      `json:"total_virtual_nodes"`
	PhysicalNodes           int      `json:"physical_nodes"`
	VirtualNodesPerPhysical int      `json:"virtual_nodes_per_physical"`
	Nodes                   []string `json:"nodes"`
}

// hashKey generates a hash for the given key
func (hr *HashRing) hashKey(key string) uint32 {
	h := sha1.New()
	h.Write([]byte(key))
	hash := h.Sum(nil)

	// Convert first 4 bytes to uint32
	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}

// updateSortedKeys updates the sorted keys slice
func (hr *HashRing) updateSortedKeys() {
	hr.sortedKeys = make([]uint32, 0, len(hr.nodes))
	for hash := range hr.nodes {
		hr.sortedKeys = append(hr.sortedKeys, hash)
	}
	sort.Slice(hr.sortedKeys, func(i, j int) bool {
		return hr.sortedKeys[i] < hr.sortedKeys[j]
	})
}

// getUniqueNodes returns a list of unique physical nodes
func (hr *HashRing) getUniqueNodes() []string {
	seen := make(map[string]bool)
	var unique []string

	for _, node := range hr.nodes {
		if !seen[node] {
			seen[node] = true
			unique = append(unique, node)
		}
	}

	return unique
}

// RendezvousHash implements rendezvous (highest random weight) hashing
// Alternative to consistent hashing that provides better distribution
type RendezvousHash struct {
	nodes []string
	mu    sync.RWMutex
}

// NewRendezvousHash creates a new rendezvous hash
func NewRendezvousHash() *RendezvousHash {
	return &RendezvousHash{}
}

// UpdateNodes updates the nodes list
func (rh *RendezvousHash) UpdateNodes(nodes []string) {
	rh.mu.Lock()
	defer rh.mu.Unlock()
	rh.nodes = make([]string, len(nodes))
	copy(rh.nodes, nodes)
}

// GetNode returns the node with highest weight for the given key
func (rh *RendezvousHash) GetNode(key string) string {
	rh.mu.RLock()
	defer rh.mu.RUnlock()

	if len(rh.nodes) == 0 {
		return ""
	}

	var bestNode string
	var maxWeight uint32

	for _, node := range rh.nodes {
		weight := rh.computeWeight(key, node)
		if weight > maxWeight {
			maxWeight = weight
			bestNode = node
		}
	}

	return bestNode
}

// GetNodes returns multiple nodes with highest weights
func (rh *RendezvousHash) GetNodes(key string, count int) []string {
	rh.mu.RLock()
	defer rh.mu.RUnlock()

	if len(rh.nodes) == 0 || count <= 0 {
		return nil
	}

	type nodeWeight struct {
		node   string
		weight uint32
	}

	weights := make([]nodeWeight, len(rh.nodes))
	for i, node := range rh.nodes {
		weights[i] = nodeWeight{
			node:   node,
			weight: rh.computeWeight(key, node),
		}
	}

	// Sort by weight descending
	sort.Slice(weights, func(i, j int) bool {
		return weights[i].weight > weights[j].weight
	})

	// Return top N nodes
	result := make([]string, 0, count)
	for i := 0; i < count && i < len(weights); i++ {
		result = append(result, weights[i].node)
	}

	return result
}

// computeWeight computes the weight for a key-node pair
func (rh *RendezvousHash) computeWeight(key, node string) uint32 {
	combined := key + node
	h := sha1.New()
	h.Write([]byte(combined))
	hash := h.Sum(nil)

	// Convert first 4 bytes to uint32
	return uint32(hash[0])<<24 | uint32(hash[1])<<16 | uint32(hash[2])<<8 | uint32(hash[3])
}
