package registry

import (
	"context"
	"sync"
	"time"

	"github.com/cartridge/weights/internal/service"
)

// Subscriber is a callback invoked for every version update.
type Subscriber func(service.VersionSnapshot)

// Registry represents weight metadata persistence.
type Registry interface {
	Upsert(ctx context.Context, input service.PublishInput) (service.VersionSnapshot, error)
	Current(ctx context.Context, runID string) (service.VersionSnapshot, error)
	Subscribe(runID string, opts service.WatchOptions) (<-chan service.VersionSnapshot, func())
}

// MemoryRegistry is an in-memory implementation of Registry.
type MemoryRegistry struct {
	mu         sync.RWMutex
	versions   map[string]service.VersionSnapshot
	history    map[string][]service.VersionSnapshot
	watchers   map[string]map[int]chan service.VersionSnapshot
	nextWatch  int
	historyCap int
}

// NewMemoryRegistry creates a memory registry storing up to historyCap versions per run.
func NewMemoryRegistry(historyCap int) *MemoryRegistry {
	if historyCap <= 0 {
		historyCap = 1
	}
	return &MemoryRegistry{
		versions:   make(map[string]service.VersionSnapshot),
		history:    make(map[string][]service.VersionSnapshot),
		watchers:   make(map[string]map[int]chan service.VersionSnapshot),
		historyCap: historyCap,
	}
}

// Upsert records a new version for the run and notifies watchers.
func (m *MemoryRegistry) Upsert(ctx context.Context, input service.PublishInput) (service.VersionSnapshot, error) {
	snapshot := service.VersionSnapshot{
		RunID:       input.RunID,
		Step:        input.Step,
		Checksum:    input.Checksum,
		ArtifactURI: input.ArtifactURI,
		InlineBytes: append([]byte(nil), input.InlineBytes...),
		Metadata:    cloneMap(input.Metadata),
		PublishedAt: input.PublishedAt.Round(time.Millisecond),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.versions[input.RunID] = snapshot
	history := append(m.history[input.RunID], snapshot)
	if len(history) > m.historyCap {
		history = history[len(history)-m.historyCap:]
	}
	m.history[input.RunID] = history

	for _, ch := range m.watchers[input.RunID] {
		select {
		case ch <- snapshot:
		default:
			go func(out chan service.VersionSnapshot, snap service.VersionSnapshot) {
				select {
				case out <- snap:
				case <-ctx.Done():
				}
			}(ch, snapshot)
		}
	}

	return snapshot, nil
}

// Current returns the latest version for a run.
func (m *MemoryRegistry) Current(_ context.Context, runID string) (service.VersionSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot, ok := m.versions[runID]
	if !ok {
		return service.VersionSnapshot{}, service.ErrRunNotFound
	}
	return snapshot, nil
}

// Subscribe registers a watcher for the run.
func (m *MemoryRegistry) Subscribe(runID string, opts service.WatchOptions) (<-chan service.VersionSnapshot, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.watchers[runID] == nil {
		m.watchers[runID] = make(map[int]chan service.VersionSnapshot)
	}

	ch := make(chan service.VersionSnapshot, 1)
	id := m.nextWatch
	m.nextWatch++
	m.watchers[runID][id] = ch

	if opts.ReplayLatest {
		if snapshot, ok := m.versions[runID]; ok {
			ch <- snapshot
		}
	}

	cancel := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if watchers := m.watchers[runID]; watchers != nil {
			if ch, ok := watchers[id]; ok {
				delete(watchers, id)
				close(ch)
			}
			if len(watchers) == 0 {
				delete(m.watchers, runID)
			}
		}
	}

	return ch, cancel
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
