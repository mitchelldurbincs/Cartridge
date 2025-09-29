package policy

import (
	"testing"

	"github.com/cartridge/actor/pkg/proto/engine/v1"
)

func TestRandomPolicy_Discrete(t *testing.T) {
	// Create discrete action space (like TicTacToe)
	actionSpace := &engine.Capabilities_DiscreteN{
		DiscreteN: 9,
	}

	policy, err := NewRandom(actionSpace)
	if err != nil {
		t.Fatalf("Failed to create random policy: %v", err)
	}

	// Test action selection
	observation := make([]byte, 116) // TicTacToe observation size
	action, err := policy.SelectAction(observation)
	if err != nil {
		t.Fatalf("Failed to select action: %v", err)
	}

	// Should return 1 byte for discrete action
	if len(action) != 1 {
		t.Errorf("Expected 1 byte action, got %d bytes", len(action))
	}

	// Action should be in valid range [0, 8]
	actionValue := action[0]
	if actionValue >= 9 {
		t.Errorf("Action %d out of range [0, 8]", actionValue)
	}
}

func TestRandomPolicy_MultiDiscrete(t *testing.T) {
	// Create multi-discrete action space
	actionSpace := &engine.Capabilities_Multi{
		Multi: &engine.MultiDiscrete{
			Nvec: []uint32{3, 4, 2}, // 3 dimensions
		},
	}

	policy, err := NewRandom(actionSpace)
	if err != nil {
		t.Fatalf("Failed to create random policy: %v", err)
	}

	// Test action selection
	observation := make([]byte, 64)
	action, err := policy.SelectAction(observation)
	if err != nil {
		t.Fatalf("Failed to select action: %v", err)
	}

	// Should return 12 bytes (3 dimensions * 4 bytes each)
	expectedBytes := 3 * 4
	if len(action) != expectedBytes {
		t.Errorf("Expected %d bytes action, got %d bytes", expectedBytes, len(action))
	}
}

func TestRandomPolicy_Continuous(t *testing.T) {
	// Create continuous action space
	actionSpace := &engine.Capabilities_Continuous{
		Continuous: &engine.BoxSpec{
			Low:   []float32{-1.0, -2.0},
			High:  []float32{1.0, 2.0},
			Shape: []uint32{2},
		},
	}

	policy, err := NewRandom(actionSpace)
	if err != nil {
		t.Fatalf("Failed to create random policy: %v", err)
	}

	// Test action selection
	observation := make([]byte, 32)
	action, err := policy.SelectAction(observation)
	if err != nil {
		t.Fatalf("Failed to select action: %v", err)
	}

	// Should return 8 bytes (2 dimensions * 4 bytes each)
	expectedBytes := 2 * 4
	if len(action) != expectedBytes {
		t.Errorf("Expected %d bytes action, got %d bytes", expectedBytes, len(action))
	}
}

func TestRandomPolicy_InvalidActionSpace(t *testing.T) {
	// Test with nil action space
	_, err := NewRandom(nil)
	if err == nil {
		t.Error("Expected error for nil action space")
	}
}

func TestRandomPolicy_MultipleSelections(t *testing.T) {
	// Test that multiple selections produce different results (probabilistically)
	actionSpace := &engine.Capabilities_DiscreteN{
		DiscreteN: 9,
	}

	policy, err := NewRandom(actionSpace)
	if err != nil {
		t.Fatalf("Failed to create random policy: %v", err)
	}

	observation := make([]byte, 116)
	actionSet := make(map[byte]bool)

	// Generate 100 actions, should see some variety
	for i := 0; i < 100; i++ {
		action, err := policy.SelectAction(observation)
		if err != nil {
			t.Fatalf("Failed to select action: %v", err)
		}
		actionSet[action[0]] = true
	}

	// Should have at least 2 different actions (highly probable)
	if len(actionSet) < 2 {
		t.Errorf("Expected multiple different actions, got only %d unique actions", len(actionSet))
	}
}