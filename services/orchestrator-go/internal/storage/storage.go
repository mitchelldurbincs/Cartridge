package storage

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/cartridge/orchestrator/internal/types"
)

var (
	// ErrNotFound indicates the requested resource does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates optimistic concurrency or constraint violation.
	ErrConflict = errors.New("conflict")
	// ErrNoCommands indicates there are no pending commands for a run.
	ErrNoCommands = errors.New("no commands")
)

// RunStore captures the persistence operations the orchestrator relies on.
type RunStore interface {
	CreateRun(ctx context.Context, run types.Run) error
	GetRun(ctx context.Context, id string) (types.Run, error)
	UpdateRun(ctx context.Context, run types.Run) error
	AppendTransition(ctx context.Context, transition RunTransition) error
	AppendCommand(ctx context.Context, command types.RunCommand) error
	GetCommand(ctx context.Context, runID, commandID string) (types.RunCommand, error)
	NextPendingCommand(ctx context.Context, runID string) (types.RunCommand, error)
	SaveCommand(ctx context.Context, command types.RunCommand) error
}

// RunTransition records a state change for auditing.
type RunTransition struct {
	RunID     string         `json:"run_id"`
	FromState types.RunState `json:"from_state"`
	ToState   types.RunState `json:"to_state"`
	ChangedBy string         `json:"changed_by"`
	Reason    string         `json:"reason"`
	CreatedAt time.Time      `json:"created_at"`
}

// MemoryStore is an in-memory RunStore for development/testing.
type MemoryStore struct {
	mu          sync.RWMutex
	runs        map[string]types.Run
	commands    map[string]map[string]types.RunCommand // runID -> commandID -> command
	transitions map[string][]RunTransition
}

// NewMemoryStore constructs a MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		runs:        make(map[string]types.Run),
		commands:    make(map[string]map[string]types.RunCommand),
		transitions: make(map[string][]RunTransition),
	}
}

// CreateRun inserts a new run, enforcing uniqueness.
func (m *MemoryStore) CreateRun(_ context.Context, run types.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.runs[run.ID]; exists {
		return ErrConflict
	}
	m.runs[run.ID] = run
	return nil
}

// GetRun fetches a run by ID.
func (m *MemoryStore) GetRun(_ context.Context, id string) (types.Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.runs[id]
	if !ok {
		return types.Run{}, ErrNotFound
	}
	return run, nil
}

// UpdateRun replaces the stored run.
func (m *MemoryStore) UpdateRun(_ context.Context, run types.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[run.ID]; !ok {
		return ErrNotFound
	}
	m.runs[run.ID] = run
	return nil
}

// AppendTransition adds a state transition entry.
func (m *MemoryStore) AppendTransition(_ context.Context, transition RunTransition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transitions[transition.RunID] = append(m.transitions[transition.RunID], transition)
	return nil
}

// AppendCommand inserts a command if not already present.
func (m *MemoryStore) AppendCommand(_ context.Context, command types.RunCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	runCommands, ok := m.commands[command.RunID]
	if !ok {
		runCommands = make(map[string]types.RunCommand)
		m.commands[command.RunID] = runCommands
	}
	if _, exists := runCommands[command.ID]; exists {
		return ErrConflict
	}
	runCommands[command.ID] = command
	return nil
}

// SaveCommand upserts a command record.
func (m *MemoryStore) SaveCommand(_ context.Context, command types.RunCommand) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	runCommands, ok := m.commands[command.RunID]
	if !ok {
		runCommands = make(map[string]types.RunCommand)
		m.commands[command.RunID] = runCommands
	}
	runCommands[command.ID] = command
	return nil
}

// GetCommand fetches a command by run + ID.
func (m *MemoryStore) GetCommand(_ context.Context, runID, commandID string) (types.RunCommand, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runCommands, ok := m.commands[runID]
	if !ok {
		return types.RunCommand{}, ErrNotFound
	}
	cmd, ok := runCommands[commandID]
	if !ok {
		return types.RunCommand{}, ErrNotFound
	}
	return cmd, nil
}

// NextPendingCommand returns the oldest undelivered command for a run.
func (m *MemoryStore) NextPendingCommand(_ context.Context, runID string) (types.RunCommand, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, exists := m.runs[runID]; !exists {
		return types.RunCommand{}, ErrNotFound
	}
	runCommands, ok := m.commands[runID]
	if !ok {
		return types.RunCommand{}, ErrNoCommands
	}
	var pending []types.RunCommand
	for _, cmd := range runCommands {
		if cmd.DeliveredAt == nil {
			pending = append(pending, cmd)
		}
	}
	if len(pending) == 0 {
		return types.RunCommand{}, ErrNoCommands
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].IssuedAt.Before(pending[j].IssuedAt)
	})
	return pending[0], nil
}
