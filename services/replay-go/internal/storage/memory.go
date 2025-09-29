package storage

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryBackend implements an in-memory replay buffer
type MemoryBackend struct {
	mu           sync.RWMutex
	transitions  map[string]*Transition // ID -> Transition
	episodes     map[string][]string    // EpisodeID -> TransitionIDs
	envIndex     map[string][]string    // EnvID -> TransitionIDs
	timeIndex    []string               // TransitionIDs sorted by timestamp
	maxSize      uint64                 // Maximum number of transitions to store
	rng          *rand.Rand
}

// NewMemoryBackend creates a new in-memory storage backend
func NewMemoryBackend(maxSize uint64) *MemoryBackend {
	return &MemoryBackend{
		transitions: make(map[string]*Transition),
		episodes:    make(map[string][]string),
		envIndex:    make(map[string][]string),
		timeIndex:   make([]string, 0),
		maxSize:     maxSize,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Store implements Backend.Store
func (m *MemoryBackend) Store(ctx context.Context, transition *Transition) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate ID if not provided
	if transition.ID == "" {
		transition.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if transition.Timestamp.IsZero() {
		transition.Timestamp = time.Now()
	}

	// Set default priority if not provided
	if transition.Priority == 0 {
		transition.Priority = 1.0
	}

	// Store the transition
	m.transitions[transition.ID] = transition

	// Update episode index
	if transition.EpisodeID != "" {
		m.episodes[transition.EpisodeID] = append(m.episodes[transition.EpisodeID], transition.ID)
	}

	// Update environment index
	if transition.EnvID != "" {
		m.envIndex[transition.EnvID] = append(m.envIndex[transition.EnvID], transition.ID)
	}

	// Update time index (maintain sorted order)
	m.insertInTimeIndex(transition.ID, transition.Timestamp)

	// Evict old transitions if we exceed maxSize
	m.evictIfNeeded()

	return nil
}

// StoreBatch implements Backend.StoreBatch
func (m *MemoryBackend) StoreBatch(ctx context.Context, transitions []*Transition) ([]string, error) {
	ids := make([]string, len(transitions))

	for i, transition := range transitions {
		if err := m.Store(ctx, transition); err != nil {
			return ids[:i], err
		}
		ids[i] = transition.ID
	}

	return ids, nil
}

// Sample implements Backend.Sample
func (m *MemoryBackend) Sample(ctx context.Context, config *SampleConfig) ([]*Transition, []float32, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get candidate transitions
	candidates := m.getCandidates(config)

	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("no transitions available for sampling")
	}

	// Determine sample size
	sampleSize := int(config.BatchSize)
	if sampleSize > len(candidates) {
		sampleSize = len(candidates)
	}

	var sampled []*Transition
	var weights []float32

	if config.Prioritized {
		sampled, weights = m.prioritizedSample(candidates, sampleSize, config.PriorityAlpha)
	} else {
		sampled = m.uniformSample(candidates, sampleSize)
		weights = make([]float32, sampleSize)
		for i := range weights {
			weights[i] = 1.0
		}
	}

	return sampled, weights, nil
}

// GetStats implements Backend.GetStats
func (m *MemoryBackend) GetStats(ctx context.Context, envID string) (*Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &Stats{
		TotalTransitions: uint64(len(m.transitions)),
		TotalEpisodes:    uint64(len(m.episodes)),
		TransitionsByEnv: make(map[string]uint64),
	}

	// Calculate storage bytes (approximate)
	for _, t := range m.transitions {
		stats.StorageBytes += uint64(len(t.State) + len(t.Action) + len(t.NextState) +
			len(t.Observation) + len(t.NextObservation) + 100) // ~100 bytes overhead
	}

	// Count transitions by environment
	for env, transitions := range m.envIndex {
		if envID == "" || env == envID {
			stats.TransitionsByEnv[env] = uint64(len(transitions))
		}
	}

	// Find oldest and newest timestamps
	if len(m.timeIndex) > 0 {
		oldest := m.transitions[m.timeIndex[0]]
		newest := m.transitions[m.timeIndex[len(m.timeIndex)-1]]
		stats.OldestTimestamp = &oldest.Timestamp
		stats.NewestTimestamp = &newest.Timestamp
	}

	return stats, nil
}

// UpdatePriorities implements Backend.UpdatePriorities
func (m *MemoryBackend) UpdatePriorities(ctx context.Context, transitionIDs []string, priorities []float32) error {
	if len(transitionIDs) != len(priorities) {
		return fmt.Errorf("mismatched lengths: %d IDs vs %d priorities", len(transitionIDs), len(priorities))
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, id := range transitionIDs {
		if transition, exists := m.transitions[id]; exists {
			transition.Priority = priorities[i]
		}
	}

	return nil
}

// Clear implements Backend.Clear
func (m *MemoryBackend) Clear(ctx context.Context, envID string, beforeTimestamp *time.Time, keepLastN uint32) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var toDelete []string

	for id, transition := range m.transitions {
		shouldDelete := false

		// Filter by environment
		if envID != "" && transition.EnvID != envID {
			continue
		}

		// Filter by timestamp
		if beforeTimestamp != nil && transition.Timestamp.Before(*beforeTimestamp) {
			shouldDelete = true
		}

		if shouldDelete {
			toDelete = append(toDelete, id)
		}
	}

	// Apply keepLastN constraint
	if keepLastN > 0 {
		relevantTransitions := make([]string, 0)
		for _, id := range m.timeIndex {
			transition := m.transitions[id]
			if envID == "" || transition.EnvID == envID {
				relevantTransitions = append(relevantTransitions, id)
			}
		}

		if len(relevantTransitions) > int(keepLastN) {
			keepCount := len(relevantTransitions) - int(keepLastN)
			for i := 0; i < keepCount; i++ {
				id := relevantTransitions[i]
				if !contains(toDelete, id) {
					toDelete = append(toDelete, id)
				}
			}
		}
	}

	// Delete the transitions
	for _, id := range toDelete {
		m.deleteTransition(id)
	}

	return uint64(len(toDelete)), nil
}

// Close implements Backend.Close
func (m *MemoryBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.transitions = nil
	m.episodes = nil
	m.envIndex = nil
	m.timeIndex = nil

	return nil
}

// Helper methods

func (m *MemoryBackend) insertInTimeIndex(id string, timestamp time.Time) {
	// Binary search for insertion point
	idx := sort.Search(len(m.timeIndex), func(i int) bool {
		return m.transitions[m.timeIndex[i]].Timestamp.After(timestamp)
	})

	// Insert at the found position
	m.timeIndex = append(m.timeIndex, "")
	copy(m.timeIndex[idx+1:], m.timeIndex[idx:])
	m.timeIndex[idx] = id
}

func (m *MemoryBackend) evictIfNeeded() {
	if m.maxSize == 0 || uint64(len(m.transitions)) <= m.maxSize {
		return
	}

	// Remove oldest transitions
	toRemove := uint64(len(m.transitions)) - m.maxSize
	for i := uint64(0); i < toRemove; i++ {
		if len(m.timeIndex) > 0 {
			oldestID := m.timeIndex[0]
			m.deleteTransition(oldestID)
		}
	}
}

func (m *MemoryBackend) deleteTransition(id string) {
	transition, exists := m.transitions[id]
	if !exists {
		return
	}

	// Remove from main storage
	delete(m.transitions, id)

	// Remove from episode index
	if transition.EpisodeID != "" {
		if episodeTransitions, exists := m.episodes[transition.EpisodeID]; exists {
			m.episodes[transition.EpisodeID] = removeString(episodeTransitions, id)
			if len(m.episodes[transition.EpisodeID]) == 0 {
				delete(m.episodes, transition.EpisodeID)
			}
		}
	}

	// Remove from environment index
	if transition.EnvID != "" {
		if envTransitions, exists := m.envIndex[transition.EnvID]; exists {
			m.envIndex[transition.EnvID] = removeString(envTransitions, id)
			if len(m.envIndex[transition.EnvID]) == 0 {
				delete(m.envIndex, transition.EnvID)
			}
		}
	}

	// Remove from time index
	m.timeIndex = removeString(m.timeIndex, id)
}

func (m *MemoryBackend) getCandidates(config *SampleConfig) []*Transition {
	var candidates []*Transition

	// Start with all transitions or filter by environment
	var transitionIDs []string
	if config.EnvID != "" {
		if envTransitions, exists := m.envIndex[config.EnvID]; exists {
			transitionIDs = envTransitions
		}
	} else {
		transitionIDs = make([]string, 0, len(m.transitions))
		for id := range m.transitions {
			transitionIDs = append(transitionIDs, id)
		}
	}

	// Apply timestamp filters
	for _, id := range transitionIDs {
		transition := m.transitions[id]

		if config.MinTimestamp != nil && transition.Timestamp.Before(*config.MinTimestamp) {
			continue
		}
		if config.MaxTimestamp != nil && transition.Timestamp.After(*config.MaxTimestamp) {
			continue
		}

		candidates = append(candidates, transition)
	}

	return candidates
}

func (m *MemoryBackend) uniformSample(candidates []*Transition, sampleSize int) []*Transition {
	if sampleSize >= len(candidates) {
		return candidates
	}

	// Fisher-Yates shuffle and take first sampleSize
	indices := make([]int, len(candidates))
	for i := range indices {
		indices[i] = i
	}

	for i := len(indices) - 1; i > 0; i-- {
		j := m.rng.Intn(i + 1)
		indices[i], indices[j] = indices[j], indices[i]
	}

	sampled := make([]*Transition, sampleSize)
	for i := 0; i < sampleSize; i++ {
		sampled[i] = candidates[indices[i]]
	}

	return sampled
}

func (m *MemoryBackend) prioritizedSample(candidates []*Transition, sampleSize int, alpha float32) ([]*Transition, []float32) {
	if sampleSize >= len(candidates) {
		sampled := make([]*Transition, len(candidates))
		weights := make([]float32, len(candidates))
		copy(sampled, candidates)
		for i := range weights {
			weights[i] = 1.0
		}
		return sampled, weights
	}

	// Calculate priority weights
	totalWeight := float64(0)
	priorities := make([]float64, len(candidates))

	for i, candidate := range candidates {
		// Priority^alpha
		priority := float64(candidate.Priority)
		if alpha != 1.0 {
			priority = pow(priority, float64(alpha))
		}
		priorities[i] = priority
		totalWeight += priority
	}

	// Sample using weighted random selection
	sampled := make([]*Transition, 0, sampleSize)
	weights := make([]float32, 0, sampleSize)
	used := make(map[int]bool)

	for len(sampled) < sampleSize {
		// Random selection based on priority weights
		target := m.rng.Float64() * totalWeight
		sum := float64(0)

		for i, priority := range priorities {
			if used[i] {
				continue
			}
			sum += priority
			if sum >= target {
				sampled = append(sampled, candidates[i])
				weights = append(weights, 1.0) // For simplicity, using uniform weights
				used[i] = true
				totalWeight -= priority
				break
			}
		}
	}

	return sampled, weights
}

// Utility functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func removeString(slice []string, item string) []string {
	for i, s := range slice {
		if s == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func pow(x, y float64) float64 {
	if y == 0 {
		return 1
	}
	if y == 1 {
		return x
	}

	result := x
	for i := 1; i < int(y); i++ {
		result *= x
	}
	return result
}