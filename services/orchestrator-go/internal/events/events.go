package events

import "context"

// Publisher is implemented by downstream fan-out mechanisms.
type Publisher interface {
	PublishRunStatus(ctx context.Context, payload RunStatusEvent) error
	PublishCommandEvent(ctx context.Context, payload CommandEvent) error
}

// RunStatusEvent is emitted whenever run status/heartbeat fields change.
type RunStatusEvent struct {
	RunID            string  `json:"run_id"`
	State            string  `json:"state"`
	RuntimeStatus    string  `json:"runtime_status"`
	HealthStatus     string  `json:"health_status"`
	Step             int64   `json:"step"`
	SamplesPerSecond float64 `json:"samples_per_sec"`
	Loss             float64 `json:"loss"`
	LastError        string  `json:"last_error,omitempty"`
}

// CommandEvent tracks command lifecycle transitions.
type CommandEvent struct {
	RunID       string `json:"run_id"`
	CommandID   string `json:"command_id"`
	Type        string `json:"type"`
	Event       string `json:"event"`
	Description string `json:"description,omitempty"`
}

// NoopPublisher logs nothing; useful for tests.
type NoopPublisher struct{}

// PublishRunStatus satisfies Publisher.
func (NoopPublisher) PublishRunStatus(context.Context, RunStatusEvent) error { return nil }

// PublishCommandEvent satisfies Publisher.
func (NoopPublisher) PublishCommandEvent(context.Context, CommandEvent) error { return nil }
