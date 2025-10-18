package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cartridge/weights/internal/redisclient"
	"github.com/cartridge/weights/internal/service"
)

// RedisRegistry persists versions to Redis for durability.
type RedisRegistry struct {
	client     *redisclient.Client
	historyCap int

	mu           sync.RWMutex
	watchers     map[string]map[int]chan service.VersionSnapshot
	nextWatch    int
	lastSnapshot map[string]service.VersionSnapshot
}

// NewRedisRegistry constructs a Redis-backed registry.
func NewRedisRegistry(client *redisclient.Client, historyCap int) (*RedisRegistry, error) {
	if client == nil {
		return nil, errors.New("redis registry: client is required")
	}
	if historyCap <= 0 {
		historyCap = 1
	}
	return &RedisRegistry{
		client:       client,
		historyCap:   historyCap,
		watchers:     make(map[string]map[int]chan service.VersionSnapshot),
		lastSnapshot: make(map[string]service.VersionSnapshot),
	}, nil
}

// Upsert records a new snapshot, persists it, and notifies local watchers.
func (r *RedisRegistry) Upsert(ctx context.Context, input service.PublishInput) (service.VersionSnapshot, error) {
	snapshot := toSnapshot(input)

	payload, err := json.Marshal(snapshot)
	if err != nil {
		return service.VersionSnapshot{}, fmt.Errorf("redis registry: failed to serialise snapshot: %w", err)
	}

	if _, err := r.client.Do(ctx, "SET", currentKey(snapshot.RunID), string(payload)); err != nil {
		return service.VersionSnapshot{}, fmt.Errorf("redis registry: set current failed: %w", err)
	}
	if _, err := r.client.Do(ctx, "LPUSH", historyKey(snapshot.RunID), string(payload)); err != nil {
		return service.VersionSnapshot{}, fmt.Errorf("redis registry: append history failed: %w", err)
	}
	if _, err := r.client.Do(ctx, "LTRIM", historyKey(snapshot.RunID), "0", fmt.Sprintf("%d", r.historyCap-1)); err != nil {
		return service.VersionSnapshot{}, fmt.Errorf("redis registry: trim history failed: %w", err)
	}

	r.recordSnapshot(snapshot)
	r.notifyWatchers(snapshot)
	return snapshot, nil
}

// Current loads the current snapshot for the run.
func (r *RedisRegistry) Current(ctx context.Context, runID string) (service.VersionSnapshot, error) {
	reply, err := r.client.Do(ctx, "GET", currentKey(runID))
	if err != nil {
		return service.VersionSnapshot{}, fmt.Errorf("redis registry: get current failed: %w", err)
	}
	if reply == nil {
		return service.VersionSnapshot{}, service.ErrRunNotFound
	}

	var snapshot service.VersionSnapshot
	switch v := reply.(type) {
	case []byte:
		if err := json.Unmarshal(v, &snapshot); err != nil {
			return service.VersionSnapshot{}, fmt.Errorf("redis registry: parse current failed: %w", err)
		}
	case string:
		if err := json.Unmarshal([]byte(v), &snapshot); err != nil {
			return service.VersionSnapshot{}, fmt.Errorf("redis registry: parse current failed: %w", err)
		}
	default:
		return service.VersionSnapshot{}, fmt.Errorf("redis registry: unexpected reply type %T", reply)
	}

	r.recordSnapshot(snapshot)
	return snapshot, nil
}

// Subscribe registers a local watcher and optionally replays the latest snapshot.
func (r *RedisRegistry) Subscribe(runID string, opts service.WatchOptions) (<-chan service.VersionSnapshot, func()) {
	ch := make(chan service.VersionSnapshot, 1)

	r.mu.Lock()
	if r.watchers[runID] == nil {
		r.watchers[runID] = make(map[int]chan service.VersionSnapshot)
	}
	id := r.nextWatch
	r.nextWatch++
	r.watchers[runID][id] = ch
	last := r.lastSnapshot[runID]
	r.mu.Unlock()

	if opts.ReplayLatest {
		if last.RunID != "" {
			ch <- last
		} else if snapshot, err := r.Current(context.Background(), runID); err == nil {
			ch <- snapshot
		}
	}

	cancel := func() {
		r.mu.Lock()
		if watchers := r.watchers[runID]; watchers != nil {
			if existing, ok := watchers[id]; ok {
				delete(watchers, id)
				close(existing)
			}
			if len(watchers) == 0 {
				delete(r.watchers, runID)
			}
		}
		r.mu.Unlock()
	}

	return ch, cancel
}

func (r *RedisRegistry) recordSnapshot(snapshot service.VersionSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastSnapshot[snapshot.RunID] = snapshot
}

func (r *RedisRegistry) notifyWatchers(snapshot service.VersionSnapshot) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	watchers := r.watchers[snapshot.RunID]
	if len(watchers) == 0 {
		return
	}

	for _, ch := range watchers {
		select {
		case ch <- snapshot:
		default:
		}
	}
}

func currentKey(runID string) string {
	return fmt.Sprintf("weights:current:%s", runID)
}

func historyKey(runID string) string {
	return fmt.Sprintf("weights:history:%s", runID)
}

func toSnapshot(input service.PublishInput) service.VersionSnapshot {
	return service.VersionSnapshot{
		RunID:       input.RunID,
		Step:        input.Step,
		Checksum:    input.Checksum,
		ArtifactURI: input.ArtifactURI,
		InlineBytes: append([]byte(nil), input.InlineBytes...),
		Metadata:    cloneMap(input.Metadata),
		PublishedAt: input.PublishedAt.Round(time.Millisecond),
	}
}
