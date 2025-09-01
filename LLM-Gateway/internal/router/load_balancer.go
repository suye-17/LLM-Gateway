// Package router implements load balancing strategies
package router

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-gateway/gateway/pkg/types"
)

// LoadBalancer defines the interface for load balancing strategies
type LoadBalancer interface {
	SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error)
}

// RoundRobinBalancer implements round-robin load balancing
type RoundRobinBalancer struct {
	counter uint64
}

// NewRoundRobinBalancer creates a new round-robin load balancer
func NewRoundRobinBalancer() LoadBalancer {
	return &RoundRobinBalancer{}
}

// SelectProvider selects a provider using round-robin algorithm
func (b *RoundRobinBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	index := atomic.AddUint64(&b.counter, 1) % uint64(len(providers))
	return providers[index], nil
}

// WeightedRoundRobinBalancer implements weighted round-robin load balancing
type WeightedRoundRobinBalancer struct {
	weights  map[string]int
	counters map[string]int
	mu       sync.Mutex
}

// NewWeightedRoundRobinBalancer creates a new weighted round-robin load balancer
func NewWeightedRoundRobinBalancer() LoadBalancer {
	return &WeightedRoundRobinBalancer{
		weights:  make(map[string]int),
		counters: make(map[string]int),
	}
}

// SelectProvider selects a provider using weighted round-robin algorithm
func (b *WeightedRoundRobinBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Initialize weights if not set (default to 100)
	for _, provider := range providers {
		name := provider.GetName()
		if _, exists := b.weights[name]; !exists {
			b.weights[name] = 100 // Default weight
		}
		if _, exists := b.counters[name]; !exists {
			b.counters[name] = 0
		}
	}

	// Find provider with highest weight/counter ratio
	var selectedProvider types.Provider
	maxRatio := -1.0

	for _, provider := range providers {
		name := provider.GetName()
		weight := b.weights[name]
		counter := b.counters[name]

		ratio := float64(weight) / float64(counter+1)
		if ratio > maxRatio {
			maxRatio = ratio
			selectedProvider = provider
		}
	}

	// Increment counter for selected provider
	if selectedProvider != nil {
		b.counters[selectedProvider.GetName()]++
	}

	return selectedProvider, nil
}

// LeastLatencyBalancer selects provider with lowest latency
type LeastLatencyBalancer struct {
	stats *RoutingStats
}

// NewLeastLatencyBalancer creates a new least-latency load balancer
func NewLeastLatencyBalancer(stats *RoutingStats) LoadBalancer {
	return &LeastLatencyBalancer{stats: stats}
}

// SelectProvider selects the provider with the lowest average latency
func (b *LeastLatencyBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	b.stats.mu.RLock()
	defer b.stats.mu.RUnlock()

	var bestProvider types.Provider
	var minLatency time.Duration = time.Duration(1<<63 - 1) // Max duration

	for _, provider := range providers {
		name := provider.GetName()
		if latencyStats, exists := b.stats.LatencyStats[name]; exists && latencyStats.Count > 0 {
			if latencyStats.Average < minLatency {
				minLatency = latencyStats.Average
				bestProvider = provider
			}
		} else {
			// If no stats available, consider this provider (for cold start)
			if bestProvider == nil {
				bestProvider = provider
			}
		}
	}

	if bestProvider == nil {
		bestProvider = providers[0] // Fallback to first provider
	}

	return bestProvider, nil
}

// CostOptimizedBalancer selects provider with lowest cost
type CostOptimizedBalancer struct{}

// NewCostOptimizedBalancer creates a new cost-optimized load balancer
func NewCostOptimizedBalancer() LoadBalancer {
	return &CostOptimizedBalancer{}
}

// SelectProvider selects the provider with the lowest estimated cost
func (b *CostOptimizedBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	var bestProvider types.Provider
	var minCost float64 = math.MaxFloat64

	for _, provider := range providers {
		if cost, err := provider.EstimateCost(req); err == nil {
			if cost.TotalCost < minCost {
				minCost = cost.TotalCost
				bestProvider = provider
			}
		}
	}

	if bestProvider == nil {
		bestProvider = providers[0] // Fallback to first provider
	}

	return bestProvider, nil
}

// LeastConnectionsBalancer selects provider with fewest active connections
type LeastConnectionsBalancer struct {
	connections map[string]int64
	mu          sync.RWMutex
}

// NewLeastConnectionsBalancer creates a new least-connections load balancer
func NewLeastConnectionsBalancer() LoadBalancer {
	return &LeastConnectionsBalancer{
		connections: make(map[string]int64),
	}
}

// SelectProvider selects the provider with the fewest active connections
func (b *LeastConnectionsBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	var bestProvider types.Provider
	var minConnections int64 = math.MaxInt64

	for _, provider := range providers {
		name := provider.GetName()
		connections := b.connections[name]
		if connections < minConnections {
			minConnections = connections
			bestProvider = provider
		}
	}

	if bestProvider == nil {
		bestProvider = providers[0] // Fallback to first provider
	}

	return bestProvider, nil
}

// IncrementConnections increments the connection count for a provider
func (b *LeastConnectionsBalancer) IncrementConnections(providerName string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connections[providerName]++
}

// DecrementConnections decrements the connection count for a provider
func (b *LeastConnectionsBalancer) DecrementConnections(providerName string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.connections[providerName] > 0 {
		b.connections[providerName]--
	}
}

// RandomBalancer selects providers randomly
type RandomBalancer struct {
	rand *rand.Rand
	mu   sync.Mutex
}

// NewRandomBalancer creates a new random load balancer
func NewRandomBalancer() LoadBalancer {
	return &RandomBalancer{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SelectProvider selects a provider randomly
func (b *RandomBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	index := b.rand.Intn(len(providers))
	return providers[index], nil
}

// StickySessionBalancer maintains session affinity
type StickySessionBalancer struct {
	sessions map[string]string // userID -> providerName
	fallback LoadBalancer
	mu       sync.RWMutex
}

// NewStickySessionBalancer creates a new sticky session load balancer
func NewStickySessionBalancer(fallback LoadBalancer) LoadBalancer {
	return &StickySessionBalancer{
		sessions: make(map[string]string),
		fallback: fallback,
	}
}

// SelectProvider selects a provider maintaining session affinity
func (b *StickySessionBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	// Extract user ID from request (if available)
	userID := ""
	if req.User != nil {
		userID = *req.User
	}

	if userID != "" {
		b.mu.RLock()
		if providerName, exists := b.sessions[userID]; exists {
			// Check if the provider is still available
			for _, provider := range providers {
				if provider.GetName() == providerName {
					b.mu.RUnlock()
					return provider, nil
				}
			}
		}
		b.mu.RUnlock()
	}

	// No existing session or provider not available, use fallback
	provider, err := b.fallback.SelectProvider(providers, req)
	if err != nil {
		return nil, err
	}

	// Store session mapping
	if userID != "" {
		b.mu.Lock()
		b.sessions[userID] = provider.GetName()
		b.mu.Unlock()
	}

	return provider, nil
}

// ClearSession removes a session mapping
func (b *StickySessionBalancer) ClearSession(userID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, userID)
}

// ConsistentHashBalancer implements consistent hashing for load balancing
type ConsistentHashBalancer struct {
	hashRing *HashRing
}

// NewConsistentHashBalancer creates a new consistent hash load balancer
func NewConsistentHashBalancer() LoadBalancer {
	return &ConsistentHashBalancer{
		hashRing: NewHashRing(),
	}
}

// SelectProvider selects a provider using consistent hashing
func (b *ConsistentHashBalancer) SelectProvider(providers []types.Provider, req *types.ChatCompletionRequest) (types.Provider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	// Update hash ring with current providers
	providerNames := make([]string, len(providers))
	for i, provider := range providers {
		providerNames[i] = provider.GetName()
	}
	b.hashRing.UpdateNodes(providerNames)

	// Create key from request (use model + first message for consistency)
	key := req.Model
	if len(req.Messages) > 0 {
		key += req.Messages[0].Content
	}

	selectedName := b.hashRing.GetNode(key)
	for _, provider := range providers {
		if provider.GetName() == selectedName {
			return provider, nil
		}
	}

	// Fallback to first provider if hash ring lookup fails
	return providers[0], nil
}
