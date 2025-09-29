// Package policy provides action selection strategies for the actor
package policy

// Policy interface for action selection
type Policy interface {
	// SelectAction chooses an action given the current observation
	// Returns the action encoded as bytes (matching engine format)
	SelectAction(observation []byte) ([]byte, error)
}